package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
)

func TestGeneralDraftRestartAndImmutableDispatch(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	m.discussions.editor = &generalCommentEditor{draft: "original", replyTo: "PR comment:12", cursor: 3}
	m.discussions.snapshot.Snapshot.Events = []source.ConversationEvent{{ID: "PR comment:12", Kind: "PR comment", Body: "remote body never stored"}}
	m.SetGeneralCommentSubmitter(func(_ context.Context, _ source.Metadata, body string) (source.ConversationEvent, error) {
		recovered := draftTestModel(t, store)
		e := recovered.discussions.editor
		if e == nil || !e.uncertain || e.attemptedBody != body || e.replyTo != "PR comment:12" || e.cursor != 3 || !e.attemptedIDs["PR comment:12"] {
			t.Fatalf("not durable before dispatch: %#v", e)
		}
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	cmd := m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("no submission")
	}
	// Crash before dispatch retains intent without making a network write.
	recovered := draftTestModel(t, store)
	if !recovered.discussions.editor.uncertain {
		t.Fatal("interrupted post retriable")
	}
	m.applyGeneralCommentResult(cmd().(GeneralCommentResult))
	m.persistDrafts()
	recovered = draftTestModel(t, store)
	e := recovered.discussions.editor
	e.draft = "edited after timeout"
	e.cursor = 2
	if !recovered.persistDraft(recovered.reviewTabState) {
		t.Fatal(recovered.ActionError)
	}
	again := draftTestModel(t, store)
	if again.discussions.editor.attemptedBody != "original" || again.discussions.editor.draft != e.draft || !again.discussions.editor.uncertain {
		t.Fatal("immutable attempt lost")
	}
	if again.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("automatic retry")
	}
	if !again.unsentReviewDrafts() {
		t.Fatal("quit ignores general draft")
	}
	if len(again.discussions.snapshot.Snapshot.Events) != 0 {
		t.Fatal("persisted fetched events")
	}
	// Offline read failure cannot clear uncertainty.
	again.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		return DiscussionSnapshot{}, errors.New("offline")
	})
	// Explicit discard is durable.
	again.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	again.persistDrafts()
	if draftTestModel(t, store).discussions.editor != nil {
		t.Fatal("discard recovered")
	}
}

func TestGeneralDraftReconciliationSurvivesRestart(t *testing.T) {
	for _, tc := range []struct {
		name                                     string
		complete, verified, match, prior, failed bool
	}{
		{name: "partial", verified: true, match: true}, {name: "stale", complete: true, match: true}, {name: "offline", failed: true},
		{name: "match", complete: true, verified: true, match: true}, {name: "authoritative absence", complete: true, verified: true},
		{name: "prior identical is not this attempt", complete: true, verified: true, match: true, prior: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := session.Open(t.TempDir() + "/private")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m := draftTestModel(t, store)
			m.discussions.editor = &generalCommentEditor{draft: "edited", cursor: 2, attemptedBody: "original", attemptedIDs: map[string]bool{"PR comment:12": true}, uncertain: true}
			m.persistDrafts()
			m = draftTestModel(t, store)
			result := DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: tc.verified, Snapshot: source.DiscussionSnapshot{Complete: tc.complete, Timeline: true}}}
			if tc.match {
				id := "PR comment:13"
				if tc.prior {
					id = "PR comment:12"
				}
				result.Snapshot.Snapshot.Events = []source.ConversationEvent{{ID: id, Kind: "PR comment", Body: "original"}}
			}
			if tc.failed {
				result.Err = errors.New("offline")
			}
			m.applyDiscussionResult(result)
			m.persistDrafts()
			m = draftTestModel(t, store)
			e := m.discussions.editor
			authoritative := tc.complete && tc.verified && !tc.failed
			if e.uncertain == authoritative || e.matched != (authoritative && tc.match && !tc.prior) || e.attemptedBody != "original" {
				t.Fatalf("reconciliation: %#v", e)
			}
			if (!authoritative || e.matched) && m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
				t.Fatal("unsafe retry")
			}
		})
	}
}

