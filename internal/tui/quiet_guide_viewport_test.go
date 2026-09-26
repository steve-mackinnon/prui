package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
)

func TestQuietGuideViewportShowsSelectedLowerExplanation(t *testing.T) {
	for _, kind := range []rowKind{guideRow, sectionRow} {
		for _, width := range []int{60, 120} {
			t.Run(fmt.Sprintf("kind-%d-width-%d", kind, width), func(t *testing.T) {
				m := New(context.Background(), nil)
				t.Cleanup(m.Close)
				s := largeSession(12, 12)
				s.Guides = &guide.Bundle{Status: guide.Generated}
				for i, unit := range s.Inventory.Units {
					s.Guides.Items = append(s.Guides.Items, guide.Item{
						Title: fmt.Sprintf("Guide %02d", i), Description: fmt.Sprintf("Guide context %02d", i),
						Sections: []guide.Section{{Title: fmt.Sprintf("Section %02d", i), Description: fmt.Sprintf("Section context %02d", i), UnitIDs: []string{unit.ID}}},
					})
				}
				m.openReviewTab(s)
				m.Files, m.Inventory, m.Focus = false, false, paneList
				m.Width, m.Height = width, 12
				rows := m.rows()
				selected := -1
				for i, row := range rows {
					if row.guide == 10 && row.kind == kind {
						selected = i
						break
					}
				}
				if selected < 0 {
					t.Fatal("fixture lacks lower hierarchy row")
				}
				m.Row, m.Selected = selected, rows[selected].units[0]
				view := ansi.Strip(m.View().Content)
				label, explanation := "Guide 10", "Guide context 10"
				if kind == sectionRow {
					label, explanation = "Section 10", "Section context 10"
				}
				selectedVisible := false
				for _, line := range strings.Split(view, "\n") {
					if strings.HasPrefix(line, "› ") && strings.Contains(line, label) {
						selectedVisible = true
					}
				}
				if !selectedVisible || !strings.Contains(view, explanation) {
					t.Fatalf("selected lower row or its explanation is hidden:\n%s", view)
				}
			})
		}
	}
}
