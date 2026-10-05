package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/theme"
	"strings"
	"testing"
)

func TestSuggestionEditorSelectionQueueRecoveryShape(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "RIGHT", 1)
	m.toggleCommentRange()
	selectRawTarget(t, m, "RIGHT", 2)
	m.openSuggestionComposer()
	c := m.Composer
	if c == nil || !c.Suggestion || c.Draft != "first\nsecond" || c.Target.StartLine != 1 {
		t.Fatalf("composer %+v", c)
	}
	c.Draft = "replacement\n```"
	c.Cursor = len([]rune(c.Draft))
	view := m.inlineEditorLines()
	var text string
	for _, l := range view {
		text += l.Text + "\n"
	}
	if !strings.Contains(text, "Before") || !strings.Contains(text, "- first") || !strings.Contains(text, "+ replacement") {
		t.Fatal(text)
	}
	d := draftContent(m.reviewTabState)
	if d.Composer == nil || !d.Composer.Suggestion || d.Composer.Body != c.Draft || d.Composer.Before != "first\nsecond" {
		t.Fatal("replacement recovery lost", d)
	}
	m.commentComposerKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if len(m.Pending) != 1 || m.Pending[0].Target.StartLine != 1 {
		t.Fatal("queue target lost")
	}
	got, err := source.ParseSuggestion(m.Pending[0].Body)
	if err != nil || got != "replacement\n```" {
		t.Fatal(got, err)
	}
}
func TestSuggestionApplyRequiresSeparateConfirmationAndBlocksRetry(t *testing.T) {
	m := rangeTestModel()
	m.push(pageSuggestionApply)
	m.SuggestionApply = &source.SuggestionApplication{}
	calls := 0
	m.SetSuggestionActions(nil, func(context.Context, source.SuggestionApplication) (source.SuggestionResult, error) {
		calls++
		return source.SuggestionResult{SHA: strings.Repeat("c", 40)}, nil
	})
	if cmd := m.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || calls != 0 || !m.SuggestionConfirm {
		t.Fatal("first enter wrote")
	}
	cmd := m.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.draft.attempt != "suggestion" || m.draft.attempted.Application == nil {
		t.Fatal("attempt not frozen")
	}
	if next := m.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}); next != nil {
		t.Fatal("uncertain retry allowed")
	}
	_ = cmd()
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestSuggestionRejectsOldAndHistoricalSelections(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "LEFT", 1)
	m.openSuggestionComposer()
	if m.Composer != nil || m.ActionError == nil {
		t.Fatal("old suggestion accepted")
	}
	m.ContextView = viewCommits
	m.openSuggestionComposer()
	if !strings.Contains(m.ActionError.Error(), "historical") {
		t.Fatal(m.ActionError)
	}
}

func TestSuggestionReplacementEmptyAndEscRemainEditorState(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "RIGHT", 1)
	m.openSuggestionComposer()
	m.Composer.Draft = ""
	m.Composer.Cursor = 0
	var received CommentSubmission
	m.SetCommentSubmitter(func(_ context.Context, s CommentSubmission) (source.ReviewComment, error) {
		received = s
		return s.Comment, nil
	})
	cmd := m.commentComposerKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("deletion suggestion blocked")
	}
	_ = cmd()
	replacement, err := source.ParseSuggestion(received.Comment.Body)
	if err != nil || replacement != "" {
		t.Fatal(replacement, err)
	}
	if m.Composer == nil || !m.Composer.Suggestion || m.Composer.Draft != "" {
		t.Fatal("replacement editor mutated")
	}
}

func TestSuggestionReplacementDraftRecoversOfflineWithoutFlattening(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 3, StartLine: 1, StartSide: "RIGHT"}
	m.Composer = &commentComposer{Target: target, Suggestion: true, Before: "first\nsecond\nthird", Draft: "edited replacement", PendingIndex: -1}
	if !m.persistDraft(m.reviewTabState) {
		t.Fatal(m.ActionError)
	}
	restored := draftTestModel(t, store)
	if restored.Composer == nil || !restored.Composer.Suggestion || restored.Composer.Target != target || restored.Composer.Draft != "edited replacement" || restored.Composer.Before != m.Composer.Before {
		t.Fatal("recovery lost suggestion", restored.Composer)
	}
	if restored.submitComment != nil || restored.applySuggestion != nil {
		t.Fatal("recovery enabled remote writes")
	}
}

