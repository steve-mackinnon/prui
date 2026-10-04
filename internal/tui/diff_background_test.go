package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

func TestChangedRowsFillBackgroundWithoutSyntax(t *testing.T) {
	for _, name := range []string{theme.Terminal, theme.Light, theme.CatppuccinMocha, theme.GitHubLight} {
		palette, _ := theme.Resolve(name, nil)
		m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
		for _, class := range []lineClass{classAdded, classRemoved} {
			for _, text := range []string{"+", "+ordinary text"} {
				rendered := m.frameBodyLine(text, class, 30, false)
				cells := canvasCells(rendered, 30, 1)
				first := cells.CellAt(0, 0).Style.Bg
				if first == nil {
					t.Fatalf("%s: changed row has no background", name)
				}
				for x := 0; x < 30; x++ {
					if !sameCanvasColor(first, cells.CellAt(x, 0).Style.Bg) {
						t.Fatalf("%s: background stops at column %d", name, x)
					}
				}
				if ansi.Strip(rendered) != text+strings.Repeat(" ", 30-len(text)) {
					t.Fatal("text or width changed")
				}
				line := diffLine{styledLine: styledLine{class, text}}
				split := m.renderSideBySideCell(&diffCell{line: &line, number: 10}, 30, 50)
				splitCells := canvasCells(split, 30, 1)
				for x := 0; x < 30; x++ {
					if !sameCanvasColor(first, splitCells.CellAt(x, 0).Style.Bg) {
						t.Fatalf("%s: split background missing at %d", name, x)
					}
				}
			}
		}
		add := canvasCells(m.frameBodyLine("+", classAdded, 30, false), 30, 1).CellAt(0, 0).Style.Bg
		remove := canvasCells(m.frameBodyLine("-", classRemoved, 30, false), 30, 1).CellAt(0, 0).Style.Bg
		if sameCanvasColor(add, remove) {
			t.Fatal("addition and deletion backgrounds identical")
		}
		context := canvasCells(m.frameBodyLine(" context", classContext, 30, false), 30, 1)
		if context.CellAt(29, 0).Style.Bg != nil {
			t.Fatal("context received change background")
		}
	}
}
