package tui

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

func inboxIntegrationSession(t *testing.T, store *session.Store, repository string) *review.Session {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write(".gitattributes", "generated.go review-generated\n")
	r.Write("generated.go", "old generated\n")
	r.Write("code.txt", "before\n"+strings.Repeat("context\n", 30))
	base := r.Commit()
	r.Write("generated.go", "new generated\n")
	r.Write("code.txt", "after\n"+strings.Repeat("context\n", 30))
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: repository, Number: 7}, BaseRepository: repository, HeadRepository: repository, BaseSHA: base, HeadSHA: head}
	raw, err := review.OpenWithConfig(context.Background(), r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{CacheFullSource: true})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
func inboxIntegrationDraft(t *testing.T, store *session.Store, s *review.Session, body string, uncertain bool) session.Draft {
	t.Helper()
	meta := s.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "code.txt", Side: "RIGHT", StartSide: "RIGHT", StartLine: 1, Line: 2}
	file := target
	file.SubjectType = "file"
	file.Side = ""
	file.StartSide = ""
	file.StartLine = 0
	file.Line = 0
	pending := []source.ReviewComment{{Target: target, Body: "```suggestion\n" + body + "\n```"}, {Target: file, Body: body + " file"}}
	attempt := source.PullRequestReview{Identity: meta.Identity, CommitID: meta.HeadSHA, Event: "COMMENT", Body: body + " summary", Comments: pending[:1]}
	a := source.SuggestionApplication{Metadata: meta, Target: target, CommentID: 9, Branch: "feature", Before: "before", Replacement: body, Content: body + "\n", CommentBody: pending[0].Body}
	input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": target.Path, "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
	a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
	draft := session.Draft{Version: 3, Summary: attempt.Body, Pending: pending, Attempt: "suggestion", Attempted: &session.DraftAttempt{Kind: "suggestion", Application: &a}}
	if !uncertain {
		draft.Attempt = ""
		draft.Attempted = nil
		draft.SuggestionApply = &a
	}
	if meta.Identity.Repository == "o/first" {
		draft.Version = 4
		draft.General = &session.GeneralDraft{Body: body + " edited general", Cursor: 3, ReplyTo: "PR comment:42", AttemptedBody: body + " immutable general", ObservedIDs: []string{"PR comment:42"}, Uncertain: true}
	}
	d, err := store.SaveDraft(context.Background(), session.DraftKeyFor(meta), 0, draft)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestInboxCombinedSwitchRefreshCancelRestorePreservesFrozenDraftsAndNavigation(t *testing.T) {
	t.Run("reachable prepared suggestion keyboard flow", func(t *testing.T) { inboxCombinedPersistence(t, false) })
	t.Run("uncertain modal guard and background tab persistence", func(t *testing.T) { inboxCombinedPersistence(t, true) })
}
func inboxCombinedPersistence(t *testing.T, uncertain bool) {
	ctx := context.Background()
	store, err := session.Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := inboxIntegrationSession(t, store, "o/first")
	second := inboxIntegrationSession(t, store, "o/second")
	firstDraft := inboxIntegrationDraft(t, store, first, "PRIVATE FIRST", uncertain)
	secondDraft := inboxIntegrationDraft(t, store, second, "PRIVATE SECOND", uncertain)
	rawRows := inboxPrivateRows(t, store.Path())
	m := New(ctx, nil)
	defer m.Close()
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(first)
	namedKey(m, tea.KeyEnter)
	inboxLeaveApplicationForFixture(t, m, uncertain)
	key(m, '2')
	m.Width, m.Height = 160, 24
	code := -1
	for i, f := range m.Session.Inventory.Files {
		if string(f.NewPath) == "code.txt" {
			code = i
		}
	}
	if code < 0 {
		t.Fatal("source fixture missing")
	}
	m.selectFile(code)
	m.Focus = paneDiff
	m.cursorActive = true
	// Raw comment selection must remain anchored through merged layout controls.
	var raw source.ReviewCommentTarget
	for i, row := range m.displayDetail() {
		if row.target != nil && row.target.Path == "code.txt" && row.target.Side == "RIGHT" && row.target.Line == 1 {
			raw = *row.target
			m.setCursor(i)
			m.setSelectedDiffTarget(row.target)
			break
		}
	}
	if raw.Path == "" {
		t.Fatal("raw selection missing")
	}
	key(m, 'B')
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	key(m, 'S')
	m.resizeList(4)
	if got := m.selectedDiffTarget(); got == nil || *got != raw || m.commitFilter.open {
		t.Fatal("layout/collapse retargeted selection or opened commit filter")
	}
	key(m, 'S') // Return to unified source rows before searching.
	m.navigation = codeNavigation{mode: "NEW", whitespace: true}
	m.fileCache = fileDetailCache{}
	searchInput(m, "context")
	namedKey(m, tea.KeyEnter)
	sourceID := m.displayDetail()[m.cursor()].searchID
	if sourceID.Unit == "" || m.selectedDiffTarget() != nil {
		t.Fatal("source search lost immutable row or became comment target")
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	m.resizeList(3)
	if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], sourceID) {
		t.Fatal("collapse moved source search anchor")
	}
	m.search = [2]*diffSearchState{}
	key(m, 'F')
	namedKey(m, tea.KeyEscape)
	if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], sourceID) {
		t.Fatal("path search moved source anchor")
	}
	m.Focus = paneDiff
	m.Horizontal = 2
	navigation := m.navigation
	scroll, cursor, horizontal := m.Scroll[0], m.Cursor[0], m.Horizontal
	pin, _ := json.Marshal(first)
	draft, _ := json.Marshal(draftContent(m.reviewTabState))
	writes := 0
	m.SetReviewSubmitter(func(context.Context, ReviewSubmission) error { writes++; return nil })
	reads := 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		reads++
		return source.Readiness{Identity: id, ObservedAt: time.Now(), HeadSHA: first.Inventory.Comparison.Metadata.HeadSHA}, nil
	})
	loader := func(_ context.Context, _ source.InboxOptions, refresh bool) (source.Inbox, error) {
		return source.Inbox{Viewer: "account", Complete: !refresh, Problems: []string{"partial remote evidence"}, Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: second.Inventory.Comparison.Metadata.Identity}, Activity: "unknown"}, {PullRequest: source.PullRequest{Identity: first.Inventory.Comparison.Metadata.Identity}, Activity: "read"}}}, nil
	}
	opened := 0
	m.SetInbox(loader, nil, func(_ context.Context, checkout string, id source.Identity, _ func(string)) (*review.Session, error) {
		opened++
		if checkout != "" || id != second.Inventory.Comparison.Metadata.Identity {
			t.Fatal("inbox borrowed repository checkout or identity")
		}
		return store.Load(second.ID)
	}, source.InboxOptions{}, false, false)
	// Actual keyboard dispatch: Files -> switcher -> inbox -> explicit refresh.
	_, switcher := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if switcher != nil {
		completeAction(t, m, switcher)
	}
	_, load := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if load == nil || m.top() != pageInbox {
		t.Fatal("inbox not reachable from real switcher")
	}
	m.Update(load())
	_, refresh := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m.Update(refresh())
	if m.inbox.data.Complete {
		t.Fatal("partial refresh promoted to complete")
	}
	afterPin, _ := json.Marshal(first)
	afterDraft, _ := json.Marshal(draftContent(m.reviewTabState))
	if !bytes.Equal(pin, afterPin) || !bytes.Equal(draft, afterDraft) {
		t.Fatal("inbox refresh changed source or private attempt")
	}
	_, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, open)
	if opened != 1 || m.Session.ID != second.ID || m.top() != pageDraftRecovery || m.Pending[0].Target.Identity != second.Inventory.Comparison.Metadata.Identity || m.draft.attempt != secondDraft.Attempt {
		t.Fatal("own frozen session/draft recovery lost")
	}
	namedKey(m, tea.KeyEnter)
	inboxLeaveApplicationForFixture(t, m, uncertain)
	key(m, 'P')
	_, load = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m.Update(load())
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], sourceID) || m.selectedDiffTarget() != nil {
		t.Fatal("inbox return lost read-only source identity")
	}
	if m.Session.ID != first.ID || m.navigation != navigation || m.Scroll[0] != scroll || m.Cursor[0] != cursor || m.Horizontal != horizontal || m.Focus != paneDiff {
		t.Fatal("returning to original inbox identity disturbed navigation")
	}
	key(m, 'S')
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 24})
	if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], sourceID) || m.selectedDiffTarget() != nil {
		t.Fatal("restored inbox source anchor lost during narrow/wide layout")
	}
	// Cancel refresh through real dispatch, then deliver late evidence.
	key(m, 'P')
	_, load = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m.Update(load())
	_, late := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	namedKey(m, tea.KeyEscape)
	m.Update(late())
	if m.top() != pagePullRequestPicker || m.Session.ID != first.ID {
		t.Fatal("cancel retargeted active review")
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'C')
	if !m.commitFilter.open {
		t.Fatal("C commit filter contract lost")
	}
	key(m, 'C')
	_, ready := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if ready == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R readiness contract lost")
	}
	m.Update(ready())
	namedKey(m, tea.KeyEscape)
	if reads != 1 || writes != 0 {
		t.Fatal("wrong readiness dispatch or implicit review write")
	}
	for _, pair := range []struct {
		s *review.Session
		d session.Draft
	}{{first, firstDraft}, {second, secondDraft}} {
		got, err := store.LoadDraft(ctx, session.DraftKeyFor(pair.s.Inventory.Comparison.Metadata))
		want, _ := json.Marshal(pair.d)
		actual, _ := json.Marshal(got)
		if err != nil || !bytes.Equal(want, actual) {
			t.Fatal("private draft/attempt was changed or retargeted", err)
		}
		saved, err := store.Load(pair.s.ID)
		if err != nil || saved.SnapshotReference != pair.s.SnapshotReference || saved.Generation != pair.s.Generation {
			t.Fatal("frozen saved source changed", err)
		}
	}

	// Reopen the store and model as a new process would; recovery must retain
	// original application bytes and general delivery intent under their own keys.
	m.Close()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	restartedStore, err := session.Open(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer restartedStore.Close()
	restarted := New(ctx, nil)
	defer restarted.Close()
	restarted.SetLifecycle(restartedStore, nil, nil)
	for _, original := range []*review.Session{first, second} {
		saved, err := restartedStore.Load(original.ID)
		if err != nil {
			t.Fatal(err)
		}
		restarted.openReviewTab(saved)
		if restarted.top() != pageDraftRecovery || restarted.SuggestionApply == nil || restarted.SuggestionApply.Target.Identity != original.Inventory.Comparison.Metadata.Identity {
			t.Fatal("restart retargeted immutable application")
		}
		namedKey(restarted, tea.KeyEnter)
		if uncertain && restarted.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
			t.Fatal("uncertain application retried on recovery")
		}
		if original.ID == first.ID && (restarted.discussions.editor == nil || restarted.discussions.editor.attemptedBody != firstDraft.General.AttemptedBody || !restarted.discussions.editor.uncertain || restarted.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil) {
			t.Fatal("general immutable intent lost or retried")
		}
	}
	if !bytes.Equal(rawRows, inboxPrivateRows(t, restartedStore.Path())) {
		t.Fatal("private SQLite payload bytes or generations changed")
	}
}

