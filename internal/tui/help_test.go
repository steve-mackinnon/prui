package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

func TestHelpViewportMakesAllControlsReachableAndReturnsToReview(t *testing.T) {
	for _, height := range []int{12, 24} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.openReviewTab(screenSession())
			m.Width, m.Height = 120, height
			m.Focus, m.Selected = paneDiff, 1
			key(m, '?')
			seen := ""
			for i := 0; i < 100; i++ {
				view := ansi.Strip(m.View().Content)
				lines := strings.Split(view, "\n")
				if len(lines) > height || !strings.Contains(lines[len(lines)-1], "esc: back") {
					t.Fatalf("help viewport lost bounds or controls: %q", view)
				}
				seen += "\n" + view
				namedKey(m, tea.KeyDown)
			}
			for _, b := range bindings {
				if b.groups&groupHelp == 0 {
					continue
				}
				// At 120 columns each panel has 55 columns of content.
				for _, fragment := range strings.Split(ansi.Wrap(b.keys+": "+b.desc, 55, ""), "\n") {
					if !strings.Contains(seen, fragment) {
						t.Fatalf("help binding fragment not reachable: %q", fragment)
					}
				}
			}
			if !strings.Contains(seen, "Reading progress is local") {
				t.Fatal("help notes unreachable")
			}
			namedKey(m, tea.KeyEscape)
			if m.top() != pageReview || m.Focus != paneDiff || m.Selected != 1 {
				t.Fatal("help changed review state")
			}
			key(m, '?')
			if !strings.Contains(ansi.Strip(m.View().Content), "Inventory complete") {
				t.Fatal("reopening help retained old scroll")
			}
		})
	}
}

func TestHelpViewportPagesWrapsAndKeepsEscapedText(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 40, 12
	m.ActionError = fmt.Errorf("%s", strings.Repeat("long-word-", 12)+"\x1b]52;unsafe\a tail")
	key(m, '?')
	start := m.View().Content
	namedKey(m, tea.KeyPgDown)
	if m.View().Content == start {
		t.Fatal("page down did not advance help")
	}
	namedKey(m, tea.KeyHome)
	if m.View().Content != start {
		t.Fatal("home did not restore help start")
	}
	all := ""
	for i := 0; i < 200; i++ {
		view := ansi.Strip(m.View().Content)
		for _, line := range strings.Split(view, "\n") {
			if visibleWidth(line) > 40 {
				t.Fatalf("help exceeds width: %q", line)
			}
		}
		all += view
		namedKey(m, tea.KeyDown)
	}
	if strings.Contains(all, "\x1b]") || !strings.Contains(all, "unsafe") || !strings.Contains(all, "tail") {
		t.Fatal("help lost wrapped error or exposed controls")
	}
}

func TestHelpPanelsFitResponsiveWidths(t *testing.T) {
	for _, width := range []int{1, 7, 8, 40, 99, 100, 120, 159, 160, 240} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.Width, m.Height = width, 24
			lines := m.helpLines()
			for _, line := range lines {
				if visibleWidth(line) > width {
					t.Fatalf("panel exceeds %d columns: %q", width, line)
				}
			}
			if width < 8 {
				return
			}
			want := 1
			if width >= 160 {
				want = 3
			} else if width >= 100 {
				want = 2
			}
			if got := strings.Count(ansi.Strip(lines[0]), "┌"); got != want {
				t.Fatalf("got %d panel columns, want %d", got, want)
			}
		})
	}
}

func TestHelpPanelThemePreservesTextAndHighlightsHierarchy(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	plain := ansi.Strip(strings.Join(m.helpCard("Navigate", "j/k: move cursor", 40), "\n"))
	for _, mode := range []string{theme.Dark, theme.Light} {
		palette, err := theme.Resolve(mode, nil)
		if err != nil {
			t.Fatal(err)
		}
		m.SetTheme(palette)
		styled := strings.Join(m.helpCard("Navigate", "j/k: move cursor", 40), "\n")
		if ansi.Strip(styled) != plain {
			t.Fatal("theme changed help panel text or geometry")
		}
		if !strings.Contains(styled, m.styleLine(classTitle, "Navigate")) ||
			!strings.Contains(styled, m.styleLine(classHunk, "j/k:")) {
			t.Fatal("help panel lost heading or shortcut styling")
		}
	}
}
