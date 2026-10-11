package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Wrap source before attaching overlays so every visual row participates in
// scrolling and cursor navigation. Unbroken tokens stay intact for panning.
func wrapDiffLines(lines []diffLine, width int) []diffLine {
	return wrapDiffLinesWithNumbers(lines, width, true)
}

func wrapDiffLinesWithNumbers(lines []diffLine, width int, numbers bool) []diffLine {
	return wrapDiffRows(lines, width, numbers, nil)
}

// ends records the exclusive visual-row boundary for each logical source row.
func wrapDiffRows(lines []diffLine, width int, numbers bool, ends *[]int) []diffLine {
	var result []diffLine
	for _, line := range lines {
		if line.sideBySide == nil || line.sideBySide.full != nil {
			if visibleWidth(line.Text) <= width || (line.Class != classAdded && line.Class != classRemoved && line.Class != classContext) {
				result = append(result, line)
				if ends != nil {
					*ends = append(*ends, len(result))
				}
				continue
			}
			marker, text := splitPatchMarker(line.Text)
			offset := len(marker)
			for _, part := range strings.Split(ansi.Wordwrap(text, max(1, width-1), ""), "\n") {
				copy := wrappedSyntax(line, part, &offset)
				copy.Text = marker + part
				result = append(result, copy)
			}
			if ends != nil {
				*ends = append(*ends, len(result))
			}
			continue
		}
		row := *line.sideBySide
		bounds, _ := splitCellBounds(width)
		gutter := 2
		if numbers {
			gutter = 8
		}
		wrapCell := func(cell *diffCell) []*diffCell {
			if cell == nil || cell.line == nil {
				return nil
			}
			marker, text := splitPatchMarker(cell.line.Text)
			var cells []*diffCell
			offset := len(marker)
			for _, part := range strings.Split(ansi.Wordwrap(text, max(1, bounds.Dx()-gutter), ""), "\n") {
				source := wrappedSyntax(*cell.line, part, &offset)
				source.Text = marker + part
				copy := *cell
				copy.line = &source
				cells = append(cells, &copy)
			}
			return cells
		}
		old, new := wrapCell(row.old), wrapCell(row.new)
		for i := 0; i < max(len(old), len(new)); i++ {
			copy := line
			wrapped := diffRow{}
			if i < len(old) {
				wrapped.old = old[i]
			}
			if i < len(new) {
				wrapped.new = new[i]
			}
			copy.sideBySide = &wrapped
			copy.target = rowTarget(wrapped)
			result = append(result, copy)
		}
		if ends != nil {
			*ends = append(*ends, len(result))
		}
	}
	return result
}

type diffWrapCache struct {
	source []diffLine
	rows   []diffLine
	ends   []int
	width  int
}

func (m *Model) wrapSource(lines []diffLine) []diffLine {
	// Reserve cursor gutters even while the list is focused to keep row positions stable.
	width := max(1, m.detailWidth()-2)
	if m.lineNumbersEnabled() && !m.sideBySideEnabled() {
		width = max(1, width-10)
	}
	if len(lines) > 0 && lines[0].sideBySide != nil {
		width = max(1, m.detailWidth()-4)
	}
	cache := &m.diffWrapCache
	if len(lines) > 0 && len(cache.source) == len(lines) && cache.width == width && &cache.source[0] == &lines[0] {
		return cache.rows
	}
	ends := make([]int, 0, len(lines))
	rows := wrapDiffRows(lines, width, m.lineNumbersEnabled(), &ends)
	*cache = diffWrapCache{source: lines, rows: rows, ends: ends, width: width}
	return rows
}

func (m *Model) wrappedGuideAnchor(detail guideDetail, r row) (int, bool) {
	offset, ok := anchorForLayout(detail, r, m.sideBySideEnabled())
	if !ok {
		return 0, false
	}
	lines := detail.lines
	if m.sideBySideEnabled() {
		lines = detail.splitLines
	}
	return len(m.wrapSource(lines[:min(offset, len(lines))])), true
}