func TestSuggestionApplyAttemptDurableRestartAndIndependentPayload(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	meta.BaseRepository = "owner/repo"
	meta.HeadRepository = "owner/repo"
	m.Session.Inventory.Comparison.Metadata = meta
	body, _ := source.SuggestionBody("new")
	a := source.SuggestionApplication{Metadata: meta, Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, CommentID: 9, Branch: "feature", Before: "old", Replacement: "new", Content: strings.Repeat("x", 400<<10), CommentBody: body}
	// Build the documented wire fixture independently of the implementation.
	input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": "text", "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
	a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
	m.SuggestionApply = &a
	if !m.prepareDraftAttempt("suggestion") {
		t.Fatal(m.ActionError)
	}
	d := draftContent(m.reviewTabState)
	if d.Version != 3 || d.SuggestionApply != nil || d.Attempted.Application == nil {
		t.Fatal("duplicate attempt or unsafe version")
	}
	restored := draftTestModel(t, store)
	if restored.SuggestionApply == nil || restored.draft.attempt != "suggestion" || restored.SuggestionApply.Content != a.Content || string(restored.draft.attempted.Application.Payload) != string(a.Payload) {
		t.Fatal("attempt recovery failed", restored.ActionError)
	}
	restored.draftRecoveryKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if restored.top() != pageSuggestionApply {
		t.Fatal("apply recovery did not open preview")
	}
	restored.SuggestionApply.Replacement = "later edit"
	if restored.draft.attempted.Application.Replacement != "new" {
		t.Fatal("edited attempt payload")
	}
	if cmd := restored.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("offline uncertain retry")
	}
	restored.draftRecoveryKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
}

