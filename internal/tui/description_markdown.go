package tui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"pr-review/internal/theme"
)

// renderDescriptionMarkdown converts a frozen PR description into safe,
// width-aware terminal lines. It intentionally accepts no Model state so its
// output can be cached by the Description view.
func renderDescriptionMarkdown(body string, width int) ([]string, error) {
	return renderDescriptionMarkdownForTheme(body, width, theme.Dark)
}

// renderDescriptionMarkdownForTheme uses Glamour's light palette only when
// the active TUI theme has a light background. Other themes retain the dark
// palette, which is legible on terminal and high-contrast backgrounds.
func renderDescriptionMarkdownForTheme(body string, width int, themeName string) ([]string, error) {
	if width < 1 {
		width = 1
	}

	style := styles.DarkStyle
	if themeName == theme.Light {
		style = styles.LightStyle
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create markdown renderer: %w", err)
	}

	rendered, err := renderer.Render(normalizeDescriptionControls(body))
	if err != nil {
		return nil, fmt.Errorf("render markdown description: %w", err)
	}
	return strings.Split(strings.TrimSuffix(rendered, "\n"), "\n"), nil
}

func normalizeDescriptionControls(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	var safe strings.Builder
	safe.Grow(len(body))
	for _, r := range body {
		switch {
		case r == '\n' || r == '\t':
			safe.WriteRune(r)
		case r == '<':
			// Glamour sanitizes raw HTML away. Encode it before parsing so it
			// remains visible, inert text as approved for PR descriptions.
			safe.WriteString("&lt;")
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&safe, `\x%02x`, r)
		default:
			safe.WriteRune(r)
		}
	}
	return safe.String()
}
