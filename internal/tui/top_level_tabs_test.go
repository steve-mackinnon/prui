package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
)

func TestTopLevelReviewTabsOrderAndKeyboardSelection(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 16
	want := "  PRs [P]  › Overview [1]    Files [2]    Guide [3]    Commits [4]"
	if got := strings.Split(ansi.Strip(m.View().Content), "\n")[1]; got != want {
		t.Fatalf("tab strip = %q, want %q", got, want)
	}
	for _, tc := range []struct {
		key   rune
		label string
	}{{'1', "Overview [1]"}, {'2', "Files [2]"}, {'3', "Guide [3]"}, {'4', "Commits [4]"}, {'G', "Guide [3]"}, {'2', "Files [2]"}} {
		key(m, tc.key)
		got := ansi.Strip(m.View().Content)
		if !strings.Contains(strings.Split(got, "\n")[1], "› "+tc.label) {
			t.Fatalf("%c did not select %s:\n%s", tc.key, tc.label, got)
		}
		if strings.Contains(got, "File (F)") || strings.Contains(got, "Guide (G)") {
			t.Fatal("nested tab selector remains")
		}
	}
	key(m, '3')
	if !strings.Contains(ansi.Strip(m.View().Content), "No guide yet. Press g") || m.Busy {
		t.Fatal("Guide did not preserve explicit generation state")
	}
}

func TestTopLevelReviewTabsCycleAndWrap(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.openReviewTab(screenSession())
	key(m, '1')
	for _, label := range []string{"Files [2]", "Guide [3]", "Commits [4]", "Overview [1]"} {
		key(m, 'v')
		if !strings.Contains(ansi.Strip(m.contextViewTabs()), "› "+label) {
			t.Fatalf("forward cycle did not select %s", label)
		}
	}
	for _, label := range []string{"Commits [4]", "Guide [3]", "Files [2]", "Overview [1]"} {
		key(m, 'V')
		if !strings.Contains(ansi.Strip(m.contextViewTabs()), "› "+label) {
			t.Fatalf("reverse cycle did not select %s", label)
		}
	}
}

func TestTopLevelReviewTabsMouseSelection(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.openReviewTab(screenSession())
	m.Width, m.Height = 120, 16
	x := visibleWidth("  PRs [P]") + 2
	for _, label := range []string{"Overview [1]", "Files [2]", "Guide [3]", "Commits [4]"} {
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: 1})
		if !strings.Contains(ansi.Strip(m.contextViewTabs()), "› "+label) {
			t.Fatalf("mouse did not select %s", label)
		}
		x += visibleWidth(label) + 4
	}
	key(m, '2')
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 20, Y: 2})
	if !strings.Contains(ansi.Strip(m.contextViewTabs()), "› Files [2]") {
		t.Fatal("pane header changed view")
	}
}

func TestOpeningGuideStartsAtFirstSectionAndPreservesPosition(t *testing.T) {
	for _, shortcut := range []rune{'3', 'G'} {
		t.Run(string(shortcut), func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			s := largeTextSession(2, 2)
			s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{
				{Title: "First guide", Sections: []guide.Section{
					{Title: "First section", UnitIDs: []string{s.Inventory.Units[0].ID}},
					{Title: "Last section", UnitIDs: []string{s.Inventory.Units[1].ID}},
				}},
				{Title: "Last guide", Sections: []guide.Section{{Title: "Repeated unit", UnitIDs: []string{s.Inventory.Units[0].ID}}}},
			}}
			m.openReviewTab(s)
			m.Width, m.Height = 120, 12
			m.Selected = 1
			key(m, shortcut)
			if m.Row != 0 || m.Selected != 0 || m.offset() != 0 {
				t.Fatalf("Guide opened at row/unit/offset %d/%d/%d; want 0/0/0", m.Row, m.Selected, m.offset())
			}
			key(m, 'j')
			key(m, 'j')
			key(m, 'j')
			if m.rows()[m.Row].section != 1 || m.offset() == 0 {
				t.Fatal("could not navigate to last section")
			}
			row, selected, offset, cursor := m.Row, m.Selected, m.offset(), m.cursor()
			key(m, '2')
			key(m, shortcut)
			if m.Row != row || m.Selected != selected || m.offset() != offset || m.cursor() != cursor {
				t.Fatalf("Guide position = %d/%d/%d/%d, want %d/%d/%d/%d", m.Row, m.Selected, m.offset(), m.cursor(), row, selected, offset, cursor)
			}
		})
	}
}

