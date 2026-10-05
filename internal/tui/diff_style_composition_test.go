package tui

import (
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"image/color"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
	"prui/internal/theme"
	"reflect"
	"strings"
	"testing"
)

func TestDiffStyleScreenshotRegressions(t *testing.T) {
	palette, _ := theme.Resolve(theme.GitHubDark, nil)
	m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
	f := inventory.FileChange{OldPath: []byte("x.ts"), NewPath: []byte("x.ts")}
	for _, patch := range []string{
		"@@ -0,0 +1,3 @@\n+  .map(toItem),\n+}));\n+}\n",
		"@@ -1 +1 @@\n-    slug: \"swarm\",\n+    id: \"swarm\",\n",
	} {
		rows := textHunkLines(f, inventory.ReviewUnit{}, []byte(patch), source.Identity{}, "")
		for _, row := range rows {
			if row.Class != classAdded && row.Class != classRemoved {
				continue
			}
			rendered := m.styleLine(sourceLineClass(row.Class), m.syntaxText(row, 0, 100, ""))
			if ansi.Strip(rendered) != row.Text {
				t.Fatalf("text changed: %q", rendered)
			}
			cells := canvasCells(rendered, visibleWidth(row.Text), 1)
			fg, _ := palette.Color(theme.Foreground)
			markerRole := theme.Added
			if row.Class == classRemoved {
				markerRole = theme.Removed
			}
			marker, _ := palette.Color(markerRole)
			if !sameCanvasColor(cells.CellAt(0, 0).Style.Fg, marker) {
				t.Errorf("marker color lost: %q", row.Text)
			}
			end := len(row.Text)
			if at := strings.Index(row.Text, ":"); at >= 0 {
				end = at
			}
			for x := 1; x < end; x++ {
				if !sameCanvasColor(cells.CellAt(x, 0).Style.Fg, fg) {
					t.Errorf("%q: source cell %d has wrong foreground", row.Text, x)
				}
			}
			for _, span := range row.wordChanges {
				for x := span.Start; x < span.End; x++ {
					c := cells.CellAt(x, 0)
					if c.Style.Attrs&uv.AttrBold == 0 || c.Style.Underline == 0 {
						t.Errorf("%q: emphasis lost at %d", row.Text, x)
					}
				}
			}
		}
	}
}

func TestDiffStyleThemesProfilesAndFragments(t *testing.T) {
	names := append(theme.BuiltInNames(), "custom")
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			var palette theme.Theme
			if name == "custom" {
				palette, _ = theme.Resolve(theme.GitHubDark, map[theme.Token]string{theme.Foreground: "#d0a0f0"})
			} else {
				palette, _ = theme.Resolve(name, nil)
			}
			for _, profile := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI, colorprofile.Ascii, colorprofile.NoTTY} {
				m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: profile}
				fg, _ := palette.Color(theme.Foreground)
				green, _ := palette.Color(theme.Added)
				plain := diffLine{styledLine: styledLine{classAdded, "+.map(toItem)"}}
				plainRendered := downsample(m.styleLine(classAddedSource, m.syntaxText(plain, 0, 40, "")), profile)
				if profile > colorprofile.Ascii {
					cells := canvasCells(plainRendered, len(plain.Text), 1)
					for x := 1; x < len(plain.Text); x++ {
						if !sameCanvasColor(cells.CellAt(x, 0).Style.Fg, profile.Convert(fg)) {
							t.Fatalf("%s: token-free cell %d inherited a diff color", profile, x)
						}
					}
				}
				for _, class := range []lineClass{classAdded, classRemoved, classContext} {
					marker := "+"
					if class == classRemoved {
						marker = "-"
					}
					if class == classContext {
						marker = " "
					}
					raw := `+plain "green" tail`
					row := diffLine{styledLine: styledLine{class, marker + raw}, syntax: []syntax.Span{{Start: 8, End: 15, Kind: syntax.String}}}
					before := row
					for _, fragmented := range []bool{false, true} {
						row.wordChanges = nil
						if fragmented {
							row.wordChanges = []syntax.Span{{Start: 1, End: len(row.Text), Kind: syntax.Operator}}
						}
						got := downsample(m.styleLine(sourceLineClass(class), m.syntaxText(row, 0, 80, "")), profile)
						if ansi.Strip(got) != row.Text {
							t.Fatalf("%s: source changed: %q", profile, got)
						}
						if profile <= colorprofile.Ascii {
							if len(colorlessSequences(got)) > 0 {
								t.Fatal("colorless output leaked colors")
							}
							continue
						}
						cells := canvasCells(got, visibleWidth(row.Text), 1)
						for x := 1; x < len(row.Text); x++ {
							want := fg
							if x >= 8 && x < 15 {
								want = green
							}
							if !sameCanvasColor(cells.CellAt(x, 0).Style.Fg, profile.Convert(want)) {
								t.Fatalf("%s class %d fragmented %v cell %d wrong foreground", profile, class, fragmented, x)
							}
							if fragmented && (cells.CellAt(x, 0).Style.Attrs&uv.AttrBold == 0 || cells.CellAt(x, 0).Style.Underline == 0) {
								t.Fatalf("emphasis lost at %d", x)
							}
						}
					}
					if row.Text != before.Text || !reflect.DeepEqual(row.syntax, before.syntax) {
						t.Fatal("cached source changed")
					}
				}
			}
		})
	}
}

