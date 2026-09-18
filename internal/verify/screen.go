package verify

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"

	"pr-review/internal/tui"
)

const (
	screenColumns = 120
	screenRows    = 24
	screenCellW   = 8
	screenCellH   = 18
)

func renderScreen(screen string) (string, string) {
	lines := strings.Split(screen, "\n")
	if len(lines) > screenRows {
		lines = lines[:screenRows]
	}
	for i, line := range lines {
		lines[i] = clipRunes(tui.Escape(line), screenColumns)
	}
	text := strings.Join(lines, "\n")

	var b strings.Builder
	b.Grow(1024 + len(text)*2)
	width, height := strconv.Itoa(screenColumns*screenCellW), strconv.Itoa(screenRows*screenCellH)
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	b.WriteString(width)
	b.WriteString(`" height="`)
	b.WriteString(height)
	b.WriteString(`" viewBox="0 0 `)
	b.WriteString(width)
	b.WriteByte(' ')
	b.WriteString(height)
	b.WriteString(`">`)
	b.WriteString(`<rect width="100%" height="100%" fill="#000"/>`)
	b.WriteString(`<g fill="#fff" font-family="monospace" font-size="14">`)
	for row, line := range lines {
		if line == "" {
			continue
		}
		b.WriteString(`<text x="0" y="`)
		b.WriteString(strconv.Itoa((row + 1) * screenCellH))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(line))
		b.WriteString(`</text>`)
	}
	b.WriteString(`</g></svg>`)
	return text, b.String()
}

func clipRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	count := 0
	for i := range s {
		if count == limit {
			return s[:i]
		}
		count++
	}
	return s
}
