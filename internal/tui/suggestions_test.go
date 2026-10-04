package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"prui/internal/session"
	"prui/internal/source"
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
