package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (m *Model) helpLines() []string {
	_, body, _ := strings.Cut(m.healthHelpView(), "\n")
	body += "\n\nMouse\nClick: select item or diff cell; Enter: activate.\nClick view tabs to switch. Drag the file/diff divider to resize (wide terminals).\nScroll over the file/guide list or diff to move that pane. Use your terminal selection modifier to copy text.\nPanel width lasts for this run only."
	return strings.Split(ansi.Wrap(body, max(1, m.Width), ""), "\n")
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
		return "Health & help\n" + clip(footer, m.Width)
	}
	return m.styleLine(classTitle, "Health & help") + "\n" + strings.Join(visible, "\n") + "\n" + m.styleLine(classMetadata, clip(footer, m.Width))
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
	case "d", "pgdown":
		delta = m.helpBodyHeight()
	case "u", "pgup":
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
