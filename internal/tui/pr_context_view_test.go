package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestOpeningPullRequestDefaultsToDescription(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("new PR view = %v, want description", got)
	}
	m.selectReviewView(viewFiles)
	m.openReviewTab(screenSession())
	if got := m.selectedReviewView(); got != viewFiles {
		t.Fatalf("reopened PR view = %v, want preserved files selection", got)
	}
}

func TestLocalComparisonDefaultsToFiles(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	s.Inventory.Comparison.Metadata.Identity.Number = 0
	m.openReviewTab(s)
	if got := m.selectedReviewView(); got != viewFiles {
		t.Fatalf("local comparison view = %v, want files", got)
	}
}

func TestPRContextViewDefaultsAndCyclesWithoutChangingChangesState(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.selectReviewView(viewFiles)
	m.Selected, m.Row, m.Focus = 1, 1, paneDiff
	m.Scroll[1], m.Horizontal = 4, 8

	if got := m.selectedReviewView(); got != viewFiles {
		t.Fatalf("new review view = %v, want changes", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("V selected %v, want description", got)
	}
	if m.Selected != 1 || m.Row != 1 || m.Focus != paneDiff || m.Scroll[1] != 4 || m.Horizontal != 8 {
		t.Fatalf("context switch changed source review state: %#v", m)
	}
	m.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("second V selected %v, want commits", got)
	}
	m.Update(tea.KeyPressMsg{Code: 'V', Text: "V"})
	if got := m.selectedReviewView(); got != viewGuide {
		t.Fatalf("third V selected %v, want guide", got)
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
	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m.openReviewTab(second)
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("second review view = %v, want description", got)
	}
	m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("second direct selection = %v, want commits", got)
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
			m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
			view := ansi.Strip(m.View().Content)
			wantTab := "› Description [1]"
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
	if first != "  PRs [P]  › Description [1]    Files [2]    Guide [3]    Commits [4]" {
		t.Fatalf("top row = %q, want direct tab strip below identity", first)
	}
	if strings.Contains(first, "review ·") || strings.Contains(first, "ctrl+p") {
		t.Fatalf("top row retained review chrome: %q", first)
	}

	m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if got := m.selectedReviewView(); got != viewDescription {
		t.Fatalf("1 selected %v, want description", got)
	}
	m.Update(tea.KeyPressMsg{Code: '4', Text: "4"})
	if got := m.selectedReviewView(); got != viewCommits {
		t.Fatalf("4 selected %v, want commits", got)
	}
	m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if got := m.selectedReviewView(); got != viewFiles {
		t.Fatalf("2 selected %v, want files", got)
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
		{viewFiles, "Files [2]"},
		{viewDescription, "Description [1]"},
		{viewCommits, "Commits [4]"},
		{viewGuide, "Guide [3]"},
	} {
		m.selectReviewView(tc.view)
		active := m.styleLine(selectedClass(true), "› "+tc.label)
		tabs := m.contextViewTabs()
		if !strings.Contains(tabs, active) {
			t.Fatalf("selected %s tab is not highlighted", tc.label)
		}
		for _, other := range []string{"Files [2]", "Description [1]", "Guide [3]", "Commits [4]"} {
			if other != tc.label && strings.Contains(tabs, m.styleLine(selectedClass(true), "› "+other)) {
				t.Fatalf("unselected %s tab is highlighted", other)
			}
		}
	}
}

func TestReviewPaneHeadersIdentifyViewWithoutNestedTabs(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 12
	for _, tc := range []struct {
		view  reviewView
		label string
	}{{viewFiles, "Files"}, {viewGuide, "Guide"}} {
		m.selectReviewView(tc.view)
		header := strings.Split(ansi.Strip(m.View().Content), "\n")[2]
		if !strings.HasPrefix(header, "┌ "+tc.label+" ") || strings.Contains(header, "(F)") || strings.Contains(header, "(G)") {
			t.Fatalf("unexpected pane header: %q", header)
		}
	}
}
