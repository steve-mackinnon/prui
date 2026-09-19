package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
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

// renderLoadingModal places a compact, ASCII-safe modal over a clipped base
// screen. It returns plain display lines; View applies the existing styling
// contract after all dynamic text has already been escaped.
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
	content := []string{
		"╭" + strings.Repeat("─", modalWidth-2) + "╮",
		modalLine("  "+loadingSpinner(modal.frame)+"  "+title, inside),
		modalLine("", inside),
		modalLine(center(loadingBar(modal.frame), inside), inside),
		modalLine("  ↳ "+Escape(modal.notice), inside),
		modalLine("", inside),
	}
	if modal.cancelable {
		content = append(content, modalLine("  esc: cancel", inside))
	}
	content = append(content, "╰"+strings.Repeat("─", modalWidth-2)+"╯")

	startRow := max(0, (height-len(content))/2)
	left := max(0, (width-modalWidth)/2)
	for i, line := range content {
		if startRow+i >= len(lines) {
			break
		}
		lines[startRow+i] = strings.Repeat(" ", left) + line
	}
	return strings.Join(lines, "\n")
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
