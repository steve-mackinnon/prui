package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	loadingBarWidth   = 18
	loadingCometWidth = 6
)

type loadingModal struct {
	active     bool
	cancelable bool
	frame      int
	notice     string
	title      string
}

type loadingTick struct{ result <-chan tea.Msg }

func nextLoadingTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return loadingTick{} })
}

func waitForLoading(result <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case message := <-result:
			return message
		case <-timer.C:
			return loadingTick{result: result}
		}
	}
}

func loadingBar(frame int) string {
	cells := make([]rune, loadingBarWidth)
	for i := range cells {
		cells[i] = '▱'
	}
	for i := 0; i < loadingCometWidth; i++ {
		cells[(frame+i)%len(cells)] = '▰'
	}
	return string(cells)
}

func compactLoadingBar(frame int) string {
	cells := make([]byte, loadingBarWidth)
	for i := range cells {
		cells[i] = '-'
	}
	for i := 0; i < loadingCometWidth; i++ {
		cells[(frame+i)%len(cells)] = '='
	}
	return "[" + string(cells) + "]"
}

func loadingSpinner(frame int) string {
	return []string{"◐", "◓", "◑", "◒"}[frame%4]
}

// renderLoadingModal draws the card at fixed viewport coordinates over the
// clipped background. Both layers keep their own cells, including the text
// to the right of the card.
func renderLoadingModal(width, height int, background string, modal loadingModal) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 20 || height < 6 {
		return renderCompactLoading(width, height, modal)
	}

	lines := viewportLines(background, width, height)
	modalWidth := min(60, max(36, width-4))
	modalWidth = min(modalWidth, width)
	inside := max(1, modalWidth-4)
	title := modal.title
	if title == "" {
		title = "Loading"
	}
	fixedLineCount := 6
	if modal.cancelable {
		fixedLineCount++
	}
	if height <= fixedLineCount {
		return renderCompactLoading(width, height, modal)
	}
	// Reserve a stable top row for the minimum card. Longer notices grow
	// downward rather than recentering the overlay on every progress update.
	startRow := max(0, (height-fixedLineCount-1)/2)

	noticePrefix := "  ↳ "
	noticeWidth := max(1, inside-visibleWidth(noticePrefix))
	noticeLines := strings.Split(ansi.Wrap(Escape(modal.notice), noticeWidth, ""), "\n")
	maxNoticeLines := height - startRow - fixedLineCount
	if len(noticeLines) > maxNoticeLines {
		noticeLines = noticeLines[:maxNoticeLines]
		last := len(noticeLines) - 1
		noticeLines[last] = ansi.Truncate(noticeLines[last], max(0, noticeWidth-1), "") + "…"
	}

	content := []string{
		"╭" + strings.Repeat("─", modalWidth-2) + "╮",
		modalLine("  "+loadingSpinner(modal.frame)+"  "+title, inside),
		modalLine("", inside),
		modalLine(center(loadingBar(modal.frame), inside), inside),
	}
	for i, line := range noticeLines {
		prefix := strings.Repeat(" ", visibleWidth(noticePrefix))
		if i == 0 {
			prefix = noticePrefix
		}
		content = append(content, modalLine(prefix+line, inside))
	}
	content = append(content, modalLine("", inside))
	if modal.cancelable {
		content = append(content, modalLine("  esc: cancel", inside))
	}
	content = append(content, "╰"+strings.Repeat("─", modalWidth-2)+"╯")

	left := max(0, (width-modalWidth)/2)
	canvas := lipgloss.NewCanvas(width, height)
	return canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Join(lines, "\n")),
		lipgloss.NewLayer(strings.Join(content, "\n")).X(left).Y(startRow),
	)).Render()
}

func renderCompactLoading(width, height int, modal loadingModal) string {
	lines := []string{clip(compactLoadingBar(modal.frame), width), clip(Escape(modal.notice), width)}
	if modal.cancelable && len(lines) < height {
		lines = append(lines, clip("esc: cancel", width))
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}

func modalLine(text string, width int) string {
	text = clip(text, width)
	return "│ " + text + strings.Repeat(" ", max(0, width-visibleWidth(text))) + " │"
}

func viewportLines(background string, width, height int) []string {
	base := strings.Split(background, "\n")
	lines := make([]string, height)
	for i := range lines {
		if i < len(base) {
			lines[i] = clip(base[i], width)
		}
	}
	return lines
}

func center(text string, width int) string {
	text = clip(text, width)
	padding := max(0, width-visibleWidth(text))
	return strings.Repeat(" ", padding/2) + text + strings.Repeat(" ", padding-padding/2)
}