func TestGeneralDraftComparisonIsolationAndSuccessCleanup(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	old := m.Session
	m.discussions.editor = &generalCommentEditor{draft: "old pin", cursor: 4, replyTo: "PR comment:12"}
	m.persistDrafts()
	changed := *old
	changed.Inventory = old.Inventory
	changed.Inventory.Comparison.Metadata.HeadSHA = "cccccccccccccccccccccccccccccccccccccccc"
	state := m.newDraftReviewTab(&changed)
	if state.discussions.editor != nil {
		t.Fatal("silently retargeted")
	}
	other := *old
	other.Inventory = old.Inventory
	other.Inventory.Comparison.Metadata.Identity.Number++
	if m.newDraftReviewTab(&other).discussions.editor != nil {
		t.Fatal("cross PR draft")
	}
	restored := draftTestModel(t, store)
	if restored.discussions.editor.draft != "old pin" {
		t.Fatal("old work lost")
	}
	restored.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{ID: "PR comment:14", Kind: "PR comment", Body: "old pin"}, nil
	})
	cmd := restored.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit missing")
	}
	restored.applyGeneralCommentResult(cmd().(GeneralCommentResult))
	restored.persistDrafts()
	if draftTestModel(t, store).discussions.editor != nil {
		t.Fatal("success draft recovered")
	}
}

func TestGeneralDraftRejectsPreAttemptAndInlineOnlyReconciliation(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	m.discussions.editor = &generalCommentEditor{draft: "original", cursor: 3}
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	before := DiscussionResult{Target: m.activeTab, Session: m.Session, Generation: m.discussions.generation, Snapshot: DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true}}}
	cmd := m.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("submit missing")
	}
	m.applyGeneralCommentResult(cmd().(GeneralCommentResult))
	m.applyDiscussionResult(before)
	if !m.discussions.editor.uncertain {
		t.Fatal("pre-attempt read authorized retry")
	}
	inline := before
	inline.Generation = m.discussions.generation
	inline.Snapshot.Snapshot.Timeline = false
	m.applyDiscussionResult(inline)
	m.persistDrafts()
	if !draftTestModel(t, store).discussions.editor.uncertain {
		t.Fatal("inline-only read proved general absence")
	}
}

func TestGeneralDraftSaveConflictBlocksDispatch(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := draftTestModel(t, store)
	stale := draftTestModel(t, store)
	first.discussions.editor = &generalCommentEditor{draft: "preserved", cursor: 2}
	first.persistDrafts()
	stale.discussions.editor = &generalCommentEditor{draft: "stale", cursor: 1}
	called := false
	stale.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		called = true
		return source.ConversationEvent{}, nil
	})
	if stale.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil || called {
		t.Fatal("dispatch before successful atomic save")
	}
	if draftTestModel(t, store).discussions.editor.draft != "preserved" {
		t.Fatal("stale writer overwrote work")
	}
}

