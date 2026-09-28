package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func wheel(m *Model, x, y int, button tea.MouseButton) {
	m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
}

func TestMouseWheelRoutesFileListAndDiff(t *testing.T) {
	m := largeModel(largeTextSession(8, 24), 120, 10)
	defer m.Close()
	g := m.workspaceGeometry()
	wheel(m, g.Rail.Min.X, g.Rail.Min.Y, tea.MouseWheelDown)
	if got := m.Session.UnitFiles[m.Selected]; got != 3 {
		t.Fatalf("file wheel selected file %d, want 3", got)
	}
	selected := m.Selected
	before := m.offset()
	wheel(m, g.Detail.Min.X, g.Detail.Min.Y, tea.MouseWheelDown)
	if m.offset() <= before || m.Selected != selected {
		t.Fatalf("diff wheel did not scroll the detail independently: offset %d, selected %d", m.offset(), m.Selected)
	}
	wheel(m, g.Detail.Min.X, g.Detail.Min.Y, tea.MouseWheelUp)
	if m.offset() != before {
		t.Fatalf("up wheel left offset at %d, want %d", m.offset(), before)
	}
}

func TestMouseWheelRoutesGuideListAndNarrowVisiblePane(t *testing.T) {
	m := guideScrollModel(10, 120)
	defer m.Close()
	m.Row, m.Focus = 0, paneList
	g := m.workspaceGeometry()
	wheel(m, g.Rail.Min.X, g.Rail.Min.Y, tea.MouseWheelDown)
	if m.Row != 2 {
		t.Fatalf("guide wheel moved to row %d, want 2", m.Row)
	}
	row := m.Row
	before := m.offset()
	wheel(m, g.Detail.Min.X, g.Detail.Min.Y, tea.MouseWheelDown)
	if m.offset() <= before || m.Row != row {
		t.Fatalf("guide detail wheel changed row or failed to scroll: row %d, offset %d", m.Row, m.offset())
	}
	m.Width = 80
	m.Focus = paneList
	m.setOffset(0)
	before = m.offset()
	wheel(m, 10, 4, tea.MouseWheelUp)
	if m.Row >= row || m.offset() != before {
		t.Fatal("narrow list wheel did not use the visible list")
	}
	m.Focus = paneDiff
	row = m.Row
	wheel(m, 10, 4, tea.MouseWheelDown)
	if m.offset() <= before || m.Row != row {
		t.Fatal("narrow detail wheel did not use the visible diff")
	}
}

func TestMouseWheelIgnoresChromeModifiersAndBlockedSurfaces(t *testing.T) {
	m := largeModel(largeTextSession(8, 24), 120, 10)
	defer m.Close()
	g := m.workspaceGeometry()
	for _, msg := range []tea.MouseWheelMsg{
		{X: g.Detail.Min.X, Y: 1, Button: tea.MouseWheelDown},
		{X: g.Divider.Min.X, Y: 4, Button: tea.MouseWheelDown},
		{X: -1, Y: 4, Button: tea.MouseWheelDown},
		{X: g.Detail.Min.X, Y: 4, Button: tea.MouseWheelRight},
		{X: g.Detail.Min.X, Y: 4, Button: tea.MouseWheelDown, Mod: tea.ModShift},
	} {
		m.Update(msg)
	}
	if m.offset() != 0 || m.Selected != 0 {
		t.Fatal("wheel on chrome, divider, or modified input moved review")
	}
	m.Busy = true
	wheel(m, g.Detail.Min.X, 4, tea.MouseWheelDown)
	if m.offset() != 0 {
		t.Fatal("busy review accepted wheel")
	}
}

func TestMouseWheelScrollsDescription(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	s := screenSession()
	description := strings.Repeat("description line\n", 20)
	s.PullRequestDescription = &description
	m.openReviewTab(s)
	m.Width, m.Height = 60, 7
	m.selectReviewView(viewDescription)
	wheel(m, 10, 3, tea.MouseWheelDown)
	if m.DescriptionScroll != mouseWheelStep {
		t.Fatalf("description wheel scrolled %d, want %d", m.DescriptionScroll, mouseWheelStep)
	}
	wheel(m, 10, 1, tea.MouseWheelDown)
	if m.DescriptionScroll != mouseWheelStep {
		t.Fatal("description chrome wheel moved content")
	}
}
