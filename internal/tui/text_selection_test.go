package tui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func clipboardText(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if text := clipboardText(child); text != "" {
				return text
			}
		}
		return ""
	}
	// Clipboard messages have a string underlying type; timer messages do not.
	if strings.Contains(fmt.Sprintf("%T", msg), "setClipboardMsg") {
		return fmt.Sprint(msg)
	}
	return ""
}

func TestTextSelectionCopiesOnRelease(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	g := m.workspaceGeometry().Detail
	y := g.Min.Y
	line := strings.Split(m.View().Content, "\n")[y]
	want := ansi.Strip(ansi.Cut(line, g.Min.X+2, g.Min.X+9))
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: g.Min.X + 2, Y: y})
	_, cmd := m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: g.Min.X + 8, Y: y})
	if cmd != nil {
		t.Fatal("copied before release")
	}
	_, cmd = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: g.Min.X + 8, Y: y})
	if got := clipboardText(cmd); got != strings.TrimRight(want, " ") || got == "" {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "copied to clipboard") {
		t.Fatal("missing copy toast")
	}
}

func TestTextSelectionClickAndDividerDoNotCopy(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	g := m.workspaceGeometry()
	for _, x := range []int{g.Detail.Min.X + 2, g.Divider.Min.X, g.Rail.Min.X} {
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: g.Detail.Min.Y})
		_, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: g.Detail.Min.Y})
		if clipboardText(cmd) != "" {
			t.Fatal("click copied text")
		}
	}
}

func TestTextSelectionPaneClippingAndGraphemes(t *testing.T) {
	s := textSelection{
		bounds: image.Rect(5, 0, 16, 2),
		start:  image.Pt(12, 1), end: image.Pt(7, 0),
		content: "rail│ 界e\u0301 hello │other\nrail│ second   │other",
	}
	text, highlighted := s.render(24, 2)
	if text != "界e\u0301 hello\n second" {
		t.Fatalf("copy = %q", text)
	}
	if strings.Contains(text, "rail") || strings.Contains(text, "other") || strings.Contains(text, "\x1b") {
		t.Fatal("copied neighboring pane or controls")
	}
	if ansi.Strip(highlighted) != ansi.Strip(s.content) {
		t.Fatalf("highlight changed text: %q", ansi.Strip(highlighted))
	}
}

func TestTextSelectionCancelledByInput(t *testing.T) {
	for _, msg := range []tea.Msg{
		tea.KeyPressMsg{Code: tea.KeyEscape},
		tea.WindowSizeMsg{Width: 120, Height: 24},
		tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 80, Y: 4},
		tea.MouseMotionMsg{Button: tea.MouseLeft, Mod: tea.ModShift, X: 80, Y: 4},
	} {
		m := largeModel(screenSession(), 120, 24)
		g := m.workspaceGeometry().Detail
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: g.Min.X + 2, Y: g.Min.Y})
		m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: g.Min.X + 6, Y: g.Min.Y})
		m.Update(msg)
		_, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: g.Min.X + 6, Y: g.Min.Y})
		if clipboardText(cmd) != "" {
			t.Fatalf("%T did not cancel", msg)
		}
	}
}

func TestClipboardToastExpiresWithoutClearingNewerCopy(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	m.copyText("first")
	first := m.clipboardToastGeneration
	m.copyText("second")
	m.Update(clipboardToastExpired(first))
	if !strings.Contains(ansi.Strip(m.View().Content), "copied to clipboard") {
		t.Fatal("old timer hid newer toast")
	}
	m.Update(clipboardToastExpired(m.clipboardToastGeneration))
	if strings.Contains(ansi.Strip(m.View().Content), "copied to clipboard") {
		t.Fatal("toast did not disappear")
	}
}

func TestTextSelectionURLDragCopiesWithoutOpening(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	m.selectReviewView(viewDescription)
	body := "https://example.com/path"
	m.Session.PullRequestDescription = &body
	cells := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if cells.CellAt(x, y).Link.URL != "https://example.com/path" {
				continue
			}
			_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
			if cmd != nil {
				t.Fatal("opened link on selection start")
			}
			m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x + 5, Y: y})
			_, cmd = m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x + 5, Y: y})
			if clipboardText(cmd) != "https:" {
				t.Fatal("link drag did not copy label")
			}
			return
		}
	}
	t.Fatal("missing URL fixture")
}

func TestTextSelectionInFilteredDiff(t *testing.T) {
	m := largeModel(screenSession(), 120, 24)
	m.commitFilter.subset = true
	m.commitFilter.inventory = m.Session.Inventory
	g := m.filteredGeometry().Detail
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: g.Min.X + 2, Y: g.Min.Y})
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: g.Min.X + 8, Y: g.Min.Y})
	_, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: g.Min.X + 8, Y: g.Min.Y})
	if clipboardText(cmd) == "" {
		t.Fatal("filtered diff swallowed text selection")
	}
}

func TestTextSelectionOverviewDoesNotToggleComment(t *testing.T) {
	m := largeModel(screenSession(), 120, 40)
	m.selectReviewView(viewDescription)
	m.discussions.loaded = true
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{
		Complete: true, Timeline: true, Events: []source.ConversationEvent{{ID: "comment", Kind: "PR comment", Body: "Selectable comment text"}},
	}}
	m.discussions.overviewExpanded = map[string]bool{"comment": true}
	y, x := -1, -1
	for i, row := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if col := strings.Index(row, "Selectable"); col >= 0 {
			x, y = ansi.StringWidth(row[:col]), i
			break
		}
	}
	if x < 0 {
		t.Fatal("missing comment body")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !m.discussions.overviewExpanded["comment"] {
		t.Fatal("selection press collapsed comment")
	}
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x + 5, Y: y})
	_, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x + 5, Y: y})
	if clipboardText(cmd) != "Select" || !m.discussions.overviewExpanded["comment"] || m.selectedReviewView() != viewDescription {
		t.Fatal("drag changed comment state instead of only copying")
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y})
	if m.discussions.overviewExpanded["comment"] {
		t.Fatal("plain click no longer toggles comment")
	}
}
