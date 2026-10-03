package tui

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"prui/internal/theme"
)

// themeBaseColor translates the validated inherited channel into the nil
// default used by canvas composition. Semantic token colors remain explicit.
func themeBaseColor(palette theme.Theme, token theme.Token) color.Color {
	if palette.Syntax(token) == "default" {
		return nil
	}
	c, _ := palette.Color(token)
	return c
}

func (m *Model) themeContent(content string) string {
	if m.colorProfile <= colorprofile.Ascii {
		return content
	}
	foreground, background := themeBaseColor(m.theme, theme.Foreground), themeBaseColor(m.theme, theme.Background)
	painted := paintThemeCanvas(content, m.Width, m.Height, foreground, background)
	// Candidate inheritance must be applied after the active frame base. Nil
	// channels in this independent sample mean terminal defaults, not active colors.
	if (foreground == nil && background == nil) || m.top() != pageThemePicker || m.Width < 60 || m.Height < 18 || m.Loading || m.Busy || m.Err != nil {
		return painted
	}
	left, top, sampleWidth := themePickerSampleBounds(m.Width, m.Height, m.ThemePicker.Index)
	sample := m.themeCandidateSample(theme.BuiltInNames()[m.ThemePicker.Index], sampleWidth)
	canvas := themeCanvasBuffer(painted, m.Width, m.Height)
	candidate := themeCanvasBuffer(strings.Join(sample, "\n"), sampleWidth, len(sample))
	for y := 0; y < len(sample); y++ {
		for x := 0; x < sampleWidth; x++ {
			cell := candidate.CellAt(x, y)
			if cell.Width > 0 {
				canvas.SetCell(left+x, top+y, cell)
			}
		}
	}
	return canvas.Render()
}
