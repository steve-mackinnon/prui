package tui

import (
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"prui/internal/theme"
)

// selectedCommentBorder changes only the card frame, preserving body styles,
// reaction labels, and the cursor gutter.
func (m *Model) selectedCommentBorder(content string, top bool) string {
	width := visibleWidth(content)
	if width == 0 {
		return content
	}
	canvas := themeCanvasBuffer(content, width, 1)
	left := 0
	for left < width {
		text := canvas.CellAt(left, 0).Content
		if text != " " && text != "›" {
			break
		}
		left++
	}
	if left == width {
		return content
	}
	edge := canvas.CellAt(left, 0).Content
	if edge != "+" && edge != "|" {
		return content
	}
	right := width - 1
	for right > left && canvas.CellAt(right, 0).Content == " " {
		right--
	}
	accent := themeBaseColor(m.theme, theme.FocusedBorder)
	paint := func(x int, glyph string) {
		cell := canvas.CellAt(x, 0)
		cell.Content = glyph
		if m.colorProfile > colorprofile.Ascii {
			cell.Style.Fg = accent
			cell.Style.Attrs |= uv.AttrBold
		}
	}
	if edge == "|" {
		paint(left, "│")
		if right > left && canvas.CellAt(right, 0).Content == "|" {
			paint(right, "│")
		}
	} else {
		start, end := "└", "┘"
		if top {
			start, end = "┌", "┐"
		}
		paint(left, start)
		if right > left && canvas.CellAt(right, 0).Content == "+" {
			paint(right, end)
		}
		inLabel := false
		for x := left + 1; x < right; x++ {
			switch canvas.CellAt(x, 0).Content {
			case "[":
				inLabel = true
			case "]":
				inLabel = false
			case "-":
				if !inLabel {
					paint(x, "─")
				}
			}
		}
		if canvas.CellAt(right, 0).Content == "-" {
			paint(right, "─")
		}
	}
	return canvas.Render()
}
