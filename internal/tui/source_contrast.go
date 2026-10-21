package tui

import (
	"image/color"
	"math"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"prui/internal/theme"
)

// Inherited terminal backgrounds use the same family baseline as diff fills.
// The terminal's actual default colors are not available to the renderer.
func sourceBackground(t theme.Theme, class lineClass) color.Color {
	switch class {
	case classAdded, classAddedSource:
		return diffBackground(t, theme.Added)
	case classRemoved, classRemovedSource:
		return diffBackground(t, theme.Removed)
	}
	if background := themeBaseColor(t, theme.Background); background != nil {
		return background
	}
	if t.IsLight() {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{24, 24, 24, 255}
}

func sourceLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	linear := func(v uint32) float64 {
		x := float64(v) / 65535
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

func sourceContrast(a, b color.Color) float64 {
	x, y := sourceLuminance(a), sourceLuminance(b)
	return (math.Max(x, y) + 0.05) / (math.Min(x, y) + 0.05)
}

// Lift or darken only unreadable foregrounds, retaining as much of their hue
// as possible. Default foregrounds remain inherited, never explicit black.
func readableSourceColor(fg, bg color.Color, profile colorprofile.Profile) color.Color {
	if fg == nil || profile <= colorprofile.Ascii {
		return fg
	}
	bg = profile.Convert(bg)
	if sourceContrast(profile.Convert(fg), bg) >= 4.5 {
		return fg
	}
	target := color.RGBA{255, 255, 255, 255}
	if sourceContrast(color.Black, bg) > sourceContrast(target, bg) {
		target = color.RGBA{0, 0, 0, 255}
	}
	r, g, b, _ := fg.RGBA()
	mix := func(amount int) color.Color {
		channel := func(v uint32, to uint8) uint8 {
			value := (int(v>>8)*(255-amount) + int(to)*amount) / 255
			if value < 0 {
				return 0
			}
			if value > 255 {
				return 255
			}
			return uint8(value)
		}
		return profile.Convert(color.RGBA{channel(r, target.R), channel(g, target.G), channel(b, target.B), 255})
	}
	low, high := 0, 255
	for low < high {
		mid := (low + high) / 2
		if sourceContrast(mix(mid), bg) >= 4.5 {
			high = mid
		} else {
			low = mid + 1
		}
	}
	return mix(high)
}

func (m *Model) readableSource(text string, class lineClass) string {
	width := visibleWidth(text)
	if width == 0 {
		return text
	}
	canvas := themeCanvasBuffer(text, width, 1)
	background := sourceBackground(m.theme, class)
	type pair struct{ fg, bg color.Color }
	colors := make(map[pair]color.Color)
	for x := 0; x < width; x++ {
		cell := canvas.CellAt(x, 0)
		if cell.Width == 0 || cell.Style.Fg == nil {
			continue
		}
		bg := background
		if cell.Style.Bg != nil {
			bg = cell.Style.Bg
		}
		key := pair{cell.Style.Fg, bg}
		fg, ok := colors[key]
		if !ok {
			fg = readableSourceColor(key.fg, key.bg, m.colorProfile)
			colors[key] = fg
		}
		cell.Style.Fg = fg
	}
	// Canvas rendering omits trailing default spaces; source fragments must
	// retain them when word emphasis or search splits a row at whitespace.
	rendered := canvas.Render()
	return rendered + strings.Repeat(" ", max(0, width-visibleWidth(rendered)))
}
