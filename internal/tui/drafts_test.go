package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
	"time"
)

func TestDraftEditsSurviveRestartWithoutRemoteWrite(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := kindsSession()
	s.ID = strings.Repeat("d", 32)
	s.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("a", 40)
	s.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("b", 40)
	s.Inventory.Comparison.Metadata.BaseRepository = "owner/repo"
	s.Inventory.Comparison.Metadata.HeadRepository = "owner/repo"
	m := New(context.Background(), nil)
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(s)
	meta := s.Inventory.Comparison.Metadata
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a.go", Side: "RIGHT", Line: 1}, PendingIndex: -1}
	key(m, 'x')
	restored := New(context.Background(), nil)
	restored.SetLifecycle(store, nil, nil)
	restored.openReviewTab(s)
	if restored.Composer == nil || restored.Composer.Draft != "x" {
		t.Fatalf("lost composer: %#v", restored.Composer)
	}
	namedKey(restored, tea.KeyEnter) // acknowledge recovery
	namedKey(restored, tea.KeyEscape)
	fresh := New(context.Background(), nil)
	fresh.SetLifecycle(store, nil, nil)
	fresh.openReviewTab(s)
	if fresh.Composer != nil {
		t.Fatal("discarded composer recovered")
	}
	m = restored
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 's')
	fresh = New(context.Background(), nil)
	fresh.SetLifecycle(store, nil, nil)
	fresh.openReviewTab(s)
	if fresh.ReviewForm == nil || fresh.ReviewForm.Body != "s" {
		t.Fatalf("lost summary: %#v", fresh.ReviewForm)
	}
}

