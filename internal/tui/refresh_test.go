package tui

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
)

func TestRefreshPRLoadsChangedComparisonAndPreservesOldDraft(t *testing.T) {
	for _, shortcut := range []rune{'r', 'c', 'N'} {
		t.Run(string(shortcut), func(t *testing.T) {
			store, saved := programStore(t)
			m := newModel(context.Background())
			defer m.Close()
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(saved)
			m.openReviewForm()
			m.ReviewForm.Focus = 1
			key(m, 'a')
			namedKey(m, tea.KeyEscape) // Escape retains the summary in its original comparison.
			changed := saved.Snapshot
			changed.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("f", 40)
			fresh, err := store.Create(changed)
			if err != nil {
				t.Fatal(err)
			}
			m.SetLifecycle(store, fakeGitHub{fresh.Inventory.Comparison.Metadata}, func(context.Context, *review.Session, func(string)) (*review.Session, error) {
				fresh.RevisionStatus = session.Current
				return fresh, nil
			})
			refreshKey(t, m, shortcut)
			if m.Session.ID != fresh.ID || m.Session.RevisionStatus != session.Current || m.ReviewForm != nil {
				t.Fatal("refresh did not load current source with an independent draft", m.ActionError)
			}
			oldDraft, err := store.LoadDraft(context.Background(), session.DraftKeyFor(saved.Inventory.Comparison.Metadata))
			if err != nil || oldDraft.Summary != "a" {
				t.Fatal("refresh lost original summary", err)
			}
		})
	}
}

func TestRefreshPRUnchangedPreservesPositionAndLoadsConversation(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.SetLifecycle(store, fakeGitHub{saved.Inventory.Comparison.Metadata}, func(context.Context, *review.Session, func(string)) (*review.Session, error) {
		t.Fatal("unchanged refresh rebuilt source")
		return nil, nil
	})
	m.openReviewTab(saved)
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	m.Scroll[0] = 3
	m.ReviewForm = &reviewForm{Body: "keep summary"}
	commentsRead := false
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		return DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Events: []source.ConversationEvent{{ID: "42", Body: "new comment", Kind: "PR comment"}}}}, nil
	})
	m.SetCommentReader(func(context.Context, source.Metadata) ([]source.ReviewComment, error) {
		commentsRead = true
		return nil, nil
	})
	refreshKey(t, m, 'r')
	if m.Session.ID != saved.ID || m.Session.RevisionStatus != session.Current || m.Focus != paneDiff || m.Scroll[0] != 3 || m.ReviewForm == nil || m.ReviewForm.Body != "keep summary" {
		t.Fatal("unchanged refresh moved reading position or lost summary")
	}
	if !m.discussions.loaded || len(m.discussions.snapshot.Snapshot.Events) != 1 || m.discussions.snapshot.Snapshot.Events[0].Body != "new comment" || !commentsRead {
		t.Fatal("refresh did not load discussions and inline comments")
	}
}

func TestActionsExposeOnePRRefresh(t *testing.T) {
	m := actionsModel(t)
	count := 0
	for _, action := range m.reviewActions() {
		if strings.Contains(action.label, "Refresh") || action.label == "New comparison" {
			count++
			if action.label != "Refresh PR" || action.shortcut != "r" {
				t.Fatalf("separate refresh action remains: %s", action.label)
			}
		}
	}
	if count != 1 {
		t.Fatalf("got %d refresh actions, want one", count)
	}
}

func refreshKey(t *testing.T, m *Model, shortcut rune) {
	t.Helper()
	var drain func(tea.Cmd)
	drain = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				drain(child)
			}
			return
		}
		_, next := m.Update(msg)
		drain(next)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: shortcut, Text: string(shortcut)})
	drain(cmd)
	if m.Busy {
		t.Fatal("refresh left an unfinished operation")
	}
}

func enablePRRefresh(t *testing.T, m *Model) {
	t.Helper()
	store, _ := programStore(t)
	snapshot := m.Session.Snapshot
	snapshot.Inventory.Comparison.Metadata.BaseRepository = snapshot.Inventory.Comparison.Metadata.Identity.Repository
	snapshot.Inventory.Comparison.Metadata.HeadRepository = snapshot.Inventory.Comparison.Metadata.Identity.Repository
	snapshot.Inventory.Comparison.Metadata.BaseSHA = strings.Repeat("c", 40)
	snapshot.Commits = nil
	snapshot.Inventory.Units = slices.Clone(snapshot.Inventory.Units)
	snapshot.Inventory.Patches = make(map[string][]byte)
	for i := range snapshot.Inventory.Units {
		u := &snapshot.Inventory.Units[i]
		if u.PatchReference != "" {
			patch := m.Session.Inventory.Patches[u.PatchReference]
			u.PatchReference = fmt.Sprintf("%x", sha256.Sum256(patch))
			snapshot.Inventory.Patches[u.PatchReference] = patch
		}
	}
	saved, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	saved.Commits = m.Session.Commits
	m.Session = saved
	m.SetLifecycle(store, fakeGitHub{saved.Inventory.Comparison.Metadata}, nil)
}

func TestRefreshPRSourceFailureKeepsOldComparisonAndDraft(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.openReviewTab(saved)
	m.ReviewForm = &reviewForm{Body: "keep summary"}
	changed := saved.Inventory.Comparison.Metadata
	changed.HeadSHA = strings.Repeat("f", 40)
	failure := errors.New("source fetch failed")
	m.SetLifecycle(store, fakeGitHub{changed}, func(context.Context, *review.Session, func(string)) (*review.Session, error) {
		return nil, failure
	})
	refreshKey(t, m, 'r')
	if m.Session.ID != saved.ID || m.Session.RevisionStatus != session.Stale || m.ReviewForm == nil || m.ReviewForm.Body != "keep summary" || !errors.Is(m.ActionError, failure) {
		t.Fatal("failed refresh lost source, draft, or failure status", m.ActionError)
	}
	reopened, err := store.Load(saved.ID)
	if err != nil || reopened.RevisionStatus != session.Stale {
		t.Fatal("failed fetch incorrectly persisted current source", err)
	}
}

func TestRefreshPRDraftSaveFailureBlocksReplacement(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.SetLifecycle(store, fakeGitHub{saved.Inventory.Comparison.Metadata}, nil)
	m.openReviewTab(saved)
	m.loadDraft(m.reviewTabState)
	m.ReviewForm = &reviewForm{Body: "unsaved summary"}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	refreshKey(t, m, 'r')
	if m.Session.ID != saved.ID || m.ReviewForm.Body != "unsaved summary" || m.ActionError == nil {
		t.Fatal("draft save failure did not retain in-memory work")
	}
}
