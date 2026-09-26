package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
				if b.groups&groupHelp != 0 && !strings.Contains(seen, b.keys+": "+b.desc) {
					t.Fatalf("help binding not reachable: %s: %s", b.keys, b.desc)
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