func inboxPrivateRows(t *testing.T, directory string) []byte {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(directory, "store.sqlite3")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT repository,pr_number,base_sha,head_sha,base_repository,head_repository,generation,payload FROM review_drafts ORDER BY repository`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var result bytes.Buffer
	for rows.Next() {
		var repository, base, head, baseRepo, headRepo string
		var number int
		var generation uint64
		var payload []byte
		if err := rows.Scan(&repository, &number, &base, &head, &baseRepo, &headRepo, &generation, &payload); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal([]any{repository, number, base, head, baseRepo, headRepo, generation, payload})
		result.Write(encoded)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

// Prepared suggestions can return to the review via public Update. Uncertain
// applications deliberately cannot; that subtest checks the modal guard first,
// then exercises background tab/storage invariants, not a keyboard escape path.
func inboxLeaveApplicationForFixture(t *testing.T, m *Model, uncertain bool) {
	t.Helper()
	before, _ := json.Marshal(draftContent(m.reviewTabState))
	namedKey(m, tea.KeyEscape)
	after, _ := json.Marshal(draftContent(m.reviewTabState))
	if !bytes.Equal(before, after) {
		t.Fatal("Escape changed retained delivery intent")
	}
	if uncertain {
		if m.top() != pageSuggestionApply {
			t.Fatal("uncertain application escaped its reconciliation guard")
		}
		m.pop() // Model-level background review fixture, not a public keyboard path.
	} else if m.top() != pageReview {
		t.Fatal("prepared application cannot return to review")
	}
}
