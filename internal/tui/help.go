package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) helpLines() []string {
	_, body, _ := strings.Cut(m.healthHelpView(), "\n")
	body += "\n\nDiff display\nL: toggle line numbers in Files, Guides, and Commits for this review tab.\n\nFiles\n/: filter by filename or path. Enter: keep filter and navigate. Escape: clear filter.\n\n\nCommits (3)\nj/k or arrows: select commit in list, scroll in diff. n/p: next/previous commit in either pane.\nEnter/l: focus diff; Escape/h: return to list. d/u: scroll 15 lines. PgUp/PgDn: page. Home/End: first/last item or diff row.\n[/]: resize commit rail at 100+ columns. Below 100 columns, focus selects the visible pane. Commit diffs are read-only.\n\nDiscussions (D)\nj/k: select thread; Enter: detail; o: view original captured commit; Escape: return. c: refresh comments and discussions.\nOutdated describes the current anchor, independently of Resolved. Missing context remains readable here.\n\nMouse\nClick: select item or diff cell; Enter: activate.\nHover URLs to underline; click to open in your browser.\nClick view tabs to switch. Drag the file/diff divider to resize (wide terminals).\nScroll over the file/guide list or diff to move that pane. Use your terminal selection modifier to copy text.\nPanel width lasts for this run only."
	return m.helpColumns(body)
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
	width := max(1, (m.Width-(columns-1)*2)/columns)
	stacks := make([][]string, columns)
	for i, paragraph := range strings.Split(body, "\n\n") {
		title, content, _ := strings.Cut(paragraph, "\n")
		if i == 0 {
			title, content = "Health", paragraph
		} else if strings.HasPrefix(paragraph, "Controls and invalid") {
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
	footer := "↑/↓: scroll · PgUp/PgDn: page · esc: back"
	if m.Width < 40 {
		footer = "↑↓ PgUp/PgDn · esc: back"
	}
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
