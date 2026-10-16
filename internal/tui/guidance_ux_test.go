package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/session"
)

func TestHelpStartsWithFirstReviewAndExpandsShortcutsOnRequest(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 80, 24
	key(m, '?')
	compact := ansi.Strip(strings.Join(m.helpLines(), "\n"))
	for _, want := range []string{"Your first review", "Space", "Actions", "Tab", "Enter", "Ctrl+P", "Health", "s: all shortcuts"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("getting started omitted %q: %s", want, compact)
		}
	}
	if strings.Index(compact, "Your first review") > strings.Index(compact, "Health") {
		t.Fatal("health precedes getting started")
	}
	if strings.Contains(compact, "ctrl+s:") {
		t.Fatal("advanced shortcuts shown before explicit request")
	}
	m.helpKey("s")
	expanded := ansi.Strip(strings.Join(m.helpLines(), "\n"))
	for _, want := range []string{"ctrl+s:", "Commits (4)", "F: filter by filename", "Space/ctrl+k:"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded help omitted %q", want)
		}
	}
	if !strings.Contains(ansi.Strip(m.helpViewport()), "Navigate") {
		t.Fatal("shortcut toggle did not start at the shortcut reference")
	}
	if strings.Contains(expanded, "Commits (3)") || strings.Contains(expanded, "/: filter by filename") {
		t.Fatal("stale shortcuts remain")
	}
	m.helpKey("s")
	if strings.Contains(ansi.Strip(strings.Join(m.helpLines(), "\n")), "ctrl+s:") {
		t.Fatal("shortcut toggle did not collapse")
	}
}

func TestFocusedHintsTeachFamiliarKeysAndStatusRetainsWarnings(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.selectReviewView(viewFiles)
	for _, width := range []int{40, 80, 120} {
		m.Width = width
		m.Focus = paneList
		hints := ansi.Strip(m.reviewHints())
		if visibleWidth(hints) > width || width >= 90 && !strings.Contains(hints, "Space: Actions") {
			t.Fatalf("width %d lost Actions: %s", width, hints)
		}
		if width >= 80 {
			for _, want := range []string{"↑/↓", "Tab", "enter: diff"} {
				if !strings.Contains(hints, want) {
					t.Fatalf("hints omitted %q: %s", want, hints)
				}
			}
		}
	}
	m.Session.Inventory.Complete = false
	m.Session.RevisionStatus = session.Stale
	for _, width := range []int{40, 60, 120} {
		status := ansi.Strip(m.healthStatus(width, m.Session))
		if !strings.Contains(status, "R ") || !strings.Contains(status, "!") || visibleWidth(status) > width {
			t.Fatalf("width %d lost status priorities: %s", width, status)
		}
	}
}

func TestGettingStartedAtSixtyColumnsKeepsReferenceAndBackControls(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(screenSession())
	m.Width, m.Height = 60, 12
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	key(m, '?')
	for _, expanded := range []bool{false, true, false} {
		if m.helpExpanded != expanded {
			m.helpKey("s")
		}
		view := ansi.Strip(m.helpViewport())
		lines := strings.Split(view, "\n")
		if len(lines) > 12 || !strings.Contains(lines[len(lines)-1], "s:") || !strings.Contains(lines[len(lines)-1], "esc: back") {
			t.Fatalf("bounded help lost controls: %s", view)
		}
		heading := "Your first review"
		if expanded {
			heading = "Navigate"
		}
		if !strings.Contains(view, heading) {
			t.Fatalf("help omitted %q: %s", heading, view)
		}
	}
	m.helpKey("esc")
	if m.top() != pageReview || m.Focus != paneDiff {
		t.Fatal("help changed review focus")
	}
}

func TestReviewViewportKeepsNavigationAndBackHintsAtResponsiveWidths(t *testing.T) {
	for _, view := range []reviewView{viewFiles, viewGuide, viewCommits} {
		for _, focus := range []pane{paneList, paneDiff} {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.openReviewTab(screenSession())
			m.selectReviewView(view)
			m.Focus, m.commit.focus = focus, focus
			for _, width := range []int{40, 60} {
				m.Width, m.Height = width, 24
				rendered := ansi.Strip(m.View().Content)
				lines := strings.Split(rendered, "\n")
				footer := lines[len(lines)-1]
				for _, want := range []string{"enter", "esc:"} {
					if !strings.Contains(footer, want) {
						t.Fatalf("view %v focus %v width %d lost %s in footer: %s", view, focus, width, want, footer)
					}
				}
				if width == 60 {
					for _, want := range []string{"↑/↓", "Tab:"} {
						if !strings.Contains(footer, want) {
							t.Fatalf("width 60 omitted %s: %s", want, footer)
						}
					}
				}
				if !strings.Contains(rendered, "Actions") {
					t.Fatalf("width %d lost Actions discovery: %s", width, rendered)
				}
			}
		}
	}
}
