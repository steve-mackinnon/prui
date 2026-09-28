package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prui/internal/session"
	"prui/internal/source"
)

func TestMousePickerScrolledRepositorySelectionOnly(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 40, 8
	m.Stack = []page{pageRepositoryPicker}
	for i := 0; i < 20; i++ {
		m.Repositories = append(m.Repositories, session.Repository{Repository: fmt.Sprintf("owner/界-%02d", i)})
	}
	m.RepositoryPicker.Index = 12
	lines := strings.Split(m.repositoryPickerView(), "\n")
	if !strings.Contains(lines[2], "08") {
		t.Fatalf("unexpected first visible row: %q", lines[2])
	}
	if cmd := m.mousePickerClick(6, 2); cmd != nil {
		t.Fatal("click returned activation command")
	}
	if m.RepositoryPicker.Index != 8 || m.top() != pageRepositoryPicker || m.Busy {
		t.Fatal("click did not only select visible repository")
	}
}

func TestMousePickerIgnoresChromeAndOutside(t *testing.T) {
	for _, xy := range [][2]int{{0, 0}, {0, 1}, {0, 4}, {0, 5}, {-1, 2}, {40, 2}, {0, -1}, {0, 8}} {
		m := New(context.Background(), nil)
		m.Width, m.Height = 40, 8
		m.Stack = []page{pageRepositoryPicker}
		m.Repositories = []session.Repository{{Repository: "a"}, {Repository: "b"}}
		m.mousePickerClick(xy[0], xy[1])
		if m.RepositoryPicker.Index != 0 {
			t.Fatalf("chrome click %v selected row", xy)
		}
	}
}

func TestMousePickerTinyAndIndependentCursors(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 10, 1
	m.Stack = []page{pageRepositoryPicker}
	m.Repositories = []session.Repository{{Repository: "a"}, {Repository: "b"}}
	m.RepositoryPicker.Index = 1
	m.SessionPicker.Index = 7
	m.PullRequestPicker.Index = 6
	m.mousePickerClick(0, 0)
	if m.RepositoryPicker.Index != 1 || m.SessionPicker.Index != 7 || m.PullRequestPicker.Index != 6 {
		t.Fatal("tiny picker moved unrelated cursor")
	}
}

func TestMousePickerPRDetailIsNotAResult(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 80, 12
	m.Stack = []page{pagePullRequestPicker}
	m.PullRequests = []source.PullRequest{{Identity: source.Identity{Number: 1}}, {Identity: source.Identity{Number: 2}}}
	m.mousePickerClick(0, 2)
	if m.PullRequestPicker.Index != 1 {
		t.Fatal("second PR not selected")
	}
	for y := 3; y < 12; y++ {
		m.mousePickerClick(0, y)
		if m.PullRequestPicker.Index != 1 {
			t.Fatalf("detail/footer row %d changed selection", y)
		}
	}
	if m.Busy || m.Session != nil {
		t.Fatal("selection activated PR")
	}
}

func TestMousePickerFilteredSwitcherUsesDisplayedResult(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 80, 10
	m.Session = largeSession(1, 1)
	m.Stack = []page{pagePullRequestPicker}
	m.SwitcherQuery = "界"
	m.PullRequests = []source.PullRequest{
		{Identity: source.Identity{Repository: "owner/repo", Number: 1}, Title: "hidden"},
		{Identity: source.Identity{Repository: "owner/repo", Number: 2}, Title: "界 first"},
		{Identity: source.Identity{Repository: "owner/repo", Number: 3}, Title: "界 second"},
	}
	before := m.Session
	m.mousePickerClick(5, 3)
	results := m.switcherResults()
	if m.PullRequestPicker.Index != 1 || results[m.PullRequestPicker.Index].identity.Number != 3 {
		t.Fatal("click selected unfiltered identity")
	}
	if m.SwitcherQuery != "界" || m.Session != before || m.Busy {
		t.Fatal("click changed query or activated result")
	}
}

