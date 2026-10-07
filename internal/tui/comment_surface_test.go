package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
	"prui/internal/theme"
)

func TestSelectedCommentUsesBorderInsteadOfRowHighlight(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		m := largeModel(screenSession(), 160, 30)
		m.layout = layout
		palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
		m.SetTheme(palette)
		m.colorProfile = colorprofile.TrueColor
		var target *source.ReviewCommentTarget
		for _, line := range m.displayDetail() {
			if line.target != nil {
				target = line.target
				break
			}
		}
		if target == nil {
			t.Fatal("missing source target")
		}
		m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "selected body", Author: "reviewer"}}
		for i, line := range m.displayDetail() {
			if strings.Contains(line.Text, "selected body") {
				m.setCursor(i)
				m.cursorActive = true
				break
			}
		}
		view := m.View().Content
		if !strings.Contains(view, "┌") || !strings.Contains(view, "└") {
			t.Fatal("selected comment lacks solid corners")
		}
		cells := themeCanvasBuffer(view, m.Width, m.Height)
		found := false
		for y, row := range strings.Split(ansi.Strip(view), "\n") {
			body := strings.Index(row, "selected body")
			if body < 0 {
				continue
			}
			found = true
			x := visibleWidth(row[:body])
			if cells.CellAt(x, y).Style.Attrs&uv.AttrReverse != 0 {
				t.Fatal("selected comment still uses reverse row highlight")
			}
			left := x - 2
			if cells.CellAt(left, y).Content != "│" {
				t.Fatal("selected comment lacks solid side border")
			}
			if cells.CellAt(left, y-2).Content != "┌" || cells.CellAt(left, y+1).Content != "└" {
				t.Fatal("selected comment lacks solid corners")
			}
			if !sameCanvasColor(cells.CellAt(left, y).Style.Fg, themeBaseColor(palette, theme.FocusedBorder)) {
				t.Fatal("selected border lacks focus color")
			}
			if !sameCanvasColor(cells.CellAt(x, y).Style.Bg, modalBackground(palette)) {
				t.Fatal("comment background changed on selection")
			}
		}
		if !found {
			t.Fatal("comment body not rendered")
		}
		m.Close()
	}
}
