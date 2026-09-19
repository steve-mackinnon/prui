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
	cells := make([]byte, loadingBarWidth)
	for i := range cells {
		cells[i] = '-'
	}
	for i := 0; i < loadingCometWidth; i++ {
		cells[(frame+i)%len(cells)] = '='
	}
	return "[" + string(cells) + "]"
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
		"+" + strings.Repeat("-", modalWidth-2) + "+",
		"| " + clip(title, inside) + strings.Repeat(" ", max(0, inside-visibleWidth(clip(title, inside)))) + " |",
		"|" + strings.Repeat(" ", modalWidth-2) + "|",
		"| " + center(loadingBar(modal.frame), inside) + " |",
		"| " + clip(Escape(modal.notice), inside) + strings.Repeat(" ", max(0, inside-visibleWidth(clip(Escape(modal.notice), inside)))) + " |",
		"|" + strings.Repeat(" ", modalWidth-2) + "|",
	}
	if modal.cancelable {
		content = append(content, "| "+clip("esc: cancel", inside)+strings.Repeat(" ", max(0, inside-visibleWidth("esc: cancel")))+" |")
	}
	content = append(content, "+"+strings.Repeat("-", modalWidth-2)+"+")

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
	lines := []string{clip(loadingBar(modal.frame), width), clip(Escape(modal.notice), width)}
	if modal.cancelable && len(lines) < height {
		lines = append(lines, clip("esc: cancel", width))
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
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
