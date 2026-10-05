package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/session"
	"prui/internal/source"
)

func TestDraftPersistenceAndPRCarouselShareUpdateWrapper(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 'x')
	namedKey(m, tea.KeyEscape)
	m.Stack = []page{pagePullRequestPicker}
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Repository: "owner/repo", Number: 7}, Title: strings.Repeat("Long title ", 20), Author: "alice"}}
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if cmd == nil || m.prCarousel.generation == 0 {
		t.Fatal("draft persistence bypassed picker carousel orchestration")
	}
	recovered, err := store.LoadDraft(context.Background(), session.DraftKeyFor(m.Session.Inventory.Comparison.Metadata))
	if err != nil || recovered.Summary != "x" {
		t.Fatal("carousel update lost durable review summary", err)
	}
}

func TestNewComparisonCancelsCommitFilterAndRetainsPreviousDraft(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	old := m.Session
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 'x')
	namedKey(m, tea.KeyEscape)
	filterCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.commitFilter = commitFilterState{subset: true, cancel: cancel, generation: 2}
	changed := *old
	changed.Inventory = old.Inventory
	changed.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
	m.Update(ActionResult{Session: &changed, Reset: true})
	if filterCtx.Err() != context.Canceled || m.commitFilter.subset || m.commitFilter.generation != 0 {
		t.Fatal("new comparison kept previous net-filter operation")
	}
	if m.ReviewForm != nil || m.Session != &changed {
		t.Fatal("old draft retargeted to new comparison")
	}
	recovered, err := store.LoadDraft(context.Background(), session.DraftKeyFor(old.Inventory.Comparison.Metadata))
	if err != nil || recovered.Summary != "x" {
		t.Fatal("filter reset lost previous comparison draft", err)
	}
}

func TestDurableReviewSurvivesUncertainGeneralConversation(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	m.openReviewForm()
	m.ReviewForm.Focus = 1
	key(m, 's')
	namedKey(m, tea.KeyEscape)

	calls := 0
	m.SetGeneralCommentSubmitter(func(_ context.Context, _ source.Metadata, body string) (source.ConversationEvent, error) {
		calls++
		if body != "tq" {
			t.Fatalf("general editor input was intercepted: %q", body)
		}
		return source.ConversationEvent{}, source.ErrCommentDeliveryUnknown
	})
	m.openDiscussions()
	m.discussionKey("n")
	key(m, 't')
	key(m, 'q')
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("explicit general comment submit did not dispatch")
	}
	m.Update(cmd())
	if calls != 1 || m.discussions.editor == nil || !m.discussions.editor.uncertain {
		t.Fatal("unknown general delivery lost its live attempted body")
	}
	_, retry := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if retry != nil || calls != 1 {
		t.Fatal("general delivery retried before reconciliation")
	}
	key(m, 'x')
	if m.discussions.editor.draft != "tqx" || m.discussions.editor.attemptedBody != "tq" {
		t.Fatal("editing general text changed its immutable attempt")
	}
	got, err := store.LoadDraft(context.Background(), session.DraftKeyFor(m.Session.Inventory.Comparison.Metadata))
	if err != nil || got.Summary != "s" || got.Attempt != "" {
		t.Fatal("general attempt replaced or contaminated durable review draft", err)
	}

	restarted := draftTestModel(t, store)
	defer restarted.Close()
	if restarted.ReviewForm == nil || restarted.ReviewForm.Body != "s" || restarted.top() != pageDraftRecovery {
		t.Fatal("durable review did not recover after general conversation")
	}
	if restarted.discussions.editor == nil || restarted.discussions.editor.draft != "tqx" || restarted.discussions.editor.cursor != 3 || restarted.discussions.editor.replyTo != "" || restarted.discussions.editor.matched || restarted.discussions.editor.attemptedBody != "tq" || !restarted.discussions.editor.uncertain || len(restarted.discussions.snapshot.Snapshot.Events) != 0 {
		t.Fatal("local general draft recovery lost immutable evidence or fetched events leaked")
	}
	namedKey(restarted, tea.KeyEnter)
	restarted.openReviewForm()
	if restarted.top() != pageReviewSubmit || restarted.ReviewForm.Body != "s" || calls != 1 {
		t.Fatal("review recovery lost focus/text or wrote to GitHub")
	}
}