func TestMousePickerThemeSelectionDoesNotApply(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {20, 6}} {
		m := New(context.Background(), nil)
		m.Width, m.Height = size[0], size[1]
		m.Stack = []page{pageThemePicker}
		before := m.theme.Name
		_, top, _, _ := themePickerBounds(m.Width, m.Height)
		left, _, _, _ := themePickerBounds(m.Width, m.Height)
		m.mousePickerClick(left+2, top+3)
		if m.ThemePicker.Index != 1 || m.theme.Name != before || m.top() != pageThemePicker {
			t.Fatal("theme click failed selection-only behavior")
		}
	}
	m := New(context.Background(), nil)
	m.Width, m.Height = 10, 3
	m.Stack = []page{pageThemePicker}
	m.mousePickerClick(2, 1)
	if m.ThemePicker.Index != 0 {
		t.Fatal("compact controls are not a theme result")
	}
}

func TestMousePickerBusyAndEmptyIgnoreClicks(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 40, 10
	m.Stack = []page{pageRepositoryPicker}
	m.mousePickerClick(0, 2)
	if m.RepositoryPicker.Index != 0 {
		t.Fatal("empty picker moved selection")
	}
	m.Repositories = []session.Repository{{Repository: "a"}, {Repository: "b"}}
	m.Busy = true
	m.mousePickerClick(0, 3)
	if m.RepositoryPicker.Index != 0 || m.mousePickerAvailable() {
		t.Fatal("busy picker accepted selection")
	}
}

func TestMousePickerSwitcherChangingDetailReservation(t *testing.T) {
	for _, height := range []int{4, 8, 12} {
		for _, selected := range []int{0, 6} {
			m := New(context.Background(), nil)
			m.Width, m.Height = 80, height
			m.Session = largeSession(1, 1)
			m.Stack = []page{pagePullRequestPicker}
			// The opened review has no browser metadata; PR results reserve details.
			m.tabs = []workspaceTab{{identity: source.Identity{Repository: "owner/repo", Number: 99}}}
			for i := 1; i <= 9; i++ {
				m.PullRequests = append(m.PullRequests, source.PullRequest{Identity: source.Identity{Repository: "owner/repo", Number: i}, Title: fmt.Sprintf("result-%02d", i)})
			}
			m.PullRequestPicker.Index = selected
			lines := strings.Split(m.switcherView(), "\n")
			found := false
			for y, line := range lines {
				if strings.Contains(line, "result-") {
					index := 0
					for i := 1; i <= 9; i++ {
						if strings.Contains(line, fmt.Sprintf("result-%02d", i)) {
							index = i
							break
						}
					}
					m.mousePickerClick(4, y)
					if m.PullRequestPicker.Index != index {
						t.Fatalf("height=%d selected=%d row=%d got=%d want=%d", height, selected, y, m.PullRequestPicker.Index, index)
					}
					found = true
					break
				}
			}
			if !found && selected != 0 {
				t.Fatal("selected PR missing")
			}
		}
	}
}

func TestMousePickerUpdateSelectionAndKeyboardNavigation(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 40, 10
	m.Stack = []page{pageRepositoryPicker}
	m.Repositories = []session.Repository{{Repository: "a"}, {Repository: "b"}}
	for i := 0; i < 2; i++ {
		_, cmd := m.Update(tea.MouseClickMsg{X: 1, Y: 3, Button: tea.MouseLeft})
		if cmd != nil || m.RepositoryPicker.Index != 1 || m.Busy || m.top() != pageRepositoryPicker {
			t.Fatal("mouse selected or activated incorrectly")
		}
	}
	m.Update(tea.MouseClickMsg{X: 1, Y: 2, Button: tea.MouseRight})
	if m.RepositoryPicker.Index != 1 {
		t.Fatal("right click changed selection")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.RepositoryPicker.Index != 0 {
		t.Fatal("keyboard navigation stopped working")
	}
}
