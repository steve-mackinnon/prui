package tui

import (
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/theme"
	"reflect"
	"strings"
	"testing"
)

func TestSyntaxEscapingWrappingAndPanning(t *testing.T) {
	raw := []byte("\tα \"hello world\"\x1b")
	spans := []syntax.Span{{Start: 0, End: len(raw), Kind: syntax.String}}
	line := diffLine{styledLine: styledLine{classAdded, "+" + Escape(string(raw))}, syntax: escapedSpans(raw, spans)}
	palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
	m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
	before := append([]syntax.Span(nil), line.syntax...)
	rows := wrapDiffLines([]diffLine{line}, 14)
	if len(rows) < 2 {
		t.Fatal("fixture did not wrap")
	}
	for _, row := range rows {
		got := m.syntaxText(row, 2, 12, "› ")
		want := "› " + clip(string([]rune(row.Text)[2:]), 10)
		if ansi.Strip(got) != want {
			t.Fatalf("text changed: %q != %q", ansi.Strip(got), want)
		}
		if !strings.Contains(got, "\x1b[") {
			t.Fatal("syntax styling absent")
		}
	}
	if !reflect.DeepEqual(before, line.syntax) {
		t.Fatal("cached spans mutated")
	}
	m.colorProfile = colorprofile.NoTTY
	if got := m.syntaxText(line, 0, 100, ""); got != line.Text {
		t.Fatalf("plain output changed %q", got)
	}
	if got := escapedSpans(raw, []syntax.Span{{Start: -1, End: 4, Kind: syntax.String}, {Start: 2, End: 3, Kind: syntax.String}}); len(got) != 0 {
		t.Fatal("invalid offsets accepted", got)
	}
}

func TestSyntaxSidesAndLegacyFragments(t *testing.T) {
	f := inventory.FileChange{OldPath: []byte("x.go"), NewPath: []byte("x.go")}
	u := inventory.ReviewUnit{OldRange: inventory.Range{Start: 10}, NewRange: inventory.Range{Start: 10}}
	patch := []byte("@@ -10 +10 @@\n same\n")
	tokens := syntax.Patch{1: {Old: []syntax.Span{{Start: 0, End: 4, Kind: syntax.Comment}}, New: []syntax.Span{{Start: 0, End: 4, Kind: syntax.String}}}}
	rows := textHunkLines(f, u, patch, source.Identity{}, "", tokens)
	split := projectSideBySideRows(rows)
	if split[1].old.line.syntax[0].Kind != syntax.Comment || split[1].new.line.syntax[0].Kind != syntax.String {
		t.Fatal("split context uses wrong side")
	}
	if rows[1].syntax[0].Kind != syntax.String {
		t.Fatal("unified context must use new side")
	}
	legacy := textHunkLines(f, u, []byte("@@ -1 +1 @@\n-var a = 1\n+var a = 2\n"), source.Identity{}, "")
	if len(legacy[1].syntax) == 0 || len(legacy[2].syntax) == 0 {
		t.Fatal("legacy fragment fallback missing")
	}
	plain := textHunkLines(f, u, patch, source.Identity{}, "", syntax.Patch{})
	if len(plain[1].syntax) != 0 {
		t.Fatal("explicit empty capture should not fall back")
	}
}

func TestSyntaxThemeColorsSurviveDiffAndSelectionStyles(t *testing.T) {
	for _, name := range []string{theme.CatppuccinMocha, theme.GitHubLight} {
		palette, _ := theme.Resolve(name, nil)
		m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
		line := diffLine{styledLine: styledLine{classAdded, "+var answer = 42"}, syntax: []syntax.Span{{Start: 1, End: 4, Kind: syntax.Keyword}, {Start: 14, End: 16, Kind: syntax.Number}}}
		rendered := m.styleLine(line.Class, m.syntaxText(line, 0, 30, ""))
		cells := canvasCells(rendered, 30, 1)
		expected, _ := palette.Color(theme.FocusedBorder)
		if !sameCanvasColor(cells.CellAt(1, 0).Style.Fg, expected) {
			t.Fatalf("%s: keyword color overridden", name)
		}
		if cells.CellAt(1, 0).Style.Bg == nil {
			t.Fatalf("%s: diff background missing", name)
		}
		selected := m.styleLine(classSelection, m.syntaxText(line, 0, 30, ""))
		if !sameCanvasColor(canvasCells(selected, 30, 1).CellAt(1, 0).Style.Fg, expected) {
			t.Fatalf("%s: selection erased syntax", name)
		}
		cell := &diffCell{line: &line, number: 42}
		if ansi.Strip(m.renderSideBySideCell(cell, 30, 0)) != "   42 + var answer = 42       " {
			t.Fatalf("split text changed: %q", ansi.Strip(m.renderSideBySideCell(cell, 30, 0)))
		}
	}
}
