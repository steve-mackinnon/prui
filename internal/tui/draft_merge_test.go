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
