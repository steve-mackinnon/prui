package tui

import (
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var canvasForeground = color.RGBA{R: 205, G: 214, B: 244, A: 255}
var canvasBackground = color.RGBA{R: 30, G: 30, B: 46, A: 255}

func canvasCells(text string, width, height int) uv.ScreenBuffer {
	b := uv.NewScreenBuffer(width, height)
	b.Method = ansi.GraphemeWidth
	uv.NewStyledString(text).Draw(b, b.Bounds())
	return b
}

func sameCanvasColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestThemeCanvasRestoresBaseChannelsAfterResets(t *testing.T) {
	content := "A\x1b[31mB\x1b[0mC\x1b[44mD\x1b[49mE\x1b[39mF\x1b[1;7mG\x1b[0mH"
	got := paintThemeCanvas(content, 10, 2, canvasForeground, canvasBackground)
	if plain := ansi.Strip(got); plain != "ABCDEFGH  \n          " {
		t.Fatalf("canvas text = %q", plain)
	}
	cells := canvasCells(got, 10, 2)
	for y := 0; y < 2; y++ {
		for x := 0; x < 10; x++ {
			cell := cells.CellAt(x, y)
			wantFg, wantBg := color.Color(canvasForeground), color.Color(canvasBackground)
			if y == 0 && x == 1 {
				wantFg = ansi.Red
			}
			if y == 0 && x == 3 {
				wantBg = ansi.Blue
			}
			if !sameCanvasColor(cell.Style.Fg, wantFg) || !sameCanvasColor(cell.Style.Bg, wantBg) {
				t.Errorf("cell (%d,%d) channels = %#v", x, y, cell.Style)
			}
		}
	}
	if attrs := cells.CellAt(6, 0).Style.Attrs; attrs&(uv.AttrBold|uv.AttrReverse) != uv.AttrBold|uv.AttrReverse {
		t.Fatalf("focused attributes = %v", attrs)
	}
	if strings.Contains(got, "\x1b]10;") || strings.Contains(got, "\x1b]11;") {
		t.Fatal("canvas changes terminal defaults")
	}
}

func TestThemeCanvasPadsEmptyAndWideContent(t *testing.T) {
	for _, tc := range []struct{ content, want string }{
		{"", "     \n     \n     "},
		{"界e\u0301\n🙂", "界e\u0301  \n🙂   \n     "},
		{"1234界", "1234 \n     \n     "},
		{"123456\nabcdef\nXYZ\nextra", "12345\nabcde\nXYZ  "},
	} {
		got := paintThemeCanvas(tc.content, 5, 3, canvasForeground, canvasBackground)
		if plain := ansi.Strip(got); plain != tc.want {
			t.Errorf("canvas(%q) text = %q, want %q", tc.content, plain, tc.want)
		}
		cells := canvasCells(got, 5, 3)
		for y := 0; y < 3; y++ {
			for x := 0; x < 5; x++ {
				cell := cells.CellAt(x, y)
				if cell.Width == 0 {
					continue
				}
				if !sameCanvasColor(cell.Style.Fg, canvasForeground) || !sameCanvasColor(cell.Style.Bg, canvasBackground) {
					t.Errorf("unpainted cell (%d,%d)", x, y)
				}
			}
		}
	}
}

func TestThemeCanvasInheritedChannelsAndPassthrough(t *testing.T) {
	original := "text\x1b[0m\n"
	for _, tc := range []struct{ fg, bg color.Color }{{nil, nil}, {canvasForeground, nil}, {nil, canvasBackground}} {
		got := paintThemeCanvas(original, 6, 2, tc.fg, tc.bg)
		if tc.fg == nil && tc.bg == nil {
			if got != original {
				t.Fatal("inherited canvas changed bytes")
			}
			continue
		}
		cells := canvasCells(got, 6, 2)
		for y := 0; y < 2; y++ {
			for x := 0; x < 6; x++ {
				s := cells.CellAt(x, y).Style
				if !sameCanvasColor(s.Fg, tc.fg) || !sameCanvasColor(s.Bg, tc.bg) {
					t.Fatalf("inherited channel changed at (%d,%d)", x, y)
				}
			}
		}
	}
	for _, size := range [][2]int{{0, 2}, {2, 0}, {-1, 2}, {2, -1}} {
		if got := paintThemeCanvas(original, size[0], size[1], canvasForeground, canvasBackground); got != original {
			t.Errorf("invalid size %v changed bytes", size)
		}
	}
}

func TestThemeCanvasPreservesHyperlinks(t *testing.T) {
	content := ansi.SetHyperlink("https://example.com") + "link" + ansi.ResetHyperlink() + " plain"
	got := paintThemeCanvas(content, 12, 1, canvasForeground, canvasBackground)
	cells := canvasCells(got, 12, 1)
	if cells.CellAt(0, 0).Link.URL != "https://example.com" || cells.CellAt(4, 0).Link.URL != "" {
		t.Fatal("hyperlink boundary changed")
	}
}

func TestThemeCanvasRepaintsResizeAndThemeSwitch(t *testing.T) {
	old := paintThemeCanvas("long text", 12, 3, canvasForeground, canvasBackground)
	if ansi.StringWidth(strings.Split(old, "\n")[0]) != 12 {
		t.Fatal("initial canvas width")
	}
	nextBg := color.RGBA{R: 40, G: 40, B: 40, A: 255}
	next := paintThemeCanvas("x", 4, 2, canvasForeground, nextBg)
	if plain := ansi.Strip(next); plain != "x   \n    " {
		t.Fatalf("resized canvas = %q", plain)
	}
	cells := canvasCells(next, 4, 2)
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			if !sameCanvasColor(cells.CellAt(x, y).Style.Bg, nextBg) {
				t.Fatal("stale background")
			}
		}
	}
	if got := paintThemeCanvas("x", 4, 2, nil, nil); got != "x" {
		t.Fatal("switch to inherited palette retained padding")
	}
}
