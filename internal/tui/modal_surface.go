package tui

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"prui/internal/theme"
)

// modalSurface paints the complete card, including padding and borders, while
// retaining semantic foreground colors and explicit sample backgrounds.
func (m *Model) modalSurface(content string) string {
	if m.colorProfile <= colorprofile.Ascii {
		return content
	}
	lines := strings.Split(content, "\n")
	width := 0
	for _, line := range lines {
		width = max(width, visibleWidth(line))
	}
	return paintThemeCanvas(content, width, len(lines), themeBaseColor(m.theme, theme.Foreground), modalBackground(m.theme))
}

func modalBackground(palette theme.Theme) color.Color {
	base := themeBaseColor(palette, theme.Background)
	if base == nil {
		if palette.IsLight() {
			return color.RGBA{R: 228, G: 228, B: 228, A: 255}
		}
		return color.RGBA{R: 48, G: 48, B: 48, A: 255}
	}
	r, g, b, _ := base.RGBA()
	blend := func(channel uint32) uint8 {
		v := uint16((channel >> 8) & 0xff)
		if palette.IsLight() {
			v = v * 95 / 100
		} else {
			v += (255 - v) * 10 / 100
		}
		return uint8(v & 0xff)
	}
	return color.RGBA{R: blend(r), G: blend(g), B: blend(b), A: 255}
}

// Optional painting keeps standalone geometry renderers usable without a model.
func paintModalSurface(content string, painters []func(string) string) string {
	if len(painters) > 0 {
		return painters[0](content)
	}
	return content
}
