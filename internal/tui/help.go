package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) helpLines() []string {
	body := "Your first review\n1. Choose Files above to read the changes.\n2. Use arrows in the focused pane; Tab switches list and diff. Enter activates; Escape goes back.\n3. Press Space for Actions. Use the displayed hotkey, or select with arrows and Enter.\n4. In the diff, Enter opens a comment. Enter in the editor posts now; Ctrl+P queues a local pending comment.\n5. Choose Submit review to send your decision and pending comments after confirmation."
	body += "\n\nFind your way\nClick Overview, Files, Guide, or Commits above to switch views.\nDouble-click a PR or list item to open it; double-click a diff line to comment.\nUse the mouse wheel to scroll, and click visible page or form controls.\nSpace or Ctrl+K: Actions from the review; browse tools and use their hotkeys, or arrows and Enter.\n?: open this help. s: all shortcuts. Escape: return to your place."
	_, health, _ := strings.Cut(m.healthHelpView(), "\n")
	if m.helpExpanded {
		status, reference, _ := strings.Cut(health, "\n\n")
		return m.helpColumns(reference + "\n\nHealth\n" + status)
	}
	lines := m.helpColumns(body)
	lines = append(lines, "")
	return append(lines, m.helpColumns("Health\n"+health)...)
}

// helpColumns keeps sections intact and balances their heights across the screen.
// Narrow terminals retain the same section order in a single column.
func (m *Model) helpColumns(body string) []string {
	columns := 1
	if m.Width >= 160 {
		columns = 3
	} else if m.Width >= 100 {
		columns = 2
	}
	columns = min(columns, len(strings.Split(body, "\n\n")))
	width := max(1, (m.Width-(columns-1)*2)/columns)
	stacks := make([][]string, columns)
	for _, paragraph := range strings.Split(body, "\n\n") {
		title, content, _ := strings.Cut(paragraph, "\n")
		if strings.HasPrefix(paragraph, "Controls and invalid") {
			title, content = "Notes", paragraph
		}
		card := m.helpCard(title, content, width)
		column := 0
		for c := 1; c < columns; c++ {
			if len(stacks[c]) < len(stacks[column]) {
				column = c
			}
		}
		if len(stacks[column]) > 0 {
			stacks[column] = append(stacks[column], "")
		}
		stacks[column] = append(stacks[column], card...)
	}
	height := 0
	for _, stack := range stacks {
		height = max(height, len(stack))
	}
	lines := make([]string, height)
	for row := range lines {
		cells := make([]string, columns)
		for column, stack := range stacks {
			cell := ""
			if row < len(stack) {
				cell = stack[row]
			}
			cells[column] = cell + strings.Repeat(" ", max(0, width-ansi.StringWidth(cell)))
		}
		lines[row] = strings.Join(cells, "  ")
	}
	return lines
}

func (m *Model) helpCard(title, content string, width int) []string {
	// Keep useful text visible even when there is no room for a frame.
	if width < 8 {
		return strings.Split(ansi.Hardwrap(title+"\n"+content, width, false), "\n")
	}
	inner := width - 4
	border := func(s string) string { return m.styleLine(classPaneBorder, s) }
	frame := func(s string) string {
		return border("│ ") + s + strings.Repeat(" ", max(0, inner-ansi.StringWidth(s))) + border(" │")
	}
	lines := []string{border("┌" + strings.Repeat("─", width-2) + "┐")}
	for _, line := range strings.Split(ansi.Wrap(title, inner, ""), "\n") {
		lines = append(lines, frame(m.styleLine(classTitle, line)))
	}
	lines = append(lines, border("├"+strings.Repeat("─", width-2)+"┤"))
	for _, entry := range strings.Split(content, "\n") {
		for i, line := range strings.Split(ansi.Wrap(entry, inner, ""), "\n") {
			styled := m.styleLine(classContext, line)
			if keys, desc, ok := strings.Cut(line, ": "); i == 0 && ok && len(keys) <= 24 {
				styled = m.styleLine(classHunk, keys+":") + m.styleLine(classContext, " "+desc)
			}
			lines = append(lines, frame(styled))
		}
	}
	return append(lines, border("└"+strings.Repeat("─", width-2)+"┘"))
}

func (m *Model) helpBodyHeight() int { return max(1, m.Height-2) }

func (m *Model) helpViewport() string {
	lines := m.helpLines()
	height := m.helpBodyHeight()
	m.helpScroll = min(max(0, m.helpScroll), max(0, len(lines)-height))
	visible := lines[m.helpScroll:min(len(lines), m.helpScroll+height)]
	footer := pageMouseFooter(m.helpMouseControls(), " · ")
	if m.Height == 1 {
		return clip(footer, m.Width)
	}
	if m.Height == 2 {
		return clip("Health & help", m.Width) + "\n" + clip(footer, m.Width)
	}
	return m.styleLine(classTitle, clip("Health & help", m.Width)) + "\n" + strings.Join(visible, "\n") + "\n" + m.styleLine(classMetadata, clip(footer, m.Width))
}

func (m *Model) helpKey(key string) {
	var delta int
	switch key {
	case "s":
		m.helpExpanded = !m.helpExpanded
		m.helpScroll = 0
		return
	case "esc":
		m.pop()
		return
	case "j", "down":
		delta = 1
	case "k", "up":
		delta = -1
	case "d":
		delta = fastScrollStep
	case "pgdown":
		delta = m.helpBodyHeight()
	case "u":
		delta = -fastScrollStep
	case "pgup":
		delta = -m.helpBodyHeight()
	case "home":
		m.helpScroll = 0
		return
	case "end":
		m.helpScroll = max(0, len(m.helpLines())-m.helpBodyHeight())
		return
	default:
		return
	}
	m.helpScroll = min(max(0, m.helpScroll+delta), max(0, len(m.helpLines())-m.helpBodyHeight()))
}
