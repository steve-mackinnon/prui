package tui

import (
	"fmt"
	"strings"
	"testing"

	"prui/internal/guide"
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
