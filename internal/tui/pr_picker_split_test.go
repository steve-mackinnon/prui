package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestPRPickerSplitPreviewAndPinnedFooter(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 24
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 42}, Title: "Retry requests", Description: "## Changes\n\nPreserve cancellation."}}
	view := ansi.Strip(m.View().Content)
	lines := strings.Split(view, "\n")
	if len(lines) != m.Height || !strings.Contains(lines[m.Height-1], "enter: open") {
		t.Fatalf("footer is not pinned: %s", view)
	}
	if !strings.Contains(view, "│") || !strings.Contains(view, "Preserve cancellation.") {
		t.Fatalf("split preview missing: %s", view)
	}
	for _, line := range lines {
		if visibleWidth(line) > m.Width {
			t.Fatalf("overflow: %q", line)
		}
	}
}

func TestPRPickerPreviewScrollAndSelectionReset(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 12
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Description: strings.Repeat("Long paragraph.\n\n", 30)}, {Identity: source.Identity{Number: 2}, Description: "Second description"}}
	m.pullRequestPickerKey("tab")
	m.pullRequestPickerKey("down")
	if m.PullRequestPicker.Index != 0 || m.PullRequestPicker.previewScroll == 0 {
		t.Fatal("preview focus moved selection instead of scrolling")
	}
	m.pullRequestPickerKey("tab")
	m.pullRequestPickerKey("down")
	if m.PullRequestPicker.Index != 1 || m.PullRequestPicker.previewScroll != 0 {
		t.Fatal("selection did not reset preview")
	}
}

func TestPRPickerBoundsAndStates(t *testing.T) {
	for _, width := range []int{24, 80, 100, 120, 180} {
		for _, height := range []int{1, 2, 4, 10, 24} {
			m := New(context.Background(), nil)
			m.Stack = []page{pagePullRequestPicker}
			m.Width, m.Height = width, height
			m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Title: "界 " + strings.Repeat("Long title ", 30), Description: "\x1b[31m\r\nSafe preview"}}
			view := ansi.Strip(m.View().Content)
			lines := strings.Split(view, "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d: height %d", width, height, len(lines))
			}
			for _, line := range lines {
				if visibleWidth(line) > width {
					t.Fatalf("%dx%d: overflow %q", width, height, line)
				}
			}
			if height > 1 && !strings.Contains(lines[height-1], "enter:") {
				t.Fatalf("missing footer: %s", view)
			}
			m.Close()
		}
	}
}

func TestPRPickerProgramPreviewFocusAndResize(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 24
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Title: "First", Description: strings.Repeat("Read this paragraph.\n\n", 30)}, {Identity: source.Identity{Number: 2}, Title: "Second", Description: "Second description"}}
	h := runProgram(t, m)
	h.expect("split preview", func(f programFrame) bool {
		return strings.Contains(f.text, "│") && strings.Contains(f.text, "Read this paragraph.")
	})
	h.key(tea.KeyTab)
	h.expect("preview focus", func(f programFrame) bool { return strings.Contains(f.text, "j/k: scroll") })
	h.key('j')
	h.key(tea.KeyTab)
	h.key('j')
	h.expect("selected second", func(f programFrame) bool {
		return strings.Contains(f.text, "› #2") && strings.Contains(f.text, "Second description")
	})
	h.p.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	h.expect("stacked preview", func(f programFrame) bool {
		return !strings.Contains(f.text, "│") && strings.Contains(f.text, "Second description") && len(strings.Split(f.text, "\n")) == 24
	})
}

func TestPRPickerMousePreviewDoesNotSelect(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 12
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}, Description: strings.Repeat("Paragraph.\n\n", 20)}, {Identity: source.Identity{Number: 2}}}
	m.mousePickerClick(80, 2)
	if !m.PullRequestPicker.previewFocus || m.PullRequestPicker.Index != 0 {
		t.Fatal("preview click changed selection")
	}
	m.mouseWheel(tea.MouseWheelMsg{X: 80, Y: 3, Button: tea.MouseWheelDown})
	if m.PullRequestPicker.previewScroll == 0 || m.PullRequestPicker.Index != 0 {
		t.Fatal("preview wheel did not scroll independently")
	}
	m.mousePickerClick(2, 2)
	if m.PullRequestPicker.Index != 1 || m.PullRequestPicker.previewFocus || m.PullRequestPicker.previewScroll != 0 {
		t.Fatal("list click did not select and reset preview")
	}
}

func TestPRPickerFooterPinnedForEmptyLoadingAndError(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Stack = []page{pagePullRequestPicker}
	m.Width, m.Height = 120, 24
	for _, state := range []string{"empty", "loading", "error"} {
		m.Busy, m.ActionError = false, nil
		switch state {
		case "loading":
			m.Busy, m.notice = true, "Loading pull requests..."
		case "error":
			m.ActionError = errors.New("List unavailable")
		}
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(lines) != m.Height || strings.TrimSpace(lines[m.Height-1]) == "" {
			t.Fatalf("%s footer not pinned", state)
		}
	}
}

func TestPRPickerShiftJKScrollsPreviewWithoutChangingFocus(t *testing.T) {
	for _, switcher := range []bool{false, true} {
		m := New(context.Background(), nil)
		m.Stack = []page{pagePullRequestPicker}
		m.Width, m.Height = 120, 12
		if switcher {
			m.Session = screenSession()
		}
		m.PullRequests = []source.PullRequest{{Identity: source.Identity{Repository: "example/review", Number: 1}, Title: "Selected", Description: strings.Repeat("Paragraph.\n\n", 30)}}
		if switcher {
			m.PullRequestPicker.Index = len(m.switcherResults()) - 1
		}
		selected := m.PullRequestPicker.Index
		for _, focused := range []bool{false, true} {
			m.PullRequestPicker.previewFocus = focused
			m.PullRequestPicker.previewScroll = 0
			key(m, 'J')
			if m.PullRequestPicker.previewScroll == 0 || m.PullRequestPicker.Index != selected || m.PullRequestPicker.previewFocus != focused || m.SwitcherQuery != "" {
				t.Fatal("Shift+J did not independently scroll preview")
			}
			key(m, 'K')
			if m.PullRequestPicker.previewScroll != 0 || m.PullRequestPicker.Index != selected || m.PullRequestPicker.previewFocus != focused {
				t.Fatal("Shift+K did not return preview to top")
			}
		}
		m.Close()
	}
}
