package tui

import (
	tea "charm.land/bubbletea/v2"
	"testing"
)

func TestMouseResizeGrabOffsetAndBounds(t *testing.T) {
	for _, width := range []int{100, 101, 160, 201} {
		m := largeModel(largeTextSession(1, 1), width, 30)
		d := m.workspaceGeometry().Divider
		original := m.listWidth()
		_, handled := m.mouseResize(tea.MouseClickMsg{X: d.Min.X + 2, Y: d.Min.Y, Button: tea.MouseLeft})
		if !handled {
			t.Fatal("divider press not consumed")
		}
		m.mouseResize(tea.MouseMotionMsg{X: d.Min.X + 7, Y: d.Min.Y, Button: tea.MouseLeft})
		if m.listWidth() != original+5 {
			t.Fatalf("width %d: grab offset lost: got %d want %d", width, m.listWidth(), original+5)
		}
		m.mouseResize(tea.MouseMotionMsg{X: 0, Y: d.Min.Y, Button: tea.MouseLeft})
		if m.listWidth() != 20 {
			t.Fatalf("minimum width = %d", m.listWidth())
		}
		m.mouseResize(tea.MouseMotionMsg{X: width - 1, Y: d.Min.Y, Button: tea.MouseLeft})
		if m.listWidth() != min((width-3)/2, width-43) {
			t.Fatalf("maximum width = %d", m.listWidth())
		}
		_, handled = m.mouseResize(tea.MouseReleaseMsg{X: 0, Y: 0, Button: tea.MouseLeft})
		if !handled {
			t.Fatal("release not consumed")
		}
		saved := m.listWidth()
		m.mouseResize(tea.MouseMotionMsg{X: 25, Y: d.Min.Y, Button: tea.MouseLeft})
		if m.listWidth() != saved {
			t.Fatal("motion after release resized")
		}
	}
}

func TestMouseResizeCancellation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event tea.MouseMsg
	}{
		{"lost left", tea.MouseMotionMsg{X: 45, Y: 10, Button: tea.MouseNone}},
		{"other button", tea.MouseMotionMsg{X: 45, Y: 10, Button: tea.MouseRight}},
		{"fresh press", tea.MouseClickMsg{X: 0, Y: 10, Button: tea.MouseLeft}},
		{"outside terminal", tea.MouseMotionMsg{X: -1, Y: 10, Button: tea.MouseLeft}},
		{"modifier", tea.MouseMotionMsg{X: 45, Y: 10, Button: tea.MouseLeft, Mod: tea.ModShift}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := largeModel(largeTextSession(1, 1), 160, 30)
			d := m.workspaceGeometry().Divider
			m.mouseResize(tea.MouseClickMsg{X: d.Min.X, Y: d.Min.Y, Button: tea.MouseLeft})
			_, handled := m.mouseResize(tc.event)
			if !handled {
				t.Fatal("cancellation must consume stale gesture")
			}
			m.mouseResize(tea.MouseMotionMsg{X: 45, Y: 10, Button: tea.MouseLeft})
			if m.listWidth() != 36 {
				t.Fatal("cancelled drag still changes width")
			}
		})
	}
}

func TestMouseResizePreservesSelectionAndRestartsPath(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 160, 15)
	m.Files = true
	m.Focus = paneDiff
	m.cursorInViewport(1)
	m.cursorActive = true
	before, comment := m.cursorAnchor()
	if before == nil {
		t.Fatal("fixture needs commentable target")
	}
	selected, row, focus := m.Selected, m.Row, m.Focus
	generation := m.guidePathGeneration
	d := m.workspaceGeometry().Divider
	m.mouseResize(tea.MouseClickMsg{X: d.Min.X, Y: d.Min.Y, Button: tea.MouseLeft})
	m.mouseResize(tea.MouseMotionMsg{X: 70, Y: d.Min.Y, Button: tea.MouseLeft})
	after, afterComment := m.cursorAnchor()
	if after == nil || *before != *after || comment != afterComment {
		t.Fatal("resize changed semantic anchor")
	}
	if m.Selected != selected || m.Row != row || m.Focus != focus {
		t.Fatal("resize changed selection or focus")
	}
	if m.guidePathGeneration != generation+1 {
		t.Fatal("path scroll not restarted")
	}
	if m.offset() < 0 || m.offset() != m.clampOffset(m.offset()) {
		t.Fatal("invalid detail offset")
	}
}

func TestMouseResizeNarrowAndContextCancel(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 160, 30)
	m.tabs = []workspaceTab{{}}
	m.activeTab = 0
	d := m.workspaceGeometry().Divider
	m.mouseResize(tea.MouseClickMsg{X: d.Min.X, Y: d.Min.Y, Button: tea.MouseLeft})
	m.mouseResize(tea.MouseMotionMsg{X: 55, Y: d.Min.Y, Button: tea.MouseLeft})
	for _, narrow := range []bool{true, false} {
		if narrow {
			m.Width = 99
		} else {
			m.Width = 160
			m.ContextView = viewDescription
		}
		m.mouseResize(tea.MouseClickMsg{X: 55, Y: d.Min.Y, Button: tea.MouseLeft})
		m.mouseResize(tea.MouseMotionMsg{X: 25, Y: d.Min.Y, Button: tea.MouseLeft})
		if m.railWidth != 55 {
			t.Fatal("hidden divider changes preference")
		}
	}
	m.ContextView = viewChanges
	if m.listWidth() != 55 {
		t.Fatal("width preference lost across narrow/context view")
	}
}