func draftTestModel(t *testing.T, store *session.Store) *Model {
	t.Helper()
	s := kindsSession()
	s.ID = strings.Repeat("d", 32)
	s.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("a", 40)
	s.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("b", 40)
	m := New(context.Background(), nil)
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(s)
	return m
}
func TestDraftAttemptDurableBeforeDispatchAndReconcilesOriginalPayload(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	m.Composer = &commentComposer{Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, Draft: "original", PendingIndex: -1}
	if !m.prepareDraftAttempt("comment") {
		t.Fatal(m.ActionError)
	}
	// Simulate a crash after durable intent but before receiving a write result.
	restarted := draftTestModel(t, store)
	if restarted.draft.attempt != "comment" || restarted.Composer.Draft != "original" {
		t.Fatal("lost in-flight request")
	}
	namedKey(restarted, tea.KeyEnter) // acknowledge recovery
	calls := 0
	restarted.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		calls++
		return source.ReviewComment{}, nil
	})
	_, cmd := restarted.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || calls != 0 {
		t.Fatal("retried without reconciliation")
	}
	restarted.Composer.Draft = "edited after uncertainty"
	var observed string
	restarted.SetDraftReconciler(func(_ context.Context, _ source.Metadata, d session.Draft) (bool, error) {
		observed = d.Attempted.Comment.Body
		return false, nil
	})
	_, cmd = restarted.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	completeAction(t, restarted, cmd)
	if observed != "original" || restarted.draft.attempt != "" {
		t.Fatalf("reconciled edited request: %q", observed)
	}
	got, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || got.Attempt != "" || got.Composer.Body != "edited after uncertainty" {
		t.Fatalf("reconciliation not saved: %#v %v", got, err)
	}
}
func TestReplyDraftAndUncertainMatchSurviveRestart(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	m.CommentMenu = &commentActionMenu{Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, CommentID: 9, ReplyToID: 7, mode: commentActionReply}
	m.Comments = []source.ReviewComment{{ID: 7, Target: m.CommentMenu.Target}}
	m.resolveReplyRoot(m.CommentMenu)
	key(m, 'x')
	restarted := draftTestModel(t, store)
	if restarted.CommentMenu == nil || restarted.CommentMenu.Draft != "x" || restarted.CommentMenu.ReplyToID != 7 {
		t.Fatal("reply recovery failed")
	}
	if !restarted.prepareDraftAttempt("reply") {
		t.Fatal(restarted.ActionError)
	}
	restarted.SetDraftReconciler(func(_ context.Context, _ source.Metadata, d session.Draft) (bool, error) {
		if d.Attempted.ParentID != 7 {
			t.Fatal("reply retargeted")
		}
		return true, nil
	})
	_, cmd := restarted.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	completeAction(t, restarted, cmd)
	if restarted.draft.attempt != "reply" || !strings.Contains(restarted.ActionError.Error(), "matching submission") {
		t.Fatal("match enabled duplicate retry")
	}
	restarted = draftTestModel(t, store)
	if restarted.CommentMenu.Draft != "x" || restarted.draft.attempt != "reply" {
		t.Fatal("match dropped durable draft")
	}
	namedKey(restarted, tea.KeyEnter) // acknowledge recovery
	namedKey(restarted, tea.KeyEscape)
	reopened := draftTestModel(t, store)
	if reopened.CommentMenu != nil || reopened.draft.attempt != "" {
		t.Fatal("explicit discard not persisted")
	}
}
func TestDraftNewComparisonAndTabSwitchPreserveOldWork(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	old := m.Session
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 'a')
	namedKey(m, tea.KeyEscape)
	next := *old
	next.Inventory = old.Inventory
	next.Inventory.Comparison.Metadata.Identity.Number++
	m.openReviewTab(&next)
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 'b')
	namedKey(m, tea.KeyEscape)
	m.activateTab(0)
	if m.ReviewForm == nil || m.ReviewForm.Body != "a" {
		t.Fatal("tab switch lost draft")
	}
	changed := *old
	changed.Inventory = old.Inventory
	changed.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
	m.Update(ActionResult{Session: &changed, Reset: true})
	if m.ReviewForm != nil || len(m.Pending) != 0 {
		t.Fatal("draft retargeted to changed head")
	}
	reopened := draftTestModel(t, store)
	if reopened.ReviewForm == nil || reopened.ReviewForm.Body != "a" {
		t.Fatal("old comparison draft lost")
	}
}
func TestDraftStorageConflictBlocksWriteAndPreservesRemoteWriter(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	one := draftTestModel(t, store)
	two := draftTestModel(t, store)
	one.openReviewForm()
	one.ReviewForm.Focus = 1
	key(one, 'a')
	two.openReviewForm()
	two.ReviewForm.Focus = 1
	key(two, 'b')
	if two.draft.err == nil {
		t.Fatal("stale writer accepted")
	}
	if two.prepareDraftAttempt("review") {
		t.Fatal("storage conflict allowed remote dispatch")
	}
	got, err := store.LoadDraft(context.Background(), session.DraftKeyFor(one.Session.Inventory.Comparison.Metadata))
	if err != nil || got.Summary != "a" {
		t.Fatal("stale writer overwrote work")
	}
}

