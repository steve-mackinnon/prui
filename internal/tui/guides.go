package tui

import (
	"fmt"
	"strings"

	"pr-review/internal/guide"
	"pr-review/internal/review"
)

type rowKind int

const (
	guideRow rowKind = iota
	sectionRow
	portionRow
)

// row is a derived view of the immutable bundle. Rows are rebuilt from the
// stored guides on every render, so navigation can never add review surface or
// drift from what the snapshot froze.
type row struct {
	kind    rowKind
	guide   int   // index into Bundle.Items
	section int   // -1 for a guide row
	file    int   // index into Inventory.Files, for portion rows
	units   []int // unit indexes reachable from this row, in section order
	depth   int
	label   string // read marker excluded; it is added at render time
}

// expansion records the collapsed rows. Collapsed is the exception so a fresh
// session shows the whole hierarchy without a discovery step.
type expansion struct {
	guides   map[int]bool
	sections map[[2]int]bool
}

func newExpansion() expansion {
	return expansion{guides: map[int]bool{}, sections: map[[2]int]bool{}}
}

// rowsFor flattens a generated bundle into the left-pane hierarchy. A fallback,
// absent, or empty bundle yields no rows at all, which is what makes the
// deterministic file plan the fallback without a special case in every caller.
func rowsFor(s *review.Session, e expansion) []row {
	if s == nil || s.Guides == nil || s.Guides.Status != guide.Generated || len(s.Guides.Items) == 0 {
		return nil
	}
	index := make(map[string]int, len(s.Inventory.Units))
	for i, u := range s.Inventory.Units {
		index[u.ID] = i
	}
	var out []row
	for gi, item := range s.Guides.Items {
		start := len(out)
		out = append(out, row{kind: guideRow, guide: gi, section: -1, file: -1, depth: 0})
		var all []int
		for si, section := range item.Sections {
			order, byFile, units := sectionPortions(s, index, section.UnitIDs)
			if len(units) == 0 {
				continue
			}
			all = append(all, units...)
			if e.guides[gi] {
				continue
			}
			// The synthesized ungrouped guide carries one section that repeats
			// its own title; showing it would add a row that says nothing.
			depth := 1
			if !item.Ungrouped {
				out = append(out, row{kind: sectionRow, guide: gi, section: si, file: -1, units: units, depth: 1,
					label: mark(e.sections[[2]int{gi, si}]) + fmt.Sprintf("%d.%d %s", gi+1, si+1, Escape(section.Title))})
				if e.sections[[2]int{gi, si}] {
					continue
				}
				depth = 2
			}
			for _, f := range order {
				out = append(out, row{kind: portionRow, guide: gi, section: si, file: f, units: byFile[f], depth: depth,
					label: portionLabel(s, f, byFile[f])})
			}
		}
		if len(all) == 0 {
			out = out[:start] // a guide with no resolvable unit is not navigation
			continue
		}
		title := mark(e.guides[gi]) + fmt.Sprintf("%d. %s", gi+1, Escape(item.Title))
		if item.Ungrouped {
			title += " (not grouped by analysis)"
		}
		out[start].units, out[start].label = all, title
	}
	return out
}

// sectionPortions collapses a section's units into one portion per file, in
// section order; the same file may appear under several sections.
func sectionPortions(s *review.Session, index map[string]int, ids []string) ([]int, map[int][]int, []int) {
	var order, units []int
	byFile := map[int][]int{}
	for _, id := range ids {
		i, ok := index[id]
		if !ok {
			continue
		}
		f := s.UnitFiles[i]
		if len(byFile[f]) == 0 {
			order = append(order, f)
		}
		byFile[f] = append(byFile[f], i)
		units = append(units, i)
	}
	return order, byFile, units
}

func portionLabel(s *review.Session, file int, units []int) string {
	label := pathLabel(s.Inventory.Files[file])
	if len(units) == 1 {
		return label + " [" + string(s.Inventory.Units[units[0]].Kind) + "]"
	}
	return label + fmt.Sprintf(" [%d units]", len(units))
}

func mark(collapsed bool) string {
	if collapsed {
		return "+ "
	}
	return "- "
}

// rows is the model's view of the hierarchy; nil means the file plan is active,
// either because the reviewer pressed G or because analysis produced no guides.
func (m *Model) rows() []row {
	if m.Files {
		return nil
	}
	return rowsFor(m.Session, m.collapsed)
}

