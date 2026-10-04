package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"prui/internal/review"
	"prui/internal/source"
)

func TestInboxGenerationFiltersAndMarkRead(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Width = 150
	m.Height = 25
	calls := 0
	refreshes := 0
	loader := func(_ context.Context, o source.InboxOptions, refresh bool) (source.Inbox, error) {
		calls++
		if refresh {
			refreshes++
		}
		return source.Inbox{Viewer: "me", Complete: true, Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: source.Identity{Repository: "o/other", Number: 2}, Title: "Other repo"}, Activity: "unknown"}}}, nil
	}
	marked := false
	m.SetInbox(loader, func(_ context.Context, viewer string, item source.InboxItem) error {
		marked = viewer == "me" && item.PullRequest.Identity.Repository == "o/other"
		return nil
	}, nil, source.InboxOptions{}, true, false)
	cmd := m.Init()
	m.Update(cmd())
	if calls != 1 || refreshes != 0 {
		t.Fatal("implicit refresh")
	}
	old := m.loadInbox(true)
	newer := m.loadInbox(false)
	m.Update(newer())
	m.Update(old())
	if refreshes != 1 || m.inbox.loading {
		t.Fatal("stale search applied")
	}
	m.Update(inboxResult{Generation: m.inbox.generation - 1, Data: source.Inbox{Viewer: "wrong"}})
	if m.inbox.data.Viewer != "me" {
		t.Fatal("stale account")
	}
	m.Update(m.inboxKey("m")())
	if !marked || m.inbox.data.Items[0].Activity != "read" {
		t.Fatal("mark read")
	}
	view := m.inboxView()
	if !strings.Contains(view, "o/other#2") || !strings.Contains(view, "alerts off") {
		t.Fatal(view)
	}
	m.inboxKey("f")
	m.inbox.field = 1
	m.inboxFilterKey("q")
	if m.inbox.draftOptions.Repository != "q" {
		t.Fatal("typing filter")
	}
	m.inboxFilterKey("esc")
	if m.inbox.options.Repository != "" {
		t.Fatal("discarded filter applied")
	}
}
func TestInboxOpenUsesItemIdentityWithoutCurrentCheckout(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.currentRepository = "o/first"
	m.currentCheckout = "/wrong"
	id := source.Identity{Repository: "o/second", Number: 2}
	opened := false
	m.SetInbox(nil, nil, func(_ context.Context, checkout string, got source.Identity, _ func(string)) (*review.Session, error) {
		opened = checkout == "" && got == id
		return nil, errors.New("missing own checkout")
	}, source.InboxOptions{}, true, false)
	m.inbox.data = source.Inbox{Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: id}}}}
	completeAction(t, m, m.inboxKey("enter"))
	if !opened || m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "missing own checkout") {
		t.Fatal("wrong repository reused")
	}
}
func TestInboxLateOpenCannotReplaceEscapedReview(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.inbox.generation = 3
	m.Update(inboxOpenResult{Generation: 2, Result: PullRequestOpenResult{Session: &review.Session{}}})
	if m.Session != nil {
		t.Fatal("late opening replaced session")
	}
}