func TestHistoricalDraftRecoversSameFrozenProvenance(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := commitModel(t)
	m.Session.ID = strings.Repeat("d", 32)
	m.SetLifecycle(store, nil, nil)
	m.Session.Inventory.Comparison.Metadata.BaseRepository = "example/review"
	m.Session.Inventory.Comparison.Metadata.HeadRepository = "example/review"
	m.Session.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("c", 40)
	m.Session.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("b", 40)
	m.loadDraft(m.reviewTabState)
	target := source.ReviewCommentTarget{Identity: m.Session.Inventory.Comparison.Metadata.Identity, CommitID: m.commitEntries()[0].SHA, Path: "changed.go", Side: "RIGHT", Line: 1}
	m.Composer = &commentComposer{Target: target, CommitSHA: target.CommitID, CommitBundle: m.Session.Commits, CommitInventory: &m.Session.Inventory, Draft: "historical work", PendingIndex: -1}
	if !m.persistDraft(m.reviewTabState) {
		t.Fatal(m.ActionError)
	}
	reopened := New(context.Background(), nil)
	reopened.SetLifecycle(store, nil, nil)
	reopened.openReviewTab(m.Session)
	if reopened.Composer == nil || reopened.Composer.CommitSHA != target.CommitID || reopened.Composer.Draft != "historical work" || reopened.Composer.CommitBundle != m.Session.Commits || reopened.Composer.CommitInventory != &m.Session.Inventory {
		t.Fatal("historical draft lost provenance")
	}
	if !strings.Contains(reopened.View().Content, "historical work") {
		t.Fatal("recovery text invisible")
	}
}
func TestProgramDraftSummaryRecoversAndSaveQuitRetainsIt(t *testing.T) {
	store, saved := programStore(t)
	open := func() *programDriver {
		m := New(context.Background(), func(context.Context, func(string)) (*session.Record, error) { return store.Load(saved.ID) })
		m.SetLifecycle(store, nil, nil)
		return runProgram(t, m)
	}
	h := open()
	h.expect("loaded", func(f programFrame) bool { return f.sessionID == saved.ID && !f.busy })
	h.key('R')
	h.key(tea.KeyEnter)
	h.key('x')
	h.expect("summary edited", func(f programFrame) bool { return strings.Contains(f.text, "x▏") })
	h.key(tea.KeyEscape)
	h.key('q')
	h.expect("quit choice", func(f programFrame) bool { return f.page == pageQuitPending })
	h.key('s')
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		t.Fatal("save quit failed")
	}
	h = open()
	h.expect("recovered summary", func(f programFrame) bool {
		return f.page == pageDraftRecovery && strings.Contains(f.text, "Review summary: x")
	})
	h.quit()
}

func TestReplyAttemptFreezesAssociatedRootAnchor(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	display := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	raw := display
	raw.CommitID = strings.Repeat("c", 40)
	m.Comments = []source.ReviewComment{{ID: 7, Target: display, CurrentAnchor: &raw}}
	m.CommentMenu = &commentActionMenu{CommentID: 9, ReplyToID: 7, Target: display, Draft: "reply", mode: commentActionReply}
	if !m.prepareDraftAttempt("reply") {
		t.Fatal(m.ActionError)
	}
	if m.draft.attempted.Comment.Target != raw || m.draft.attempted.ParentID != 7 {
		t.Fatal("attempt stored display anchor")
	}
	restarted := draftTestModel(t, store)
	if restarted.draft.attempted.Comment.Target != raw || restarted.CommentMenu.Target != display {
		t.Fatal("restart conflated raw and display anchors")
	}
}

func TestUnsentReplyRecoversRawRootWithoutOverlayAndBlocksUnknownRoot(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	display := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	raw := display
	raw.CommitID = strings.Repeat("c", 40)
	m.Comments = []source.ReviewComment{{ID: 7, Target: display, CurrentAnchor: &raw}}
	m.CommentMenu = &commentActionMenu{CommentID: 9, ReplyToID: 7, Target: display, Draft: "unsent", mode: commentActionReply}
	m.resolveReplyRoot(m.CommentMenu)
	if !m.persistDraft(m.reviewTabState) {
		t.Fatal(m.ActionError)
	}
	restarted := draftTestModel(t, store)
	if len(restarted.Comments) != 0 || restarted.CommentMenu.RootAnchor == nil || *restarted.CommentMenu.RootAnchor != raw {
		t.Fatal("unsent reply lost raw root")
	}
	if !restarted.prepareDraftAttempt("reply") || restarted.draft.attempted.Comment.Target != raw {
		t.Fatal("no-overlay attempt used display anchor")
	}
	action := restarted.commentActionRequest(restarted.CommentMenu)
	if action.Comment.CurrentAnchor == nil || *action.Comment.CurrentAnchor != raw || action.Comment.ID != 7 {
		t.Fatal("dispatch differs from frozen intent")
	}
	// Legacy/unresolved editors cannot guess an associated SHA or dispatch.
	restarted.draft.attempt = ""
	restarted.draft.attempted = nil
	restarted.CommentMenu.RootAnchor = nil
	if restarted.prepareDraftAttempt("reply") {
		t.Fatal("unresolved root allowed dispatch")
	}
}
