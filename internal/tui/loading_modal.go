package tui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const loadingAnimationFrames = 60

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

// loadingOrb projects rotating particles onto a Braille grid. Two horizontal
// dots and four vertical dots per cell keep the sphere round in terminal fonts.
// Its fixed canvas prevents the breathing motion from shifting nearby text.
func loadingOrb(frame, rows int) []string {
	const columns = 18
	phase := 2 * math.Pi * float64(frame%loadingAnimationFrames) / loadingAnimationFrames
	radius := float64(rows*4-2) / 2 * (0.82 + 0.16*math.Sin(phase))
	particles := 96
	horizontalScale := 1.7
	if rows == 1 {
		particles = 8
		horizontalScale = 5
	}
	cells := make([][]rune, rows)
	for y := range cells {
		cells[y] = make([]rune, columns)
		for x := range cells[y] {
			cells[y][x] = ' '
		}
	}
	bits := [4][2]rune{{1, 8}, {2, 16}, {4, 32}, {64, 128}}
	for i := 0; i < particles; i++ {
		// Evenly spaced latitudes with a golden-angle distribution avoid clumps.
		z := 1 - 2*(float64(i)+0.5)/float64(particles)
		angle := float64(i)*math.Pi*(3-math.Sqrt(5)) + phase
		ring := math.Sqrt(1 - z*z)
		x := ring * math.Cos(angle)
		depth := ring * math.Sin(angle)
		// Tilt the sphere so particles trace visible circular paths.
		y := z*0.86 + depth*0.5
		px := int(math.Round(float64(columns) - 0.5 + x*radius*horizontalScale))
		py := int(math.Round(float64(rows*2) - 0.5 + y*radius))
		if px < 0 || px >= columns*2 || py < 0 || py >= rows*4 {
			continue
		}
		if cells[py/4][px/2] == ' ' {
			cells[py/4][px/2] = '\u2800'
		}
		cells[py/4][px/2] |= bits[py%4][px%2]
	}
	lines := make([]string, rows)
	for y := range cells {
		lines[y] = string(cells[y])
	}
	return lines
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
	orbRows := 1
	if height >= 14 {
		orbRows = 5
	}
	fixedLineCount := 5 + orbRows
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
		modalLine("  "+title, inside),
		modalLine("", inside),
	}
	for _, line := range loadingOrb(modal.frame, orbRows) {
		content = append(content, modalLine(center(line, inside), inside))
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
	lines := []string{clip(strings.TrimSpace(loadingOrb(modal.frame, 1)[0]), width), clip(Escape(modal.notice), width)}
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