func TestProgramGeneralDraftRestartImmutableReconcileAndDiscard(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	meta := m.Session.Inventory.Comparison.Metadata
	var calls atomic.Int32
	m.SetGeneralCommentSubmitter(func(_ context.Context, _ source.Metadata, body string) (source.ConversationEvent, error) {
		calls.Add(1)
		d, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
		if err != nil || d.General == nil || !d.General.Uncertain || d.General.Body != body || d.General.AttemptedBody != body {
			return source.ConversationEvent{}, errors.New("durable intent missing at dispatch")
		}
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	h := runProgram(t, m)
	h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.loading })
	h.key('D')
	h.key('n')
	h.key('t')
	h.key('q')
	h.key(tea.KeyEnter)
	h.expect("uncertain", func(f programFrame) bool { return strings.Contains(f.text, source.ErrCommentDeliveryUnknown.Error()) })
	// Stop without quit/discard; a new event loop must recover without dispatch.
	h.p.Kill()
	<-h.done
	restarted := draftTestModel(t, store)
	restarted.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		calls.Add(1)
		return source.ConversationEvent{}, errors.New("unexpected retry")
	})
	restarted.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		return DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "PR comment:77", Kind: "PR comment", Body: "tq"}}}}, nil
	})
	h2 := runProgram(t, restarted)
	h2.expect("recovery", func(f programFrame) bool { return f.page == pageDraftRecovery && strings.Contains(f.text, "tq") })
	h2.key(tea.KeyEnter)
	h2.key('x')
	h2.expect("edited recovery", func(f programFrame) bool { return strings.Contains(f.text, "tqx") })
	h2.p.Send(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	h2.expect("matched original", func(f programFrame) bool { return strings.Contains(f.text, "Matching attempted PR comment found") })
	h2.key(tea.KeyEnter)
	h2.expect("retry blocked", func(f programFrame) bool {
		return strings.Contains(f.text, "inspect it before starting another comment")
	})
	d, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || d.General == nil || d.General.Body != "tqx" || d.General.AttemptedBody != "tq" || !d.General.Matched || calls.Load() != 1 {
		t.Fatal("program changed immutable attempt or repeated dispatch", d, err, calls.Load())
	}
	h2.key(tea.KeyEscape)
	h2.expect("discard", func(f programFrame) bool { return !strings.Contains(f.text, "Enter: post") })
	h2.quit()
	if d, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta)); err != nil || d.General != nil {
		t.Fatal("program discard not durable", d, err)
	}
}

func TestGeneralDraftComparisonResetClearsPreparedSuggestionAndPreservesOriginalAttempts(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	old := m.Session
	meta := old.Inventory.Comparison.Metadata
	body, _ := source.SuggestionBody("replacement")
	a := source.SuggestionApplication{Metadata: meta, Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}, CommentID: 9, Branch: "feature", Before: "old", Replacement: "replacement", Content: "replacement\n", CommentBody: body}
	input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": "text", "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
	a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
	m.SuggestionApply = &a
	m.SuggestionConfirm = true
	m.SuggestionScroll = 7
	m.discussions.editor = &generalCommentEditor{draft: "edited general", cursor: 3, attemptedBody: "original general", uncertain: true}
	m.ReviewForm = &reviewForm{Body: "summary"}
	if !m.prepareDraftAttempt("suggestion") {
		t.Fatal(m.ActionError)
	}
	readinessCtx, cancelReadiness := context.WithCancel(context.Background())
	defer cancelReadiness()
	m.readiness = readinessState{cancel: cancelReadiness, loaded: true, snapshot: source.Readiness{Identity: meta.Identity, HeadSHA: meta.HeadSHA}}
	searchCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.search[0] = &diffSearchState{cancel: cancel}
	changed := *old
	changed.Inventory = old.Inventory
	changed.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
	m.Update(ActionResult{Session: &changed, Reset: true})
	if m.Session != &changed || m.SuggestionApply != nil || m.SuggestionConfirm || m.SuggestionScroll != 0 || m.discussions.editor != nil || m.ReviewForm != nil || m.draft.attempt != "" || searchCtx.Err() != context.Canceled || readinessCtx.Err() != context.Canceled || m.readiness.loaded || m.readiness.snapshot.HeadSHA != "" {
		t.Fatal("comparison reset carried private intent or lost search cancellation")
	}
	recovered := draftTestModel(t, store)
	defer recovered.Close()
	if recovered.SuggestionApply == nil || recovered.SuggestionApply.Target != a.Target || recovered.draft.attempted == nil || string(recovered.draft.attempted.Application.Payload) != string(a.Payload) || recovered.discussions.editor == nil || recovered.discussions.editor.attemptedBody != "original general" || !recovered.discussions.editor.uncertain || recovered.ReviewForm == nil || recovered.ReviewForm.Body != "summary" {
		t.Fatal("old comparison lost immutable request or drafts")
	}
	if recovered.suggestionApplyKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil || recovered.generalCommentKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("old uncertain work retried")
	}
}

