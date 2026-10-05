package tui

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var browserURLPattern = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\\]+`)

func validBrowserURL(raw string) bool {
	if strings.Contains(raw, "\\") || strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

func urlMatches(text string) [][2]int {
	var spans [][2]int
	for _, match := range browserURLPattern.FindAllStringIndex(text, -1) {
		raw := strings.TrimRight(text[match[0]:match[1]], ".,;:!?")
		for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}} {
			for strings.HasSuffix(raw, pair[1]) && strings.Count(raw, pair[1]) > strings.Count(raw, pair[0]) {
				raw = strings.TrimSuffix(raw, pair[1])
			}
		}
		if validBrowserURL(raw) {
			spans = append(spans, [2]int{match[0], match[0] + len(raw)})
		}
	}
	return spans
}

// Match logical text across syntax SGR boundaries, then add links without
// changing graphemes, foregrounds, backgrounds, or existing Markdown links.
func linkifyURLs(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		plain := ansi.Strip(line)
		spans := urlMatches(plain)
		if len(spans) == 0 {
			continue
		}
		width := ansi.StringWidth(line)
		if width == 0 {
			continue
		}
		canvas := themeCanvasBuffer(line, width, 1)
		for _, span := range spans {
			left, right := ansi.StringWidth(plain[:span[0]]), ansi.StringWidth(plain[:span[1]])
			for x := left; x < right; x++ {
				cell := canvas.CellAt(x, 0)
				if cell.Link.URL == "" {
					cell.Link = uv.Link{URL: plain[span[0]:span[1]]}
				}
			}
		}
		rendered := canvas.Render()
		lines[i] = rendered + strings.Repeat(" ", max(0, width-ansi.StringWidth(rendered)))
	}
	return strings.Join(lines, "\n")
}

func hoverURL(content string, width, height, x, y int) string {
	if x < 0 || y < 0 || x >= width || y >= height {
		return content
	}
	canvas := themeCanvasBuffer(content, width, height)
	target := canvas.CellAt(x, y).Link.URL
	if !validBrowserURL(target) {
		return content
	}
	// Underline visible occurrences, including wrapped Markdown labels.
	for row := 0; row < height; row++ {
		for col := 0; col < width; col++ {
			cell := canvas.CellAt(col, row)
			if cell.Width > 0 && cell.Link.URL == target {
				cell.Style.Underline = uv.UnderlineSingle
			}
		}
	}
	return canvas.Render()
}

func (m *Model) linksAvailable() bool {
	return m.reviewTabState != nil && m.mouseAvailable() && m.top() == pageReview && !m.searchOpen() && !m.commitFilter.open && !m.drag.active
}

func (m *Model) mouseURL(msg tea.MouseMsg) (tea.Cmd, bool) {
	event := msg.Mouse()
	m.linkMouseX, m.linkMouseY = event.X, event.Y
	m.linkMouseValid = event.Mod == 0
	if !m.linksAvailable() || event.Mod != 0 {
		return nil, false
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft || event.X < 0 || event.Y < 0 || event.X >= m.Width || event.Y >= m.Height {
		return nil, false
	}
	canvas := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
	target := canvas.CellAt(event.X, event.Y).Link.URL
	if !validBrowserURL(target) {
		return nil, false
	}
	return func() tea.Msg {
		if err := openBrowserURL(target); err != nil {
			return Notice("Could not open URL in browser.")
		}
		return nil
	}, true
}

func openBrowserURL(target string) error {
	if !validBrowserURL(target) {
		return fmt.Errorf("invalid browser URL")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "linux":
		command = exec.Command("xdg-open", target)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		return fmt.Errorf("unsupported browser platform")
	}
	// No shell interpolation, and browser diagnostics never reach terminal output.
	return command.Run()
}

// Resolve destinations against the complete source line before panning/clipping.
func linkSourceURLs(rendered, source string, horizontal, prefixWidth int) string {
	spans := urlMatches(source)
	if len(spans) == 0 {
		return rendered
	}
	width := ansi.StringWidth(rendered)
	if width == 0 {
		return rendered
	}
	runes := []rune(source)
	start := len(string(runes[:min(max(horizontal, 0), len(runes))]))
	removedWidth := ansi.StringWidth(source[:start])
	canvas := themeCanvasBuffer(rendered, width, 1)
	for _, span := range spans {
		left := max(prefixWidth, prefixWidth+ansi.StringWidth(source[:span[0]])-removedWidth)
		right := min(width, prefixWidth+ansi.StringWidth(source[:span[1]])-removedWidth)
		for x := left; x < right; x++ {
			canvas.CellAt(x, 0).Link = uv.Link{URL: source[span[0]:span[1]]}
		}
	}
	result := canvas.Render()
	return result + strings.Repeat(" ", max(0, width-ansi.StringWidth(result)))
}
