package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/session"
	"prui/internal/source"
	"testing"
	"time"
)

func TestMouseDoubleClickSemanticIdentityAndTimeout(t *testing.T) {
	m := New(context.Background(), nil)
	now := time.Now()
	if m.mouseActivateAt("one", now) || m.mouseActivateAt("two", now.Add(time.Millisecond)) {
		t.Fatal("different rows activated")
	}
	if !m.mouseActivateAt("two", now.Add(2*time.Millisecond)) {
		t.Fatal("same row did not activate")
	}
	if m.mouseActivateAt("two", now.Add(time.Second)) {
		t.Fatal("third click activated")
	}
	if m.mouseActivateAt("two", now.Add(2*time.Second)) {
		t.Fatal("expired click activated")
	}
	m.resetMouseActivation()
	if m.mouseActivateAt("two", now.Add(2*time.Second)) {
		t.Fatal("reset click activated")
	}
}

func TestMouseDoubleClickRepositoryOpensThroughHandler(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 80, 20
	m.Stack = []page{pageRepositoryPicker}
	m.Repositories = []session.Repository{{Repository: "owner/repo"}}
	m.listPullRequests = func(context.Context, string) ([]source.PullRequest, error) { return nil, nil }
	click := tea.MouseClickMsg{X: 2, Y: 2, Button: tea.MouseLeft}
	_, cmd := m.Update(click)
	if cmd != nil || m.Busy {
		t.Fatal("first click opened repository")
	}
	m.Update(tea.MouseReleaseMsg{X: 2, Y: 2, Button: tea.MouseLeft})
	_, cmd = m.Update(click)
	if cmd == nil || !m.Busy {
		t.Fatal("double click did not open repository")
	}
}

func TestMouseDoubleClickKeyAndWheelReset(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 80, 20
	m.Stack = []page{pageRepositoryPicker}
	m.Repositories = []session.Repository{{Repository: "a"}, {Repository: "b"}}
	m.mouseActivate("row")
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.mouseActivate("row") {
		t.Fatal("key preserved activation")
	}
	m.mouseWheel(tea.MouseWheelMsg{X: 2, Y: 2, Button: tea.MouseWheelDown})
	if m.mouseActivate("row") {
		t.Fatal("wheel preserved activation")
	}
}

func TestMouseDoubleClickUnrelatedPressResets(t *testing.T) {
	for _, intervening := range []tea.MouseClickMsg{{X: 0, Y: 0, Button: tea.MouseLeft}, {X: 2, Y: 2, Button: tea.MouseRight}, {X: -1, Y: 2, Button: tea.MouseLeft}} {
		m := New(context.Background(), nil)
		m.Width, m.Height = 80, 20
		m.Stack = []page{pageRepositoryPicker}
		m.Repositories = []session.Repository{{Repository: "owner/repo"}}
		m.listPullRequests = func(context.Context, string) ([]source.PullRequest, error) { return nil, nil }
		click := tea.MouseClickMsg{X: 2, Y: 2, Button: tea.MouseLeft}
		m.Update(click)
		m.Update(intervening)
		_, cmd := m.Update(click)
		if cmd != nil || m.Busy {
			t.Fatalf("unrelated press %#v preserved activation", intervening)
		}
	}
}
