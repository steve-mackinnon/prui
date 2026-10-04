package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

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
	painted := m.themeContentBase(content)
	if m.top() != pageThemePicker || m.Loading || m.Busy || m.Err != nil {
		return painted
	}
	left, top, width, available := themePickerBounds(m.Width, m.Height)
	if !available || (m.Session == nil && len(m.Stack) < 2) {
		return painted
	}

	// Render a temporary model so previewing never changes the saved palette or
	// the underlying page, including its scroll position and Markdown cache.
	preview := *m
	if m.reviewTabState != nil {
		state := *m.reviewTabState
		preview.reviewTabState = &state
	}
	if len(preview.Stack) > 0 {
		preview.Stack = preview.Stack[:len(preview.Stack)-1]
	}
	candidate, err := theme.Resolve(theme.BuiltInNames()[m.ThemePicker.Index], m.themeOverrides)
	if err != nil {
		return painted
	}
	preview.theme = candidate
	preview.styles = stylesFor(candidate)
	preview.descriptionCache = descriptionRenderCache{}
	background := preview.View().Content

	rows := strings.Split(painted, "\n")
	height := min(m.Height, len(themePickerRows())+7)
	if m.Width >= 60 && m.Height >= 18 {
		height = min(m.Height, len(themePickerRows())+10)
	}
	card := make([]string, 0, height)
	for y := top; y < min(top+height, len(rows)); y++ {
		card = append(card, ansi.Cut(rows[y], left, left+width))
	}
	canvas := lipgloss.NewCanvas(m.Width, m.Height)
	return canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(background),
		lipgloss.NewLayer(strings.Join(card, "\n")).X(left).Y(top),
	)).Render()
}

func (m *Model) themeContentBase(content string) string {
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
