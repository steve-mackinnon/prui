package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

func TestLightThemeCatalogSelectsLightMarkdownBaseline(t *testing.T) {
	body := "# Heading\n\nordinary paragraph e\u0301 **bold** and _emphasis_\n\n> quoted text\n\n[normal link](https://example.com)\n\n`inline`\n\n```go\nfunc main() {}\n```"
	lightBaseline, err := renderDescriptionMarkdownForTheme(body, 72, theme.Light)
	if err != nil {
		t.Fatal(err)
	}
	darkBaseline, err := renderDescriptionMarkdownForTheme(body, 72, theme.Dark)
	if err != nil {
		t.Fatal(err)
	}
	expectedText := ansi.Strip(strings.Join(lightBaseline, "\n"))
	if strings.Join(lightBaseline, "\n") == strings.Join(darkBaseline, "\n") {
		t.Fatal("fixture does not distinguish light and dark Markdown palettes")
	}
	for _, name := range []string{"ayu", "gh-light", "gruvbox-light", "one-light", "rose-pine-dawn", "vscode-light"} {
		t.Run(name, func(t *testing.T) {
			inherited, err := theme.Resolve(name, map[theme.Token]string{theme.Foreground: "default", theme.Background: "default"})
			if err != nil {
				t.Fatal(err)
			}
			inheritedLines, err := renderDescriptionMarkdownWithTheme(body, 72, inherited)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(inheritedLines, "\n") != strings.Join(lightBaseline, "\n") {
				t.Fatal("light preset did not select existing Glamour light baseline")
			}
			for _, overrides := range []map[theme.Token]string{nil, {theme.Foreground: "default"}, {theme.Background: "default"}, {theme.Foreground: "#123456", theme.Background: "#fafafa"}} {
				palette, err := theme.Resolve(name, overrides)
				if err != nil {
					t.Fatal(err)
				}
				lines, err := renderDescriptionMarkdownWithTheme(body, 72, palette)
				if err != nil {
					t.Fatal(err)
				}
				output := strings.Join(lines, "\n")
				if ansi.Strip(output) != expectedText {
					t.Fatal("light theme changed Markdown content or wrapping")
				}
				if !strings.Contains(output, "\x1b]8;") || !strings.Contains(output, "https://example.com") {
					t.Fatal("light theme removed renderer hyperlink")
				}
				for _, line := range lines {
					width := ansi.StringWidth(line)
					if width == 0 {
						continue
					}
					cells := themeCanvasBuffer(line, width, 1)
					for x := 0; x < width; x++ {
						if cells.CellAt(x, 0).Style.Bg != nil {
							t.Fatalf("light theme left Markdown background rectangle at column %d", x)
						}
					}
				}
			}
		})
	}
	// Local adaptation must leave Glamour's shared light baseline untouched.
	after, err := renderDescriptionMarkdownForTheme(body, 72, theme.Light)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(after, "\n") != strings.Join(lightBaseline, "\n") {
		t.Fatal("light preset mutated shared Markdown style")
	}
}
