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
			wantTab := "› Description [2]"
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

func TestPRContextTabsFollowIdentityAndHaveDirectNumberKeys(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 12

	view := ansi.Strip(m.View().Content)
	_, tabsAndBody, _ := strings.Cut(view, "\n")
	first, _, _ := strings.Cut(tabsAndBody, "\n")
	if first != "› Diff [1]    Description [2]    Commits [3]" {
		t.Fatalf("top row = %q, want direct tab strip below identity", first)
	}
	if strings.Contains(first, "review ·") || strings.Contains(first, "ctrl+p") {
		t.Fatalf("top row retained review chrome: %q", first)
	}

	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("2 selected %v, want description", got)
	}
	m.Update(tea.KeyPressMsg{Code: '3', Text: "3"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("3 selected %v, want commits", got)
	}
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if got := m.selectedReviewView(); got != viewChanges {
		t.Fatalf("1 selected %v, want diff", got)
	}
}

func TestPRContextTabsHighlightOnlyTheSelectedView(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())

	for _, tc := range []struct {
		view  reviewView
		label string
	}{
		{viewChanges, "Diff [1]"},
		{viewDescription, "Description [2]"},
		{viewCommits, "Commits [3]"},
	} {
		m.selectReviewView(tc.view)
		active := m.styleLine(selectedClass(true), "› "+tc.label)
		tabs := m.contextViewTabs()
		if !strings.Contains(tabs, active) {
			t.Fatalf("selected %s tab is not highlighted", tc.label)
		}
		for _, other := range []string{"Diff [1]", "Description [2]", "Commits [3]"} {
			if other != tc.label && strings.Contains(tabs, m.styleLine(selectedClass(true), "› "+other)) {
				t.Fatalf("unselected %s tab is highlighted", other)
			}
		}
	}
}

func TestReviewTabsHighlightOnlySelectedFileOrGuide(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 12

	for _, tc := range []struct {
		files  bool
		active string
		other  string
	}{
		{true, "File (F)", "Guide (G)"},
		{false, "Guide (G)", "File (F)"},
	} {
		m.Files = tc.files
		header := strings.Split(m.View().Content, "\n")[2]
		if !strings.Contains(header, m.styleLine(selectedClass(true), "› "+tc.active)) {
			t.Fatalf("selected %s tab is not highlighted", tc.active)
		}
		if strings.Contains(header, m.styleLine(selectedClass(true), "› "+tc.other)) {
			t.Fatalf("unselected %s tab is highlighted", tc.other)
		}
	}
}