func TestSuggestionConfirmationScrollKeepsControlsAndEmptyDraftUnsent(t *testing.T) {
	m := rangeTestModel()
	m.Width = 60
	m.Height = 12
	m.SuggestionApply = &source.SuggestionApplication{Target: source.ReviewCommentTarget{Path: "long/path", Side: "RIGHT", Line: 1}, Metadata: source.Metadata{HeadRepository: "owner/repo"}, Branch: "feature", Before: strings.Repeat("old\n", 30), Replacement: strings.Repeat("new\n", 30)}
	view := m.suggestionApplyView()
	if len(strings.Split(view, "\n")) > m.Height-2 || !strings.Contains(view, "enter: review confirmation") {
		t.Fatal(view)
	}
	for i := 0; i < 40; i++ {
		m.suggestionApplyKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	view = m.suggestionApplyView()
	if !strings.Contains(view, "+ new") || !strings.Contains(view, "enter: review confirmation") {
		t.Fatal(view)
	}
	m.Width = 20
	m.SuggestionConfirm = true
	if cmd := m.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || m.ActionError == nil {
		t.Fatal("tiny terminal permitted apply")
	}
	m.SuggestionApply = nil
	m.Composer = &commentComposer{Suggestion: true, Draft: ""}
	if !m.unsentReviewDrafts() {
		t.Fatal("deletion editor not considered unsent")
	}
}

func TestSuggestionRestackConversationRangeAndDraftIsolation(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	m.Height = 80
	selectRawTarget(t, m, "RIGHT", 1)
	m.toggleCommentRange()
	selectRawTarget(t, m, "RIGHT", 2)
	m.openSuggestionComposer()
	m.Composer.Draft = "combined replacement"
	m.commentComposerKey(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	queued := m.Pending[0]
	target := queued.Target
	if d := draftContent(m.reviewTabState); d.Version != 2 || d.Pending[0].Target.StartLine != 1 || d.Pending[0].Target.Line != 2 {
		t.Fatal("queued range schema changed", d)
	}
	published := queued
	published.ID = 47
	published.Author = "alice"
	published.CurrentAnchor = &target
	published.OriginalAnchor = &target
	known := false
	snapshot := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{{ID: "suggestion-thread", CurrentAnchor: &target, OriginalAnchor: &target, OriginalCommitID: target.CommitID, Resolved: &known, Outdated: &known, Comments: []source.ReviewComment{published}}}, Events: []source.ConversationEvent{{ID: "PR comment:51", Kind: "PR comment", Author: "bob", Body: "general conversation"}}}}
	m.applyDiscussionResult(DiscussionResult{Target: m.activeTab, Session: m.Session, Snapshot: snapshot})
	m.openDiscussions()
	for i, e := range m.discussionEntries() {
		if e.ID == "inline:47" {
			m.discussions.selected = i
			m.discussions.selectedID = e.ID
		}
	}
	m.discussionKey("enter")
	view := m.discussionsView()
	for _, part := range []string{"PR conversation", "Before", "After", "first", "second", "combined replacement"} {
		if !strings.Contains(view, part) {
			t.Fatalf("conversation lost %q: %s", part, view)
		}
	}
	calls := 0
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		calls++
		return source.ConversationEvent{}, nil
	})
	m.discussionKey("n")
	m.Update(tea.KeyPressMsg{Code: 't', Text: "separate general text"})
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if calls != 0 || m.Composer != nil || len(m.Pending) != 1 || m.Pending[0] != queued || m.discussions.editor == nil || m.discussions.editor.draft != "separate general text" {
		t.Fatal("general editor crossed suggestion/draft boundary")
	}
	m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.discussions.detail = false
	m.discussionKey("esc")
	m.CommentMenu = &commentActionMenu{CommentID: 47, Target: target}
	var received source.ReviewComment
	var before string
	m.SetSuggestionActions(func(_ context.Context, _ source.Metadata, c source.ReviewComment, b string) (source.SuggestionApplication, error) {
		received = c
		before = b
		return source.SuggestionApplication{}, errors.New("synthetic preparation boundary")
	}, nil)
	cmd := m.commentActionKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("canonical inline action unavailable")
	}
	m.Update(cmd())
	if received.ID != 47 || received.Target != target || before != "first\nsecond" || len(m.Pending) != 1 || m.Pending[0] != queued {
		t.Fatal("conversation refresh flattened or retargeted suggestion", received, before)
	}
}

func TestSuggestionRestackFilteredComparisonCannotCompose(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	selectRawTarget(t, m, "RIGHT", 1)
	original := m.Session
	target := *m.selectedDiffTarget()
	m.commitFilter.subset = true
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.Composer != nil || m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "read-only") || m.Session != original || *m.selectedDiffTarget() != target {
		t.Fatal("filtered comparison reused canonical composition target")
	}
	m.commitFilter.subset = false
	m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.Composer == nil || !m.Composer.Suggestion || m.Composer.Target != target {
		t.Fatal("all-commits canonical suggestion unavailable")
	}
}

func TestSuggestionRestackApplyModalUsesCanonicalThemeSurface(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	m.Width, m.Height = 120, 24
	target := source.ReviewCommentTarget{Path: "text", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
	m.SuggestionApply = &source.SuggestionApplication{Target: target, Metadata: source.Metadata{HeadRepository: "owner/repo"}, Branch: "feature", Before: "first\nsecond", Replacement: "new"}
	m.push(pageSuggestionApply)
	palette, err := theme.Resolve(theme.CatppuccinMocha, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(palette)
	plain := m.View().Content
	m.colorProfile = colorprofile.TrueColor
	colored := m.View().Content
	if ansi.Strip(colored) != plainCanvas(ansi.Strip(plain), m.Width, m.Height) {
		t.Fatal("modal theme changed suggestion confirmation text")
	}
	cells := themeCanvasBuffer(colored, m.Width, m.Height)
	found := false
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if sameCanvasColor(cells.CellAt(x, y).Style.Bg, modalBackground(palette)) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("suggestion modal bypassed canonical theme surface")
	}
	if !sameCanvasColor(cells.CellAt(0, 0).Style.Bg, themeBaseColor(palette, theme.Background)) {
		t.Fatal("modal theme leaked into background")
	}
}
