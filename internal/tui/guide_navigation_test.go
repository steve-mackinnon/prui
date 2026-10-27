package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/theme"
)

func TestGuideNavigationHeaderStaysFirstAndHandlesClicks(t *testing.T) {
	for _, width := range []int{60, 120} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s := largeTextSession(2, 2)
			s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{
				{Title: strings.Repeat("Long title ", 8), Sections: []guide.Section{{Title: "First", UnitIDs: []string{s.Inventory.Units[0].ID}}}},
				{Title: "Second", Sections: []guide.Section{{Title: "Second", UnitIDs: []string{s.Inventory.Units[1].ID}}}},
			}}
			m := largeModel(s, width, 12)
			defer m.Close()
			m.Files, m.Focus = false, paneList
			g := m.workspaceGeometry()
			click := func(x int) { m.mouseReviewClick(g.Rail.Min.X+x, g.Rail.Min.Y) }
			check := func(want int) {
				t.Helper()
				list, _ := m.reviewListPresentation()
				if len(list) == 0 || !strings.HasPrefix(list[0].text, " [← {] [→ }]") || !strings.Contains(list[0].text, fmt.Sprintf("%02d / 02", want+1)) {
					t.Fatalf("navigation missing above title: %+v", list)
				}
				if active, _ := m.activeGuide(); active != want {
					t.Fatalf("active guide = %d, want %d", active, want)
				}
			}
			check(0)
			click(2) // Previous at the first guide is a no-op.
			check(0)
			click(8)
			check(1)
			click(8) // Next at the last guide is a no-op.
			check(1)
			key(m, 'j')
			key(m, 'j')
			check(1) // Navigation stays first while browsing nested rows.
			row := m.Row
			click(6)  // Space between buttons.
			click(15) // Counter.
			if m.Row != row {
				t.Fatal("non-button header cells changed selection")
			}
			click(2)
			check(0)
		})
	}
}

func TestGuideNavigationButtonsHaveDistinctBackground(t *testing.T) {
	for _, name := range []string{theme.Dark, theme.Light} {
		t.Run(name, func(t *testing.T) {
			m := guideScrollModel(1, 120)
			defer m.Close()
			m.Focus = paneList
			palette, err := theme.Resolve(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(palette)
			m.colorProfile = colorprofile.TrueColor
			view := m.View().Content
			cells := canvasCells(view, m.Width, m.Height)
			g := m.workspaceGeometry()
			base := cells.CellAt(g.Rail.Min.X, g.Rail.Min.Y).Style.Bg
			for _, x := range []int{1, 3, 5, 7, 9, 11} {
				if sameCanvasColor(cells.CellAt(g.Rail.Min.X+x, g.Rail.Min.Y).Style.Bg, base) {
					t.Fatalf("button cell %d blends into header background", x)
				}
			}
			for _, x := range []int{6, 12, 15} {
				if !sameCanvasColor(cells.CellAt(g.Rail.Min.X+x, g.Rail.Min.Y).Style.Bg, base) {
					t.Fatalf("button background leaked into header cell %d", x)
				}
			}
			m.colorProfile = colorprofile.NoTTY
			if got := ansi.Strip(strings.Split(m.View().Content, "\n")[g.Rail.Min.Y]); got != ansi.Strip(strings.Split(view, "\n")[g.Rail.Min.Y]) {
				t.Fatal("button styling changed logical header text")
			}
		})
	}
}
