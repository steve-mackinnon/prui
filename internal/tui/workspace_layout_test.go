package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/source"
)

func TestWorkspaceIdentityPersistsAcrossContextViews(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	s.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 42}
	m.openReviewTab(s)
	m.PullRequests = []source.PullRequest{{Identity: s.Inventory.Comparison.Metadata.Identity, Title: "Fix\x1b]52;unsafe\a title"}}
	for _, view := range []reviewView{viewChanges, viewDescription, viewCommits} {
		m.selectReviewView(view)
		first, _, _ := strings.Cut(ansi.Strip(m.View().Content), "\n")
		if !strings.Contains(first, `owner/repo #42 · Fix\x1b]52;unsafe\a title`) || !strings.Contains(first, "ctrl+p: switch PR") {
			t.Fatalf("view %d identity = %q", view, first)
		}
	}
}

func TestWorkspaceGeometryAndFooterAtResponsiveWidths(t *testing.T) {
	for _, width := range []int{60, 99, 100, 120, 159, 160} {
		for _, height := range []int{6, 12, 24} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				m := New(context.Background(), nil)
				t.Cleanup(m.Close)
				m.openReviewTab(screenSession())
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
				if (width >= 100) != strings.Contains(body, " │ ") {
					t.Fatalf("unexpected pane divider: %q", body)
				}
				if strings.ContainsAny(body, "┌┐└┘") {
					t.Fatalf("body contains pane outline: %q", body)
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