// begin puts the cursor on the first guide row of a freshly loaded session, so
// the hierarchy and the right pane describe the same change from the start.
func (m *Model) begin() {
	rows := m.rows()
	if len(rows) > 0 && len(rows[0].units) > 0 {
		m.Row, m.Selected = 0, rows[0].units[0]
	}
}

// syncRow points the cursor at the deepest visible row that contains the
// selected unit, so switching views or browsing the raw inventory never leaves
// the guide cursor describing a different change than the right pane.
func (m *Model) syncRow(rows []row) {
	found := -1
	for i, r := range rows {
		for _, u := range r.units {
			if u == m.Selected {
				found = i
				break
			}
		}
	}
	if found >= 0 {
		m.Row = found
		return
	}
	m.Row = max(0, min(len(rows)-1, m.Row))
}

// moveRow walks the flattened hierarchy; a row selects its first unit so the
// right pane always shows a real frozen unit rather than generated text.
func (m *Model) moveRow(rows []row, delta int) {
	m.Row = max(0, min(len(rows)-1, m.Row+delta))
	if len(rows[m.Row].units) > 0 {
		m.Selected = rows[m.Row].units[0]
	}
	m.Horizontal = 0
}

func (m *Model) moveGuide(rows []row, delta int) {
	current := rows[max(0, min(len(rows)-1, m.Row))].guide
	target := current + delta
	for i, r := range rows {
		if r.kind == guideRow && r.guide == target {
			m.moveRow(rows, i-m.Row)
			return
		}
	}
}

// toggle expands or collapses the selected guide or section. The toggled row
// keeps its index, and its unit set still contains the selection, so expansion
// never loses the reviewer's place. Portion rows have nothing to expand, so
// they switch panes like the file plan does.
func (m *Model) toggle(rows []row) {
	r := rows[max(0, min(len(rows)-1, m.Row))]
	switch r.kind {
	case guideRow:
		m.collapsed.guides[r.guide] = !m.collapsed.guides[r.guide]
	case sectionRow:
		key := [2]int{r.guide, r.section}
		m.collapsed.sections[key] = !m.collapsed.sections[key]
	default:
		// A portion row has nothing to expand, so tab hands it to the diff
		// pane the same way enter does.
		m.Focus = paneDiff
	}
}

type listLine struct {
	row  int
	text string
}

// guideList renders selectable hierarchy rows and non-selectable context for
// the guide under the cursor. Context lines retain their row identity so
// windowing and selection styling remain correct after descriptions wrap.
func guideList(s *review.Session, rows []row, selected, width int, focused bool) []listLine {
	out := make([]listLine, 0, len(rows))
	selectedGuide := -1
	if selected >= 0 && selected < len(rows) {
		selectedGuide = rows[selected].guide
	}
	for i, r := range rows {
		marker := "  "
		if i == selected {
			marker = "· "
			if focused {
				marker = "> "
			}
		}
		line := marker + strings.Repeat("  ", r.depth)
		if r.kind == portionRow {
			line += readMarker(s, s.Inventory.Files[r.file].ID)
		}
		out = append(out, listLine{row: i, text: line + r.label})
		if r.guide != selectedGuide {
			continue
		}
		item := s.Guides.Items[r.guide]
		switch r.kind {
		case guideRow:
			if !item.Ungrouped {
				out = appendDescription(out, r.depth, Escape(item.Description), width)
			}
		case sectionRow:
			out = appendDescription(out, r.depth, Escape(item.Sections[r.section].Description), width)
		}
	}
	return out
}

func appendDescription(lines []listLine, depth int, text string, width int) []listLine {
	if strings.TrimSpace(text) == "" {
		return lines
	}
	indent := strings.Repeat("  ", depth+2)
	for _, line := range wrap(text, width-visibleWidth(indent)) {
		lines = append(lines, listLine{row: -1, text: indent + line})
	}
	return lines
}

func wrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var lines []string
	for _, word := range strings.Fields(text) {
		for visibleWidth(word) > width {
			lines = append(lines, string([]rune(word)[:width]))
			word = string([]rune(word)[width:])
		}
		if len(lines) == 0 || visibleWidth(lines[len(lines)-1])+1+visibleWidth(word) > width {
			lines = append(lines, word)
			continue
		}
		lines[len(lines)-1] += " " + word
	}
	return lines
}

func firstDisplayLine(lines []listLine, row int) int {
	for i, line := range lines {
		if line.row == row {
			return i
		}
	}
	return 0
}
