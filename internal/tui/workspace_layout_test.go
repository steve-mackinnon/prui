package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestWorkspaceIdentityPersistsAcrossContextViews(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	s.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 42}
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.PullRequests = []source.PullRequest{{Identity: s.Inventory.Comparison.Metadata.Identity, Title: "Fix\x1b]52;unsafe\a title"}}
	for _, view := range []reviewView{viewFiles, viewDescription, viewCommits} {
		m.selectReviewView(view)
		first, _, _ := strings.Cut(ansi.Strip(m.View().Content), "\n")
		if !strings.Contains(first, `owner/repo #42 · Fix\x1b]52;unsafe\a title`) {
			t.Fatalf("view %d identity = %q", view, first)
		}
	}
}

func TestKeyboardResizesReviewPaneAndKeepsTabPreference(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	first := screenSession()
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second := screenSession()
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.selectReviewView(viewFiles)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	initial := m.listWidth()
	key(m, ']')
	if got := m.listWidth(); got != initial+2 {
		t.Fatalf("] width = %d, want %d", got, initial+2)
	}
	line := strings.Split(ansi.Strip(m.View().Content), "\n")[3]
	inner := strings.TrimPrefix(line, "│")
	if inner == line || !strings.HasSuffix(line, "│") {
		t.Fatalf("missing pane frame: %q", line)
	}
	left, _, found := strings.Cut(inner, "│")
	if !found || visibleWidth(left) != m.listWidth() {
		got := visibleWidth(left)
		t.Fatalf("divider at %d, list width %d", got, m.listWidth())
	}
	m.openReviewTab(second)
	m.selectReviewView(viewFiles)
	if got := m.listWidth(); got != initial {
		t.Fatalf("new tab width = %d, want %d", got, initial)
	}
	m.activateTab(0)
	if got := m.listWidth(); got != initial+2 {
		t.Fatalf("restored tab width = %d, want %d", got, initial+2)
	}
	key(m, '[')
	if got := m.listWidth(); got != initial {
		t.Fatalf("[ width = %d, want %d", got, initial)
	}
	for range 100 {
		key(m, '[')
	}
	if got := m.listWidth(); got != 18 {
		t.Fatalf("minimum list width = %d, want 18", got)
	}
	for range 100 {
		key(m, ']')
	}
	if got := m.detailWidth(); got < 40 {
		t.Fatalf("detail narrowed to %d columns", got)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	if got := m.listWidth(); got != 57 {
		t.Fatalf("temporary width clamp = %d, want 57", got)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := m.listWidth(); got != 78 {
		t.Fatalf("narrow single pane width = %d, want 78 interior columns", got)
	}
	preferred := m.listWidthPreference
	key(m, '[')
	if m.listWidthPreference != preferred {
		t.Fatal("narrow view changed hidden pane width preference")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if got := m.detailWidth(); got != 40 {
		t.Fatalf("wide resize lost preferred width: detail = %d", got)
	}
	m.selectReviewView(viewDescription)
	key(m, '[')
	if m.listWidthPreference != preferred {
		t.Fatal("description view changed pane width preference")
	}
}

func TestWorkspaceGeometryAndFooterAtResponsiveWidths(t *testing.T) {
	for _, width := range []int{60, 99, 100, 120, 159, 160} {
		for _, height := range []int{6, 12, 24} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				m := New(context.Background(), nil)
				t.Cleanup(m.Close)
				m.openReviewTab(screenSession())
				m.selectReviewView(viewFiles)
				m.Width, m.Height = width, height
				lines := strings.Split(ansi.Strip(m.View().Content), "\n")
				if len(lines) != height {
					t.Fatalf("got %d lines, want %d", len(lines), height)
				}
				if !strings.HasSuffix(strings.Join(lines, "\n"), ansi.Strip(m.reviewStatus())) {
					t.Fatalf("missing review footer: %q", lines)
				}
				for _, line := range lines {
					if visibleWidth(line) > width {
						t.Fatalf("row exceeds width: %q", line)
					}
				}
				body := lines[3]
				if (width >= 100) != (strings.Count(body, "│") == 3) {
					t.Fatalf("unexpected pane divider: %q", body)
				}
				if !strings.HasPrefix(body, "│") || !strings.HasSuffix(body, "│") {
					t.Fatalf("body lacks pane edges: %q", body)
				}
			})
		}
	}
}

func TestWorkspaceTitleSurvivesReplacingPullRequestList(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	identity := s.Inventory.Comparison.Metadata.Identity
	m.PullRequests = []source.PullRequest{{Identity: identity, Title: "Remember this title"}}
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.PullRequests = nil
	if !strings.Contains(ansi.Strip(m.View().Content), "Remember this title") {
		t.Fatal("open review lost its title when the PR list changed")
	}
}

func TestDescriptionViewportPreservesFooterWhenScrolledToEnd(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	body := strings.Repeat("line\n\n", 30) + "Final description sentence"
	s.PullRequestDescription = &body
	m.openReviewTab(s)
	m.selectReviewView(viewFiles)
	m.Width, m.Height = 60, 10
	m.selectReviewView(viewDescription)
	m.DescriptionScroll = 10000
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(lines) > m.Height || !strings.Contains(strings.Join(lines, "\n"), "Final description sentence") {
		t.Fatalf("description end is not visible in viewport: %q", lines)
	}
	footer := ansi.Strip(m.reviewStatus())
	if !strings.HasSuffix(strings.Join(lines, "\n"), footer) {
		t.Fatalf("description hid footer: %q", lines)
	}
}