func TestDiffStyleGeometryAndLinks(t *testing.T) {
	palette, _ := theme.Resolve(theme.GitHubDark, nil)
	m := &Model{theme: palette, styles: stylesFor(palette), colorProfile: colorprofile.TrueColor}
	raw := "+literal\t界e\u0301 alpha beta gamma https://example.com/path\x1b"
	row := diffLine{styledLine: styledLine{classAdded, "+" + Escape(raw)}, rawSource: raw, wordChanges: []syntax.Span{{Start: 1, End: len("+" + Escape(raw)), Kind: syntax.Operator}}}
	before := row
	for _, line := range wrapDiffLines([]diffLine{row}, 24) {
		for _, pan := range []int{0, 1, 8, 1000} {
			for _, width := range []int{0, 1, 12, 70} {
				const prefix = "› "
				got := m.syntaxText(line, pan, width, prefix)
				runes := []rune(line.Text)
				want := clip(prefix, width) + clip(string(runes[min(pan, len(runes)):]), max(0, width-visibleWidth(clip(prefix, width))))
				if ansi.Strip(got) != want {
					t.Fatalf("pan %d width %d: %q != %q", pan, width, ansi.Strip(got), want)
				}
			}
		}
		split := m.renderSideBySideCell(&diffCell{line: &line, number: 42}, 40, 0)
		if visibleWidth(split) != 40 || !strings.HasPrefix(ansi.Strip(split), "   42 + ") {
			t.Fatalf("split geometry changed: %q", split)
		}
		cells := canvasCells(split, 40, 1)
		marker, _ := palette.Color(theme.Added)
		fg, _ := palette.Color(theme.Foreground)
		if !sameCanvasColor(cells.CellAt(6, 0).Style.Fg, marker) || !sameCanvasColor(cells.CellAt(8, 0).Style.Fg, fg) {
			t.Fatal("split marker/source colors mixed")
		}
	}
	if !reflect.DeepEqual(row, before) {
		t.Fatal("wrapping mutated cached row")
	}
	linked := linkSourceURLs(m.syntaxText(row, 0, 100, ""), row.Text, 0, 0)
	cells := themeCanvasBuffer(emphasizeSource(linked), visibleWidth(linked), 1)
	at := strings.Index(ansi.Strip(linked), "https://")
	x := visibleWidth(ansi.Strip(linked)[:at])
	if cells.CellAt(x, 0).Link.URL != "https://example.com/path" {
		t.Fatalf("link lost: %q", cells.CellAt(x, 0).Link.URL)
	}
	if strings.Contains(ansi.Strip(linked), "\x1b") {
		t.Fatal("source control executed")
	}
}