func TestGeneralDraftRejectedInputRemainsEditableAndDiscardable(t *testing.T) {
	for _, kind := range []string{"observed IDs", "text overflow", "newline overflow"} {
		t.Run(kind, func(t *testing.T) {
			store, err := session.Open(t.TempDir() + "/private")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m := draftTestModel(t, store)
			defer m.Close()
			m.openDiscussions()
			m.discussions.editor = &generalCommentEditor{draft: "saved", cursor: 5}
			calls := 0
			m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
				calls++
				return source.ConversationEvent{}, nil
			})
			if kind == "observed IDs" {
				for i := 0; i < 501; i++ {
					m.discussions.snapshot.Snapshot.Events = append(m.discussions.snapshot.Snapshot.Events, source.ConversationEvent{ID: fmt.Sprintf("PR comment:%d", i+1), Kind: "PR comment"})
				}
			} else {
				m.discussions.editor.draft = strings.Repeat("x", 65536)
				m.discussions.editor.cursor = 65536
			}
			m.persistDrafts()
			before := draftContent(m.reviewTabState)
			var cmd tea.Cmd
			switch kind {
			case "observed IDs":
				_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			case "text overflow":
				_, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
			case "newline overflow":
				_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
			}
			if cmd != nil || calls != 0 || m.draft.err != nil || m.discussions.editor.draft != before.General.Body || m.discussions.editor.attemptedBody != before.General.AttemptedBody || len(m.discussions.editor.attemptedIDs) != 0 {
				t.Fatal("rejected input poisoned private draft", m.draft.err)
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
			if m.draft.err != nil || len(m.discussions.editor.draft) != len(before.General.Body)-1 {
				t.Fatal("rejected editor cannot recover")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			if d, err := store.LoadDraft(context.Background(), session.DraftKeyFor(m.Session.Inventory.Comparison.Metadata)); err != nil || d.General != nil {
				t.Fatal("rejected editor cannot discard durably", d, err)
			}
		})
	}
}

func TestGeneralDraftReadinessAndCommitFilterOwnTheirKeysWithoutLeakingLiveEvidence(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	m.Loading = false
	m.selectReviewView(viewFiles)
	frozen := m.Session
	meta := frozen.Inventory.Comparison.Metadata
	reads, writes := 0, 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		reads++
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("c", 40), HeadVerified: true}, nil
	})
	m.SetGeneralCommentSubmitter(func(context.Context, source.Metadata, string) (source.ConversationEvent, error) {
		writes++
		return source.ConversationEvent{}, errors.New("unexpected write")
	})
	m.ReviewForm = &reviewForm{Body: "private summary"}
	m.openDiscussions()
	m.discussions.editor = &generalCommentEditor{draft: "local", cursor: 5, replyTo: "PR comment:11", attemptedBody: "original", attemptedIDs: map[string]bool{"PR comment:11": true}, uncertain: true}
	m.persistDrafts()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if cmd != nil {
		t.Fatal("readiness intercepted general editor")
	}
	key(m, 'C')
	ctrlKey(m, 'p')
	d, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || d.Version != 4 || d.General == nil || d.General.Body != "localC" || d.General.Cursor != 6 || d.General.ReplyTo != "PR comment:11" || d.General.AttemptedBody != "original" || !d.General.Uncertain || d.Summary != "private summary" || reads != 0 || writes != 0 || m.commitFilter.open || m.Composer != nil || len(m.Pending) != 0 {
		t.Fatal("editor ownership or durable isolation changed", d, err)
	}
	namedKey(m, tea.KeyEscape)
	namedKey(m, tea.KeyEscape)
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if cmd == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R readiness lost")
	}
	m.Update(cmd())
	if !m.readiness.loaded || reads != 1 || writes != 0 || m.Session != frozen || m.discussions.editor != nil {
		t.Fatal("live head changed frozen draft state")
	}
	d, err = store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || d.General != nil || d.Summary != "private summary" || d.Generation == 0 {
		t.Fatal("readiness contaminated private data", d, err)
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'C')
	if !m.commitFilter.open || m.top() != pageReview || m.Session != frozen || writes != 0 || reads != 1 {
		t.Fatal("C filter lost key ownership")
	}
	namedKey(m, tea.KeyEscape)
}
