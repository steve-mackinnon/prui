package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"testing"
)

func TestMouseRoutingOwnsOnlySupportedSurfaces(t *testing.T) {
	for _, surface := range []page{pageHelp, pageGuideConsent, pageReviewSubmit, pageQuitPending, pageEvidence, pageURL} {
		m := New(context.Background(), nil)
		m.Session = largeSession(2, 2)
		m.Width, m.Height = 120, 24
		m.Stack = []page{surface}
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4})
		if m.Selected != 0 || m.View().MouseMode != tea.MouseModeNone {
			t.Fatalf("page %v accepts mouse", surface)
		}
		m.Close()
	}
	for _, block := range []string{"busy", "loading", "composer", "menu"} {
		m := New(context.Background(), nil)
		m.Session = largeSession(2, 2)
		m.Width, m.Height = 120, 24
		m.Files = true
		switch block {
		case "busy":
			m.Busy = true
		case "loading":
			m.Loading = true
		case "composer":
			m.Composer = &commentComposer{}
		case "menu":
			m.CommentMenu = &commentActionMenu{}
		}
		m.drag.active = true
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4})
		if m.Selected != 0 || m.drag.active || m.mouseAvailable() {
			t.Fatalf("%s failed gating", block)
		}
		m.Close()
	}
}

func TestMouseRouterSelectsWithoutActivationAndCancelsDrag(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 120, 24
	m.Files = true
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("capture disabled")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: 4})
	if m.Session.UnitFiles[m.Selected] != 1 || m.Composer != nil || m.Focus != paneList {
		t.Fatal("click did not select only")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 37, Y: 3})
	if !m.drag.active {
		t.Fatal("divider did not start drag")
	}
	m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.drag.active {
		t.Fatal("key did not cancel drag")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 37, Y: 3})
	m.Update(tea.WindowSizeMsg{Width: 99, Height: 24})
	if m.drag.active {
		t.Fatal("resize did not cancel drag")
	}
}
