package tui

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// paintThemeCanvas fills inherited color channels in trusted renderer output.
// Layout and source escaping happen before this presentation boundary. Nil
// channels inherit terminal defaults; explicit semantic colors, backgrounds,
// focus attributes and hyperlinks survive the cell round trip. The caller must
// skip this step for colorless output so its logical bytes remain unchanged.
func paintThemeCanvas(content string, width, height int, foreground, background color.Color) string {
	if (foreground == nil && background == nil) || width <= 0 || height <= 0 {
		return content
	}
	canvas := themeCanvasBuffer(content, width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			cell := canvas.CellAt(x, y)
			// Wide graphemes own their continuation columns. Styling the zero-width
			// placeholders would turn them into independent cells during rendering.
			if cell.Width == 0 {
				continue
			}
			if cell.Style.Fg == nil {
				cell.Style.Fg = foreground
			}
			if cell.Style.Bg == nil {
				cell.Style.Bg = background
			}
		}
	}
	return canvas.Render()
}

// themeCanvasBuffer decomposes trusted renderer output into cells for final
// presentation transforms, preserving graphemes and hyperlink boundaries.
func themeCanvasBuffer(content string, width, height int) uv.ScreenBuffer {
	canvas := uv.NewScreenBuffer(width, height)
	canvas.Method = ansi.GraphemeWidth
	drawThemeContent(canvas, content)
	return canvas
}

// drawThemeContent handles only trusted SGR/link renderer output and newline.
// DecodeSequence has an ASCII fast path that separates trailing combining marks;
// decode printable graphemes directly so the round trip preserves their bytes.
func drawThemeContent(canvas uv.ScreenBuffer, content string) {
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	var style uv.Style
	var link uv.Link
	var state byte
	x, y := 0, 0
	for len(content) > 0 && y < canvas.Height() {
		var seq string
		var width, n int
		if state == ansi.NormalState && ((content[0] >= 0x20 && content[0] < 0x7f) || content[0] >= 0xc0) {
			seq, width = ansi.FirstGraphemeCluster(content, ansi.GraphemeWidth)
			n = len(seq)
		} else {
			seq, width, n, state = ansi.DecodeSequence(content, state, parser)
		}
		if n == 0 {
			break
		}
		content = content[n:]
		switch {
		case width > 0:
			if x < canvas.Width() {
				canvas.SetCell(x, y, &uv.Cell{Content: seq, Width: width, Style: style, Link: link})
			}
			x += width
		case seq == "\n":
			x = 0
			y++
		case seq == "\r":
			x = 0
		case ansi.HasCsiPrefix(seq) && parser.Command() == 'm':
			uv.ReadStyle(parser.Params(), &style)
		case ansi.HasOscPrefix(seq) && parser.Command() == 8:
			uv.ReadLink(parser.Data(), &link)
		}
	}
}
