package tui

import "strings"

// diffViewport composes shared file-boundary presentation with caller-owned
// scrolling and selection. Copies keep highlighting out of immutable source caches.
// The returned sticky index lets callers suppress their cursor on the pinned row.
func diffViewport(source []diffLine, offset, height, activeLine int, pinHeader bool) (rows []diffLine, sticky int) {
	offset = max(0, min(offset, max(0, len(source)-1)))
	activeHeader := -1
	for i, line := range source {
		if i > activeLine {
			break
		}
		if line.Class == classFileHeader && strings.HasPrefix(line.Text, "── ") {
			activeHeader = i
		}
	}
	// Commit metadata can precede the first file. Highlight that first boundary
	// while the viewport is still above the patch.
	if activeHeader < 0 {
		for i, line := range source {
			if line.Class == classFileHeader && strings.HasPrefix(line.Text, "── ") {
				activeHeader = i
				break
			}
		}
	}
	sticky = stickyFileHeader(source, offset)
	rows = append([]diffLine(nil), source[offset:min(len(source), offset+max(0, height))]...)
	highlight := func(line diffLine) diffLine {
		line.Class = classSelection
		if line.sideBySide != nil && line.sideBySide.full != nil {
			row := *line.sideBySide
			full := *row.full
			full.Class = classSelection
			row.full = &full
			line.sideBySide = &row
		}
		return line
	}
	if i := activeHeader - offset; i >= 0 && i < len(rows) {
		rows[i] = highlight(rows[i])
	}
	if pinHeader && sticky >= 0 && height > 1 {
		rows[0] = source[sticky]
		if sticky == activeHeader {
			rows[0] = highlight(rows[0])
		}
	} else {
		sticky = -1
	}
	return rows, sticky
}

func (m *Model) presentUnifiedDiffLine(line diffLine, horizontal, width int, prefix string) styledLine {
	return styledLine{Class: sourceLineClass(line.Class), Text: m.syntaxText(line, horizontal, width, prefix)}
}
