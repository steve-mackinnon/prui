package tui

import (
	"fmt"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/theme"
	"strings"
	"testing"
)

func TestWordDiffChangedTokens(t *testing.T) {
	rows := textHunkLines(inventory.FileChange{}, inventory.ReviewUnit{}, []byte("@@ -1 +1 @@\n-var café = 12\n+var café = 42\n"), source.Identity{}, "")
	for _, row := range rows[1:3] {
		if len(row.wordChanges) != 1 || row.Text[row.wordChanges[0].Start:row.wordChanges[0].End] != map[lineClass]string{classRemoved: "12", classAdded: "42"}[row.Class] {
			t.Fatalf("changed tokens: %#v", row)
		}
	}
}

func TestWordDiffSyntaxRenderingPreservesText(t *testing.T) {
	f := inventory.FileChange{OldPath: []byte("card.tsx"), NewPath: []byte("card.tsx")}
	rows := textHunkLines(f, inventory.ReviewUnit{}, []byte("@@ -98 +98 @@\n-  section: CatalogSection;\n+  section: string;\n"), source.Identity{}, "")
	palette, _ := theme.Resolve(theme.Terminal, nil)
	m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
	for _, row := range rows[1:3] {
		if len(row.syntax) == 0 || len(row.wordChanges) == 0 {
			t.Fatal("fixture must combine syntax colors and word emphasis")
		}
		rendered := m.syntaxText(row, 0, 100, "")
		if got := ansi.Strip(rendered); got != row.Text {
			t.Fatalf("color codes leaked into source: %q, want %q", got, row.Text)
		}
		cells := canvasCells(rendered, 100, 1)
		at := visibleWidth(row.Text[:row.wordChanges[0].Start])
		if cells.CellAt(at, 0).Style.Underline == 0 || cells.CellAt(at, 0).Style.Attrs&uv.AttrBold == 0 {
			t.Fatal("changed token lost emphasis")
		}
	}
}

func TestWordDiffFixturesAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, old, new, changedOld, changedNew string
		fallback                               bool
	}{
		{"interior", "left1 MID right1", "left2 MID right2", "left1right1", "left2right2", false},
		{"go", "return foo(12)", "return foo(42)", "12", "42", false},
		{"python", "value = 'old'", "value = 'new'", "old", "new", false},
		{"json", `{"value": true}`, `{"value": false}`, "true", "false", false},
		{"text", "hello café 世界", "hello naïve 世界", "café", "naïve", false},
		{"tabs", "\tvalue = old", "\tvalue = new", "old", "new", false},
		{"combining", "hello e\u0301", "hello a\u0301", "e\u0301", "a\u0301", false},
		{"ambiguous", "a b a z", "a a b z", "", "", true},
		{"unrelated", "abc", "xyz", "", "", true},
		{"invalid", "same \xff", "same \xfe", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := wordDiffMaxWork
			a, b := changedWords(tc.old, tc.new, &budget)
			if tc.fallback {
				if len(a)+len(b) != 0 {
					t.Fatal("expected fallback", a, b)
				}
				return
			}
			collect := func(text string, spans []syntax.Span) string {
				var out string
				for _, s := range spans {
					out += text[s.Start:s.End]
				}
				return out
			}
			if collect(tc.old, a) != tc.changedOld || collect(tc.new, b) != tc.changedNew {
				t.Fatal("wrong changes", a, b)
			}
		})
	}
	for _, patch := range []string{
		"@@ -1,2 +1 @@\n-same old\n-other old\n+same new\n",
		"@@ -1 +1,2 @@\n-same old\n+same new\n+other new\n",
		"@@ -1 +1 @@\n-same old\n+same new\n" + strings.Repeat(" context\n", wordDiffMaxBytes/8),
	} {
		rows := textHunkLines(inventory.FileChange{}, inventory.ReviewUnit{}, []byte(patch), source.Identity{}, "")
		for _, r := range rows {
			if len(r.wordChanges) > 0 {
				t.Fatal("unsafe block highlighted")
			}
		}
	}
	budget := 0
	if a, b := changedWords("same old", "same new", &budget); len(a)+len(b) > 0 {
		t.Fatal("budget ignored")
	}
	budget = wordDiffMaxWork
	if a, b := changedWords("same "+strings.Repeat("x", wordDiffMaxLine), "same new", &budget); len(a)+len(b) > 0 {
		t.Fatal("line cap ignored")
	}
	if a, b := changedWords(strings.Repeat("a ", wordDiffMaxTokens), "same new", &budget); len(a)+len(b) > 0 {
		t.Fatal("token cap ignored")
	}
}