func TestDiffStyleSearchSelectionAndResetBoundaries(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	defer m.Close()
	m.selectReviewView(viewFiles)
	m.theme, _ = theme.Resolve(theme.GitHubDark, nil)
	m.styles = stylesFor(m.theme)
	m.colorProfile = colorprofile.TrueColor
	s := m.searchState()
	s.scope = m.currentSearchScope()
	raw := "hello new tail"
	row := diffLine{styledLine: styledLine{classAdded, "+" + raw}, rawSource: raw, searchID: searchSourceID{m.Session.Inventory.Units[0].ID, 1}, wordChanges: []syntax.Span{{Start: 7, End: 10, Kind: syntax.Operator}}, syntax: []syntax.Span{{Start: 8, End: 10, Kind: syntax.Keyword}}}
	baseline := m.syntaxText(row, 0, 100, "")
	s.query = "absent"
	unmatched := m.syntaxText(row, 0, 100, "")
	if !reflect.DeepEqual(canvasCells(baseline, len(row.Text), 1), canvasCells(unmatched, len(row.Text), 1)) {
		t.Fatal("search with no match recolored source")
	}
	s.query = "new ta"
	for _, active := range []bool{false, true} {
		s.selected = -1
		if active {
			s.documents = []searchDocument{{ID: row.searchID}}
			s.matches = []searchMatch{{Document: 0, Range: searchRange{Start: 6, End: 12}}}
			s.selected = 0
		}
		highlighted := m.syntaxText(row, 0, 100, "")
		for _, class := range []lineClass{classAddedSource, classSelection, classSelectionFocused} {
			got := m.styleLine(class, highlighted)
			cells := canvasCells(got, len(row.Text), 1)
			match := cells.CellAt(7, 0)
			warning, _ := m.theme.Color(theme.Warning)
			contrast, _ := m.theme.Color(theme.Background)
			fg, _ := m.theme.Color(theme.Foreground)
			if !sameCanvasColor(match.Style.Bg, warning) || !sameCanvasColor(match.Style.Fg, contrast) || match.Style.Attrs&uv.AttrBold == 0 || match.Style.Underline == 0 {
				t.Fatalf("search/word composition lost: %#v", match.Style)
			}
			if !sameCanvasColor(cells.CellAt(13, 0).Style.Fg, fg) || cells.CellAt(13, 0).Style.Underline != 0 {
				t.Fatal("match styling leaked")
			}
			keyword, _ := m.theme.Color(theme.FocusedBorder)
			if !sameCanvasColor(cells.CellAt(8, 0).Style.Fg, keyword) {
				t.Fatal("search overwrote explicit syntax foreground")
			}
			if class == classAddedSource {
				ordinary := cells.CellAt(11, 0)
				if (ordinary.Style.Attrs&uv.AttrBold != 0) != active || (ordinary.Style.Underline != 0) != active {
					t.Fatal("active match distinction lost outside changed word")
				}
			}
			if class == classSelectionFocused && match.Style.Attrs&uv.AttrReverse == 0 {
				t.Fatal("focused selection lost reverse")
			}
		}
	}
	s.query = ""
	if !reflect.DeepEqual(canvasCells(baseline, len(row.Text), 1), canvasCells(m.syntaxText(row, 0, 100, ""), len(row.Text), 1)) {
		t.Fatal("closing search did not restore source")
	}
	trusted := "A\x1b[31mB\x1b[39mC\x1b[0mD"
	got := emphasizeSource(paintThemeCanvas(trusted, 4, 1, canvasForeground, nil))
	if ansi.Strip(got) != "ABCD" {
		t.Fatal("nested SGR corrupted text")
	}
	cells := canvasCells(got, 4, 1)
	for x := 0; x < 4; x++ {
		want := color.Color(canvasForeground)
		if x == 1 {
			want = ansi.Red
		}
		c := cells.CellAt(x, 0)
		if !sameCanvasColor(c.Style.Fg, want) || c.Style.Attrs&uv.AttrBold == 0 || c.Style.Underline == 0 {
			t.Fatalf("reset/attribute boundary at %d: %#v", x, c.Style)
		}
	}
}
