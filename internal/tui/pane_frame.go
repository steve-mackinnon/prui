package tui

type paneBodyBorders struct {
	left, divider, right lineClass
}

// paneBodyRow frames already-presented rows; each view owns selection and scroll.
func (m *Model) paneBodyRow(left, right styledLine, leftWidth, rightWidth int, focus pane, borders paneBodyBorders) string {
	if m.Width < 100 {
		if focus == paneDiff {
			return m.frameBodyLine(right.Text, right.Class, rightWidth, true)
		}
		return m.frameBodyLine(left.Text, left.Class, leftWidth, true)
	}
	return m.styleLine(borders.left, "│") +
		m.frameBodyLine(left.Text, left.Class, leftWidth, false) +
		m.styleLine(borders.divider, "│") +
		m.frameBodyLine(right.Text, right.Class, rightWidth, false) +
		m.styleLine(borders.right, "│")
}
