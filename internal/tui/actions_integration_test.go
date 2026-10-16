package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestReviewHeaderActionsAreVisibleAndClickable(t *testing.T) {
	for _, width := range []int{18, 40, 60, 120, 180} {
		for _, view := range []reviewView{viewDescription, viewFiles, viewGuide, viewCommits} {
			t.Run(fmt.Sprintf("%d/view%d", width, view), func(t *testing.T) {
				m := New(context.Background(), nil)
				t.Cleanup(m.Close)
				m.openReviewTab(screenSession())
				m.Width, m.Height = width, 24
				m.selectReviewView(view)
				first, _, _ := strings.Cut(ansi.Strip(m.View().Content), "\n")
				if !strings.Contains(first, "Actions [Space]") || visibleWidth(first) > width {
					t.Fatalf("header hides Actions: %q", first)
				}
				for i, control := range m.reviewHeaderControls() {
					m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: control.bounds.Min.X, Y: 0})
					want := pageActions
					if i == 1 {
						want = pageReviewSubmit
					}
					if m.top() != want || m.selectedReviewView() != view {
						t.Fatalf("%s did not open over original view: page %d, view %d", control.label, m.top(), m.selectedReviewView())
					}
					namedKey(m, tea.KeyEscape)
					if m.top() != pageReview {
						t.Fatal("Escape did not restore review")
					}
				}
			})
		}
	}
}

func TestActionsInputOwnership(t *testing.T) {
	for _, setup := range []struct {
		name string
		set  func(*Model)
	}{
		{"composer", func(m *Model) { m.Composer = &commentComposer{PendingIndex: -1} }},
		{"comment actions", func(m *Model) { m.CommentMenu = &commentActionMenu{} }},
		{"file filter", func(m *Model) { m.openFileFilter() }},
		{"search", func(m *Model) { m.openSearch() }},
		{"commit filter", func(m *Model) { m.openCommitFilter() }},
		{"help", func(m *Model) { m.push(pageHelp) }},
		{"busy", func(m *Model) { m.Busy = true }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			m.openReviewTab(screenSession())
			m.selectReviewView(viewFiles)
			setup.set(m)
			if m.actionsAvailable() {
				t.Fatal("Actions claims input owned by another surface")
			}
			key(m, ' ')
			ctrlKey(m, 'k')
			if m.top() == pageActions {
				t.Fatal("Actions stole input")
			}
		})
	}
}

func TestActionsMenuOwnsPasteAndUnknownKeys(t *testing.T) {
	m := actionsModel(t)
	ctrlKey(m, 'k')
	if m.top() != pageActions {
		t.Fatal("Ctrl+K did not open Actions")
	}
	for _, r := range "qé" {
		key(m, r)
	}
	m.Update(tea.PasteMsg{Content: "R/F?"})
	if m.top() != pageActions || m.fileFilterEditing || m.Composer != nil {
		t.Fatal("unknown key or paste reached a review shortcut")
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview || m.fileFilterEditing || m.searchOpen() || m.Composer != nil {
		t.Fatal("closing menu changed underlying surface")
	}
}

func TestSelectedCommitsKeepsNamedHeaderControlsClickable(t *testing.T) {
	for _, controlIndex := range []int{0, 1} {
		m := commitModel(t)
		m.selectReviewView(viewFiles)
		m.commitFilter.subset = true
		control := m.reviewHeaderControls()[controlIndex]
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: control.bounds.Min.X, Y: 0})
		want := pageActions
		if controlIndex == 1 {
			want = pageReviewSubmit
		}
		if m.top() != want {
			t.Fatalf("selected commits swallowed %s: page %d", control.label, m.top())
		}
	}
}

func TestActionsCanRestoreAllCommitsWithoutAShortcut(t *testing.T) {
	m := commitModel(t)
	m.selectReviewView(viewFiles)
	m.commitFilter.subset = true
	selectAction(t, m, "Filter by commits")
	if !m.commitFilter.open || m.top() != pageReview {
		t.Fatal("named commit filter cannot be reopened to restore All commits")
	}
	m.commitFilterKey("home")
	m.commitFilterKey("enter")
	m.commitFilterKey("esc")
	if m.commitFilter.subset || m.commitFilter.open {
		t.Fatal("named picker journey did not restore All commits")
	}
}

func TestActionsSelectedCommitsRejectsInvisibleCanonicalToggles(t *testing.T) {
	for _, label := range []string{"Toggle full inventory", "Toggle file grouping", "Collapse or reveal generated files"} {
		m := commitModel(t)
		m.selectReviewView(viewFiles)
		m.commitFilter.subset = true
		selectAction(t, m, label)
		if m.top() != pageActions || m.Inventory || m.groupFiles || m.collapseGenerated {
			t.Fatalf("%s changed hidden canonical state in selected commits", label)
		}
	}
}

func TestNarrowReviewKeepsEveryPrimaryViewVisibleAndClickable(t *testing.T) {
	for _, width := range []int{40, 60, 80} {
		m := commitModel(t)
		m.Width = width
		m.selectReviewView(viewFiles)
		strip := strings.Split(ansi.Strip(m.View().Content), "\n")[1]
		for _, label := range []string{"PRs", "Overview", "Files", "Guide", "Commits"} {
			if !strings.Contains(strip, label) {
				t.Fatalf("width %d hides %s: %s", width, label, strip)
			}
		}
		for _, view := range []reviewView{viewDescription, viewFiles, viewGuide, viewCommits} {
			labels := m.contextTabLabels()
			x := 0
			for i, label := range labels {
				if i > 0 && reviewViews[i-1] == view {
					m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 1})
					break
				}
				x += visibleWidth(label) + 2
			}
			if m.selectedReviewView() != view {
				t.Fatalf("width %d: visible tab %d was not clickable", width, view)
			}
		}
	}
}
