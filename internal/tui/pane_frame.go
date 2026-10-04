package tui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
)

type paneBodyBorders struct {
	left, divider, right lineClass
}

// paneBodyRow frames already-presented rows; each view owns selection and scroll.
func (m *Model) paneBodyRow(left, right styledLine, leftWidth, rightWidth int, focus pane, borders paneBodyBorders, mutedColumns ...int) string {
	mutedFrom := 0
	if len(mutedColumns) > 0 {
		mutedFrom = mutedColumns[0]
	}
	if m.Width < 100 {
		if focus == paneDiff {
			return m.frameBodyLine(right.Text, right.Class, rightWidth, true)
		}
		return m.frameListLine(left, leftWidth, true, mutedFrom)
	}
	return m.styleLine(borders.left, "│") +
		m.frameListLine(left, leftWidth, false, mutedFrom) +
		m.styleLine(borders.divider, "│") +
		m.frameBodyLine(right.Text, right.Class, rightWidth, false) +
		m.styleLine(borders.right, "│")
}

func (m *Model) frameListLine(line styledLine, width int, edges bool, mutedFrom int) string {
	if mutedFrom <= 0 {
		return m.frameBodyLine(line.Text, line.Class, width, edges)
	}
	text := clip(line.Text, width)
	primary := ansi.Cut(text, 0, mutedFrom)
	secondary := ansi.Cut(text, mutedFrom, width)
	rendered := m.styleLine(line.Class, primary) + m.styleLine(classMetadata, secondary) + strings.Repeat(" ", max(0, width-visibleWidth(text)))
	if edges {
		border := m.styleLine(paneBorderClass(true), "│")
		return border + rendered + border
	}
	return rendered
}