func TestWordDiffRenderingPreservesSourceAndAnchors(t *testing.T) {
	f := inventory.FileChange{OldPath: []byte("a.txt"), NewPath: []byte("a.txt")}
	patch := []byte("@@ -10 +20 @@\n-\thello café old 世界 end\n+\thello café new 世界 end\n")
	rows := textHunkLines(f, inventory.ReviewUnit{}, patch, source.Identity{}, "pinned")
	palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
	m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
	for _, row := range rows[1:3] {
		target := *row.target
		for _, wrapped := range wrapDiffLines([]diffLine{row}, 18) {
			if *wrapped.target != target {
				t.Fatal("anchor changed")
			}
			for _, pan := range []int{0, 2, 6} {
				got := m.syntaxText(wrapped, pan, 18, "")
				want := clip(string([]rune(wrapped.Text)[min(pan, len([]rune(wrapped.Text))):]), 18)
				if ansi.Strip(got) != want {
					t.Fatalf("source changed: %q != %q", ansi.Strip(got), want)
				}
			}
		}
		styled := m.syntaxText(row, 0, 100, "")
		if !strings.Contains(styled, "\x1b[") {
			t.Fatal("no changed-word styling")
		}
		cells := canvasCells(styled, 100, 1)
		at := visibleWidth(row.Text[:row.wordChanges[0].Start])
		if cells.CellAt(at, 0).Style.Underline == 0 || cells.CellAt(at, 0).Style.Attrs&uv.AttrBold == 0 {
			t.Fatal("non-color emphasis absent")
		}
		m.colorProfile = colorprofile.NoTTY
		if m.syntaxText(row, 0, 100, "") != row.Text {
			t.Fatal("plain text changed")
		}
		m.colorProfile = colorprofile.TrueColor
	}
	split := projectSideBySideRows(rows)
	for _, r := range split {
		for _, cell := range []*diffCell{r.old, r.new} {
			if cell == nil {
				continue
			}
			got := m.renderSideBySideCell(cell, 60, 0)
			if len(cell.line.wordChanges) == 0 || !strings.Contains(got, "\x1b[") {
				t.Fatal("split highlighting absent")
			}
		}
	}
	if rows[1].target.Side != "LEFT" || rows[1].target.Line != 10 || rows[2].target.Side != "RIGHT" || rows[2].target.Line != 20 {
		t.Fatal("canonical coordinates changed")
	}
	if string(patch) != "@@ -10 +20 @@\n-\thello café old 世界 end\n+\thello café new 世界 end\n" {
		t.Fatal("raw patch mutated")
	}
}

func TestWordDiffMultiLineAlignment(t *testing.T) {
	for _, tc := range []struct {
		patch       string
		highlighted bool
	}{
		{"@@ -1,2 +1,2 @@\n-alphaCount = 12\n-betaValue = 34\n+alphaCount = 56\n+betaValue = 78\n", true},
		{"@@ -1,2 +1,2 @@\n-value = 12\n-value = 34\n+value = 56\n+value = 78\n", false},
		{"@@ -1 +1 @@\n-x x\n+x\n", false},
	} {
		rows := textHunkLines(inventory.FileChange{}, inventory.ReviewUnit{}, []byte(tc.patch), source.Identity{}, "")
		for _, row := range rows {
			if row.target != nil && (len(row.wordChanges) > 0) != tc.highlighted {
				t.Fatal("alignment", tc.patch, row.wordChanges)
			}
		}
	}
}

func TestWordDiffSearchComposition(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	defer m.Close()
	m.selectReviewView(viewFiles)
	m.theme, _ = theme.Resolve(theme.CatppuccinMocha, nil)
	m.styles = stylesFor(m.theme)
	m.colorProfile = colorprofile.TrueColor
	s := m.searchState()
	s.query = "new"
	s.scope = m.currentSearchScope()
	raw := "hello new 世界"
	row := diffLine{styledLine: styledLine{classAdded, "+" + Escape(raw)}, rawSource: raw, searchID: searchSourceID{m.Session.Inventory.Units[0].ID, 1}, wordChanges: []syntax.Span{{Start: 7, End: 10, Kind: syntax.Operator}}}
	styled := m.styleLine(classAdded, m.syntaxText(row, 0, 100, ""))
	if ansi.Strip(styled) != row.Text {
		t.Fatal("search changes source")
	}
	cells := canvasCells(styled, 100, 1)
	c := cells.CellAt(7, 0)
	warning, _ := m.theme.Color(theme.Warning)
	if c.Style.Underline == 0 || c.Style.Attrs&uv.AttrBold == 0 || !sameCanvasColor(c.Style.Bg, warning) {
		t.Fatal("search and word emphasis did not compose")
	}
}

func TestWordDiffExactWorkBoundaryAndBlockLimit(t *testing.T) {
	old, newText := "left1 MID right1", "left2 MID right2"
	required := 2 * (len(wordTokens(old)) + 1) * (len(wordTokens(newText)) + 1)
	budget := required - 1
	if a, b := changedWords(old, newText, &budget); len(a)+len(b) > 0 || budget != required-1 {
		t.Fatal("insufficient work must leave pair uncomputed")
	}
	budget = required
	if a, b := changedWords(old, newText, &budget); len(a) != 2 || len(b) != 2 || budget != 0 {
		t.Fatal("exact work boundary failed", a, b, budget)
	}
	var patch strings.Builder
	patch.WriteString("@@ -1,17 +1,17 @@\n")
	for i := 0; i < 17; i++ {
		fmt.Fprintf(&patch, "-value%d = old\n", i)
	}
	for i := 0; i < 17; i++ {
		fmt.Fprintf(&patch, "+value%d = new\n", i)
	}
	rows := textHunkLines(inventory.FileChange{}, inventory.ReviewUnit{}, []byte(patch.String()), source.Identity{}, "")
	for _, row := range rows {
		if len(row.wordChanges) > 0 {
			t.Fatal("block line cap ignored")
		}
	}
}
