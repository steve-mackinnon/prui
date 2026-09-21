package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/source"
)

func TestPRContextViewDefaultsAndCyclesWithoutChangingChangesState(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Selected, m.Row, m.Focus = 1, 1, paneDiff
	m.Scroll[1], m.Horizontal = 4, 8

	if got := m.selectedReviewView(); got != viewChanges {
		t.Fatalf("new review view = %v, want changes", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("v selected %v, want description", got)
	}
	if m.Selected != 1 || m.Row != 1 || m.Focus != paneDiff || m.Scroll[1] != 4 || m.Horizontal != 8 {
		t.Fatalf("context switch changed source review state: %#v", m)
	}
	m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("second v selected %v, want commits", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("V selected %v, want description", got)
	}
}

func TestPRContextViewIsOwnedByEachOpenReviewTab(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	first := screenSession()
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "example/review", Number: 1}
	second := screenSession()
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "example/review", Number: 2}
	m.openReviewTab(first)
	m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	m.openReviewTab(second)
	if got := m.selectedReviewView(); got != viewChanges {
		t.Fatalf("second review view = %v, want changes", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("second reverse cycle = %v, want commits", got)
	}
	m.activateTab(0)
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("first review view after restore = %v, want description", got)
	}
}

func TestPRContextViewRendersTextualTabStripAndContextPlaceholder(t *testing.T) {
	for _, width := range []int{120, 60} {
		t.Run("width", func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.openReviewTab(screenSession())
			m.Width, m.Height = width, 12
			m.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
			view := ansi.Strip(m.View().Content)
			wantTab := "› Description [F2]"
			if !strings.Contains(view, wantTab) {
				t.Fatalf("context tab strip does not identify active Description:\n%s", view)
			}
			if !strings.Contains(view, "Description was not captured for this session.") {
				t.Fatalf("legacy description state missing:\n%s", view)
			}
			if strings.Contains(view, "main.go") {
				t.Fatalf("description retained a source file rail:\n%s", view)
			}
		})
	}
}

func TestPRContextTabsAreTheQuietTopRowAndHaveDirectFunctionKeys(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 12

	view := ansi.Strip(m.View().Content)
	first, _, _ := strings.Cut(view, "\n")
	if first != "› Diff [F1] ·   Description [F2] ·   Commits [F3]" {
		t.Fatalf("top row = %q, want quiet direct tab strip", first)
	}
	if strings.Contains(first, "review ·") || strings.Contains(first, "ctrl+p") {
		t.Fatalf("top row retained review chrome: %q", first)
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyF2})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("F2 selected %v, want description", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("F3 selected %v, want commits", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyF1})
	if got := m.selectedReviewView(); got != viewChanges {
		t.Fatalf("F1 selected %v, want diff", got)
	}
}
