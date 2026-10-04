package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
)

func publishedModel(t *testing.T) *Model {
	t.Helper()
	m := commitModel(t)
	yes, no := true, false
	m.Viewer = "alice"
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Timeline: true, Threads: []source.Discussion{{ID: "thread", Resolved: &no, Outdated: &yes, CanResolve: &yes, CanUnresolve: &no, Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: "old"}, {ID: 3, Author: "alice", Body: "reply", ParentID: 1}}}}, Events: []source.ConversationEvent{{ID: "PR comment:2", Kind: "PR comment", Author: "alice", Body: "general"}}}}
	m.SetPublishedSubmitter(func(context.Context, PublishedAction) (PublishedValue, error) { return PublishedValue{}, nil })
	return m
}
func TestPublishedActionsBothViewsAndCanonicalIdentity(t *testing.T) {
	for _, general := range []bool{false, true} {
		m := publishedModel(t)
		writes := 0
		m.SetPublishedSubmitter(func(_ context.Context, a PublishedAction) (PublishedValue, error) {
			writes++
			return PublishedValue{Comment: source.PublishedComment{ID: a.CommentID, Body: a.Body, Author: "alice"}}, nil
		})
		if general {
			m.openDiscussions()
			for i, entry := range m.discussionEntries() {
				if entry.ID == "PR comment:2" {
					m.discussions.selected = i
				}
			}
			m.discussions.detail = true
			m.discussions.selectedID = "PR comment:2"
			if m.discussionKey("e") != nil || m.discussions.published == nil {
				t.Fatal("general detail edit absent")
			}
		} else {
			m.CommentMenu = &commentActionMenu{CommentID: 3, Author: "alice"}
			m.commentActionKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
			if m.CommentMenu != nil || m.discussions.published == nil {
				t.Fatal("inline menu edit absent")
			}
		}
		e := m.discussions.published
		e.draft = "edited"
		e.cursor = len(e.draft)
		cmd := m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		m.Update(cmd())
		if writes != 1 || m.discussions.published != nil {
			t.Fatal("submit failed")
		}
		if general {
			if m.discussions.snapshot.Snapshot.Events[0].ID != "PR comment:2" || m.discussions.snapshot.Snapshot.Events[0].Body != "edited" {
				t.Fatal("general identity changed")
			}
		} else {
			c := m.discussions.snapshot.Snapshot.Threads[0].Comments[1]
			if c.ID != 3 || c.ParentID != 1 || c.Body != "edited" {
				t.Fatal(c)
			}
		}
	}
	for _, inline := range []bool{false, true} {
		m := publishedModel(t)
		writes := 0
		m.SetPublishedSubmitter(func(_ context.Context, a PublishedAction) (PublishedValue, error) {
			writes++
			return PublishedValue{Thread: source.Discussion{ID: a.ThreadID, Resolved: a.Resolve}}, nil
		})
		if inline {
			m.CommentMenu = &commentActionMenu{CommentID: 1}
			m.commentActionKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
		} else {
			m.openDiscussions()
			for i, entry := range m.discussionEntries() {
				if entry.ID == "inline:1" {
					m.discussions.selected = i
				}
			}
			m.discussions.detail = true
			m.discussionKey("z")
		}
		if m.discussions.published == nil || writes != 0 {
			t.Fatal("resolve missing or wrote before confirmation")
		}
		cmd := m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		m.Update(cmd())
		if writes != 1 || !*m.discussions.snapshot.Snapshot.Threads[0].Resolved || !*m.discussions.snapshot.Snapshot.Threads[0].Outdated {
			t.Fatal("canonical status missing")
		}
	}
}
func TestPublishedUnknownRetainsImmutableAttemptAndRequiresVerifiedRead(t *testing.T) {
	m := publishedModel(t)
	writes := 0
	m.SetPublishedSubmitter(func(context.Context, PublishedAction) (PublishedValue, error) {
		writes++
		return PublishedValue{}, source.ErrCommentDeliveryUnknown
	})
	if !m.openPublished(1, "", false) {
		t.Fatal("edit unavailable")
	}
	e := m.discussions.published
	e.draft = "attempted"
	cmd := m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	e.draft = "later edit"
	e.cursor = len(e.draft)
	if !e.uncertain || e.attempted.Body != "attempted" || m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil {
		t.Fatal("unknown write retried")
	}
	snap := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: false, Threads: []source.Discussion{{ID: "thread", Comments: []source.ReviewComment{{ID: 1, Body: "attempted"}}}}}}
	reconcilePublished(&m.discussions, snap)
	if !e.uncertain {
		t.Fatal("partial read reconciled")
	}
	snap.Snapshot.Complete = true
	snap.CurrentVerified = false
	reconcilePublished(&m.discussions, snap)
	if !e.uncertain {
		t.Fatal("stale read reconciled")
	}
	snap.CurrentVerified = true
	reconcilePublished(&m.discussions, snap)
	if e.uncertain || !e.matched || e.draft != "later edit" || m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEnter}) != nil || writes != 1 {
		t.Fatal("immutable attempt lost")
	}
	if m.mouseAvailable() || !m.unsentReviewDrafts() {
		t.Fatal("editor does not own input/drafts")
	}
	m.Width, m.Height = 25, 7
	for _, line := range strings.Split(m.View().Content, "\n") {
		if visibleWidth(line) > 25 {
			t.Fatal("editor exceeds viewport")
		}
	}
	m.publishedKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.discussions.published != nil {
		t.Fatal("escape failed")
	}
}
func TestPublishedFailureStaleResultAndConfirmedRefresh(t *testing.T) {
	m := publishedModel(t)
	m.openPublished(1, "", false)
	e := m.discussions.published
	e.draft = "edited"
	e.attempted = e.action
	e.attempted.Body = e.draft
	e.posting = true
	m.applyPublishedResult(PublishedResult{Target: m.activeTab, Session: m.Session, Editor: &publishedEditor{}, Err: errors.New("wrong result")})
	if !e.posting {
		t.Fatal("stale editor result applied")
	}
	m.applyPublishedResult(PublishedResult{Target: m.activeTab, Session: m.Session, Editor: e, Err: errors.New("denied")})
	if e.draft != "edited" || e.posting || e.uncertain {
		t.Fatal("failure lost draft")
	}
	m.applyPublishedResult(PublishedResult{Target: m.activeTab, Session: m.Session, Editor: e, Value: PublishedValue{Comment: source.PublishedComment{ID: 1, Body: "edited", Author: "alice"}}})
	snap := DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: false}}
	retainPublished(&m.discussions, &snap)
	if len(snap.Snapshot.Threads) != 1 || snap.Snapshot.Threads[0].Comments[0].Body != "edited" || snap.Snapshot.Complete || len(m.discussions.confirmedPublished) != 1 {
		t.Fatal("partial refresh lost confirmed identity")
	}
	snap = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", Comments: []source.ReviewComment{{ID: 1, Body: "external edit"}}}}}}
	retainPublished(&m.discussions, &snap)
	if len(m.discussions.confirmedPublished) != 0 || snap.Snapshot.Threads[0].Comments[0].Body != "external edit" {
		t.Fatal("newer canonical edit hidden")
	}
	yes, no := true, false
	a := PublishedAction{ThreadID: "thread", Resolve: &yes}
	applyPublishedValue(m.reviewStateForTarget(m.activeTab), a, PublishedValue{Thread: source.Discussion{ID: "thread", Resolved: &yes}})
	snap = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", Resolved: &no}}}}
	retainPublished(&m.discussions, &snap)
	if len(m.discussions.confirmedPublished) != 0 || *snap.Snapshot.Threads[0].Resolved {
		t.Fatal("newer reopen hidden")
	}
}