func TestTopLevelGuidePreservesWorkspaceStateAndDiffControls(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	s := screenSession()
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Greeting guide", Sections: []guide.Section{{Title: "Implementation", UnitIDs: []string{"unit-0"}}}}}}
	m.openReviewTab(s)
	m.Width, m.Height = 160, 16
	m.Focus = paneDiff
	m.Scroll[0], m.Horizontal = 2, 5
	key(m, '3')
	if m.selectedReviewView() != viewGuide || !strings.Contains(ansi.Strip(m.View().Content), "Greeting guide") {
		t.Fatal("generated guide is not reachable")
	}
	key(m, 'S')
	if !m.sideBySideEnabled() || m.workspaceGeometry().Detail.Empty() {
		t.Fatal("Guide lost split layout or pane geometry")
	}
	g := m.workspaceGeometry()
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: g.Detail.Min.X + 2, Y: g.Detail.Min.Y + 1})
	if m.Focus != paneDiff {
		t.Fatal("Guide lost diff focus")
	}
	other := screenSession()
	other.Inventory.Comparison.Metadata.Identity.Number = 99
	m.openReviewTab(other)
	m.activateTab(0)
	if m.selectedReviewView() != viewGuide || !m.sideBySideEnabled() {
		t.Fatal("workspace switch lost Guide view or layout")
	}
	key(m, '1')
	key(m, '2')
	if m.Scroll[0] != 2 || m.Horizontal != 5 {
		t.Fatal("view selection reset raw reading state")
	}
	key(m, 'G')
	if len(m.rows()) == 0 {
		t.Fatal("Guide alias did not restore hierarchy")
	}
}

func TestPRsTabOpensListWithMouseAndKeyboard(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		m := New(context.Background(), nil)
		m.openReviewTab(screenSession())
		m.Width, m.Height = 120, 16
		if mouse {
			m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: 1})
		} else {
			key(m, 'P')
		}
		if m.top() != pagePullRequestPicker || !strings.Contains(ansi.Strip(m.View().Content), "Switch pull requests") {
			t.Fatalf("PRs tab did not open PR list (mouse=%v)", mouse)
		}
		namedKey(m, tea.KeyEscape)
		if m.top() != pageReview || m.selectedReviewView() != viewDescription {
			t.Fatal("returning from PR list lost review view")
		}
		m.Close()
	}
}

func TestReviewTabsPreserveIndependentReadingPositions(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := largeTextSession(3, 3)
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{
		{Title: "First", Sections: []guide.Section{{Title: "First changes", UnitIDs: []string{s.Inventory.Units[0].ID}}}},
		{Title: "Second", Sections: []guide.Section{{Title: "Second changes", UnitIDs: []string{s.Inventory.Units[1].ID}}}},
	}}
	m.openReviewTab(s)
	m.Width, m.Height = 120, 12
	key(m, '2')
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	for range 12 {
		key(m, 'j')
	}
	m.Horizontal = 8
	type position struct {
		selected, row, cursor, offset, horizontal int
		focus                                     pane
		active                                    bool
	}
	capture := func() position {
		return position{m.Selected, m.Row, m.cursor(), m.offset(), m.Horizontal, m.Focus, m.cursorActive}
	}
	files := capture()
	key(m, '3')
	m.Focus = paneList
	m.file(1)
	namedKey(m, tea.KeyEnter)
	for range 8 {
		key(m, 'j')
	}
	m.Horizontal = 16
	guides := capture()
	key(m, 'v')
	key(m, 'V')
	if got := capture(); got != guides {
		t.Fatalf("Guide position after cycling = %+v, want %+v", got, guides)
	}
	x := visibleWidth("  PRs [P]") + 2 + visibleWidth("Overview [1]") + 4
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: 1})
	if got := capture(); got != files {
		t.Fatalf("Files position after mouse selection = %+v, want %+v", got, files)
	}
	x += visibleWidth("Files [2]") + 4
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 3, Y: 1})
	if got := capture(); got != guides {
		t.Fatalf("Guide position after mouse selection = %+v, want %+v", got, guides)
	}
	for _, destination := range []rune{'1', '4', '2', '3'} {
		key(m, destination)
		key(m, '2')
		m.View()
		if got := capture(); got != files {
			t.Fatalf("Files position after %c = %+v, want %+v", destination, got, files)
		}
		key(m, '3')
		m.View()
		if got := capture(); got != guides {
			t.Fatalf("Guide position after %c = %+v, want %+v", destination, got, guides)
		}
	}
}

func TestReselectingFilesLeavesInventoryWithoutLosingSelection(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.openReviewTab(largeTextSession(2, 2))
	key(m, '2')
	key(m, 'i')
	m.Selected = 1
	key(m, '2')
	if m.Inventory || !m.Files || m.Selected != 1 {
		t.Fatalf("Files selection = inventory:%v files:%v selected:%d", m.Inventory, m.Files, m.Selected)
	}
}
