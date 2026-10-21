package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestPRPickerDividerDrag(t *testing.T) {
	for _, switcher := range []bool{false, true} {
		m := New(context.Background(), nil)
		m.Loading = false
		m.Stack = []page{pagePullRequestPicker}
		m.Width, m.Height = 120, 24
		if switcher {
			m.Session = screenSession()
		}
		m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Title: strings.Repeat("Long title ", 12), Description: strings.Repeat("Description paragraph.\n\n", 30)}}
		headers := 1
		if switcher {
			headers = 2
		}
		g := m.prPickerGeometry(headers)
		m.PullRequestPicker.previewFocus = true
		m.PullRequestPicker.previewScroll = 2
		m.Update(tea.MouseClickMsg{X: g.leftWidth + 1, Y: g.listTop, Button: tea.MouseLeft})
		m.Update(tea.MouseMotionMsg{X: g.leftWidth + 11, Y: g.listTop + 2, Button: tea.MouseLeft})
		resized := m.prPickerGeometry(headers)
		if resized.leftWidth != g.leftWidth+10 {
			t.Fatalf("switcher=%v: width=%d, want %d", switcher, resized.leftWidth, g.leftWidth+10)
		}
		if m.PullRequestPicker.Index != 0 || !m.PullRequestPicker.previewFocus || m.PullRequestPicker.previewScroll != 2 || m.textSelection.active {
			t.Fatal("drag changed selection, focus, scroll, or started copying")
		}
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if !strings.Contains(lines[g.listTop][resized.leftWidth:], " │ ") {
			t.Fatal("rendered divider did not move")
		}
		for _, line := range lines {
			if visibleWidth(line) > m.Width {
				t.Fatal("resized pane overflow")
			}
		}
		m.Update(tea.MouseReleaseMsg{X: resized.leftWidth + 1, Y: g.listTop, Button: tea.MouseLeft})
		m.Update(tea.MouseMotionMsg{X: 10, Y: g.listTop, Button: tea.MouseLeft})
		if m.prPickerGeometry(headers).leftWidth != resized.leftWidth || m.drag.active {
			t.Fatal("release did not end drag")
		}
		m.PullRequestPicker.previewFocus = false
		m.Update(tea.MouseClickMsg{X: resized.leftWidth + 3, Y: g.previewTop, Button: tea.MouseLeft})
		if !m.PullRequestPicker.previewFocus {
			t.Fatal("resized preview could not be focused")
		}
		m.Update(tea.MouseReleaseMsg{X: resized.leftWidth + 3, Y: g.previewTop, Button: tea.MouseLeft})
		m.Update(tea.MouseClickMsg{X: g.leftWidth + 5, Y: g.listTop, Button: tea.MouseLeft})
		if m.PullRequestPicker.previewFocus {
			t.Fatal("expanded list hit testing used old width")
		}
		m.Close()
	}
}

func TestPRPickerDividerCancellationAndBounds(t *testing.T) {
	for _, stop := range []tea.Msg{
		tea.KeyPressMsg{Code: tea.KeyTab},
		tea.WindowSizeMsg{Width: 100, Height: 24},
		tea.MouseMotionMsg{X: -1, Y: 3, Button: tea.MouseLeft},
		tea.MouseMotionMsg{X: 60, Y: 3, Button: tea.MouseLeft, Mod: tea.ModShift},
	} {
		m := New(context.Background(), nil)
		m.Loading = false
		m.Stack = []page{pagePullRequestPicker}
		m.Width, m.Height = 120, 24
		g := m.prPickerGeometry(1)
		m.Update(tea.MouseClickMsg{X: g.leftWidth + 1, Y: g.listTop, Button: tea.MouseLeft})
		if !m.drag.active {
			t.Fatal("divider did not start drag")
		}
		m.Update(stop)
		width := m.prPickerGeometry(1).leftWidth
		m.Update(tea.MouseMotionMsg{X: 70, Y: 3, Button: tea.MouseLeft})
		if m.drag.active || m.prPickerGeometry(1).leftWidth != width {
			t.Fatal("cancelled drag changed width")
		}
		m.Close()
	}
	m := New(context.Background(), nil)
	defer m.Close()
	m.Loading = false
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 180, 24
	g := m.prPickerGeometry(1)
	m.Update(tea.MouseClickMsg{X: g.leftWidth + 1, Y: g.listTop, Button: tea.MouseLeft})
	for _, x := range []int{0, 179} {
		m.Update(tea.MouseMotionMsg{X: x, Y: g.listTop, Button: tea.MouseLeft})
		width := m.prPickerGeometry(1).leftWidth
		if width < 24 || m.Width-width-3 < 30 {
			t.Fatalf("unusable pane widths at x=%d: %d", x, width)
		}
	}
	preferred := m.prPickerGeometry(1).leftWidth
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.prPickerGeometry(1).split || m.drag.active {
		t.Fatal("narrow terminal retained divider")
	}
	m.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
	if m.prPickerGeometry(1).leftWidth != preferred {
		t.Fatal("terminal resize lost preferred width")
	}
}

func TestPRPickerDividerProgram(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 24
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 42}, Title: "Resize preview", Description: strings.Repeat("Description with wrapped words. ", 30)}}
	g := m.prPickerGeometry(1)
	h := runProgram(t, m)
	dividerColumn := func(line string) int {
		left, _, found := strings.Cut(line, "│")
		if !found {
			return -1
		}
		return visibleWidth(left)
	}
	h.expect("initial divider", func(f programFrame) bool {
		lines := strings.Split(f.text, "\n")
		return len(lines) > 1 && dividerColumn(lines[1]) == g.leftWidth+1
	})
	h.p.Send(tea.MouseClickMsg{X: g.leftWidth + 1, Y: g.listTop, Button: tea.MouseLeft})
	h.p.Send(tea.MouseMotionMsg{X: g.leftWidth + 13, Y: g.listTop, Button: tea.MouseLeft})
	h.expect("dragged divider", func(f programFrame) bool {
		lines := strings.Split(f.text, "\n")
		return len(lines) == 24 && dividerColumn(lines[1]) == g.leftWidth+13 && strings.Contains(f.text, "enter: open")
	})
	h.p.Send(tea.MouseReleaseMsg{X: g.leftWidth + 13, Y: g.listTop, Button: tea.MouseLeft})
	h.p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.expect("stacked preview", func(f programFrame) bool {
		return !strings.Contains(f.text, "│") && strings.Contains(f.text, "Description")
	})
	h.p.Send(tea.WindowSizeMsg{Width: 120, Height: 24})
	h.expect("restored width", func(f programFrame) bool {
		lines := strings.Split(f.text, "\n")
		return len(lines) > 1 && dividerColumn(lines[1]) == g.leftWidth+13
	})
}
