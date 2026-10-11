package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
)

func TestGuideHeadersWrapWithoutLosingTextOrRowIdentity(t *testing.T) {
	for _, title := range []string{
		"Column time as a four-quarter host tempo measurement",
		strings.Repeat("界", 24),
		strings.Repeat("e\u0301", 32),
	} {
		for _, width := range []int{24, 36, 60} {
			t.Run(fmt.Sprintf("%s/%d", title, width), func(t *testing.T) {
				s := largeTextSession(1, 1)
				s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: title,
					Sections: []guide.Section{{Title: title, UnitIDs: []string{s.Inventory.Units[0].ID}}}}}}
				rows := rowsFor(s, newExpansion())
				lines := guideList(s, rows, 1, width, true, 0)
				for _, index := range []int{0, 1} {
					var parts []string
					for _, line := range lines {
						if line.row != index {
							continue
						}
						if visibleWidth(line.text) > width {
							t.Fatalf("header exceeds width %d: %q", width, line.text)
						}
						parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line.text, "› ")))
					}
					got := strings.ReplaceAll(strings.Join(parts, ""), " ", "")
					want := strings.ReplaceAll(rows[index].label, " ", "")
					if got != want {
						t.Fatalf("wrapped header = %q, want %q", got, want)
					}
				}
			})
		}
	}
}

func TestGuideWrappedHeaderPresentationAndNavigation(t *testing.T) {
	s := largeTextSession(1, 1)
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Timing engine",
		Sections: []guide.Section{{Title: "Column time as a four-quarter host tempo measurement", Description: "Body explanation", UnitIDs: []string{s.Inventory.Units[0].ID}}}}}}
	m := largeModel(s, 120, 24)
	defer m.Close()
	m.Files, m.Focus = false, paneList
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	list, _ := m.reviewListPresentation()
	continuation := -1
	for i, line := range list {
		if line.row == 1 && !strings.Contains(line.text, "1.1") {
			continuation = i
			break
		}
	}
	if continuation < 0 {
		t.Fatal("wrapped subsection continuation is missing")
	}
	view := strings.Split(downsample(m.View().Content, colorprofile.Ascii), "\n")
	rail := func(line string) string {
		// Exclude both pane borders, including the next border's bold SGR.
		return strings.TrimSuffix(strings.Split(line, "│")[1], "\x1b[1m")
	}
	if !strings.Contains(rail(view[3+continuation]), "\x1b[1m") {
		t.Fatalf("unselected header continuation is not bold: %q", view[3+continuation])
	}
	for _, line := range view {
		if strings.Contains(line, "Body explanation") && strings.Contains(rail(line), "\x1b[1m") {
			t.Fatalf("body explanation is styled as a header: %q", line)
		}
	}
	g := m.workspaceGeometry()
	m.mouseReviewClick(g.Rail.Min.X+5, g.Rail.Min.Y+continuation)
	if m.Row != 1 {
		t.Fatalf("clicking continuation selected row %d, want subsection 1", m.Row)
	}
	list, _ = m.reviewListPresentation()
	view = strings.Split(downsample(m.View().Content, colorprofile.Ascii), "\n")
	for i, line := range list {
		if line.row == 1 && !strings.Contains(rail(view[3+i]), "\x1b[1;7m") {
			t.Fatalf("selected header fragment lacks highlight: %q", view[3+i])
		}
	}
	key(m, 'j')
	if m.rows()[m.Row].kind != portionRow {
		t.Fatal("j did not move past the wrapped subsection to its file")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Timing engine") {
		t.Fatal("guide title disappeared while browsing its file")
	}
}
