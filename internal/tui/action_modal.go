package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderActionModal keeps short, focused actions over their review context.
// Tiny terminals use the body's compact layout without spending cells on chrome.
func renderActionModal(width, height int, background, body string, painters ...func(string) string) string {
	if width < 20 || height < 8 {
		return paintModalSurface(strings.Join(viewportLines(body, width, height), "\n"), painters)
	}
	inside := min(96, width-8)
	rows := []string{"╭" + strings.Repeat("─", inside+2) + "╮"}
	for _, line := range strings.Split(body, "\n") {
		rows = append(rows, modalLine(line, inside))
	}
	rows = append(rows, "╰"+strings.Repeat("─", inside+2)+"╯")
	canvas := lipgloss.NewCanvas(width, height)
	return canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Join(viewportLines(background, width, height), "\n")),
		lipgloss.NewLayer(paintModalSurface(strings.Join(rows, "\n"), painters)).X(max(0, (width-inside-4)/2)).Y(max(0, (height-len(rows))/2)),
	)).Render()
}

func (m *Model) reviewFormModalView() string {
	// Render against the card's inner dimensions so decisions, errors and the
	// final confirmation hint remain within the frame on short terminals.
	form := *m
	if m.Width >= 20 && m.Height >= 8 {
		form.Width = min(96, m.Width-8)
		form.Height = min(22, m.Height-2)
	}
	return renderActionModal(m.Width, m.Height, m.actionModalBackground(), form.reviewFormView(), m.modalSurface)
}

func (m *Model) actionModalBackground() string {
	if m.Session == nil {
		return "No review loaded. q: quit"
	}
	return m.reviewView()
}
