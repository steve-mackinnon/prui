package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"image"
	"prui/internal/guide"
	"prui/internal/theme"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDiffSearchLiteralRanges(t *testing.T) {
	for _, tt := range []struct {
		text, query string
		want        []searchRange
	}{
		{"test TEST test", "Te", []searchRange{{0, 2}, {5, 7}, {10, 12}}},
		{"日本 Σςσ", "σ", []searchRange{{7, 9}, {9, 11}, {11, 13}}},
		{"aaaa", "aa", []searchRange{{0, 2}, {2, 4}}},
		{"x.*y", ".*", []searchRange{{1, 3}}},
		{"abc", "", nil}, {"a\xffb", "a", nil},
	} {
		if got := literalSearchRanges(tt.text, tt.query, 100); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q / %q: got %v want %v", tt.text, tt.query, got, tt.want)
		}
	}
}

func searchInput(m *Model, text string) {
	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd != nil {
		m.Update(cmd())
	}
	_, cmd = m.Update(tea.PasteMsg{Content: text})
	if cmd != nil {
		m.Update(cmd())
	}
}

func TestDiffSearchInputAndActivation(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 24)
	m.selectReviewView(viewFiles)
	m.fileFilter = "nonexistent"
	searchInput(m, "new 19")
	s := m.searchState()
	if !s.open || len(s.matches) != 2 {
		t.Fatalf("open=%v matches=%d", s.open, len(s.matches))
	}
	if !strings.Contains(m.View().Content, "2 matches in 2 files") {
		t.Fatal("missing result summary")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.open || m.Focus != paneDiff || m.Session.UnitFiles[m.Selected] != 1 || m.Composer != nil || m.fileFilter != "" {
		t.Fatal("activation did not reveal file without opening composer")
	}
	target := m.selectedDiffTarget()
	if target == nil || target.Side != "RIGHT" || target.Line != 40 {
		t.Fatalf("wrong target: %v", target)
	}
}

func TestDiffSearchJumpKeepsVisibleCodeAtLeftMargin(t *testing.T) {
	for _, split := range []bool{false, true} {
		for _, wrapped := range []bool{false, true} {
			t.Run(fmt.Sprintf("split=%v/wrapped=%v", split, wrapped), func(t *testing.T) {
				s := largeTextSession(1, 1)
				text := "CREATE TABLE hero_theme text DEFAULT light;"
				if wrapped {
					text = strings.Repeat("column text, ", 100) + text
				}
				s.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+" + text + "\n")
				m := largeModel(s, 180, 24)
				m.selectReviewView(viewFiles)
				if split {
					m.layout = diffLayoutSideBySide
				}
				m.Horizontal = 200
				searchInput(m, "hero")
				m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if m.Horizontal != 0 {
					t.Fatalf("visible wrapped match panned all code by %d", m.Horizontal)
				}
				if !strings.Contains(m.View().Content, "hero") {
					t.Fatal("jump hid search result")
				}
			})
		}
	}
}

func TestDiffSearchJumpPansOverflowAndAllowsRecovery(t *testing.T) {
	for _, split := range []bool{false, true} {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			s := largeTextSession(1, 1)
			s.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+" + strings.Repeat("界", 200) + "NEEDLE\n")
			m := largeModel(s, 180, 24)
			m.selectReviewView(viewFiles)
			if split {
				m.layout = diffLayoutSideBySide
			}
			searchInput(m, "needle")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if m.Horizontal == 0 || !strings.Contains(m.View().Content, "NEEDLE") {
				t.Fatal("overflow match not revealed")
			}
			before := m.Horizontal
			m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
			if m.Horizontal >= before {
				t.Fatal("left arrow did not recover horizontal position")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
			if m.Horizontal != 0 || !strings.Contains(m.View().Content, "界") {
				t.Fatal("home did not restore code")
			}
		})
	}
}

func TestDiffSearchOnlyCurrentGuideSection(t *testing.T) {
	session := largeTextSession(2, 2)
	session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Guide", Sections: []guide.Section{
		{Title: "First", UnitIDs: []string{session.Inventory.Units[0].ID}},
		{Title: "Second", UnitIDs: []string{session.Inventory.Units[1].ID}},
	}}}}
	m := largeModel(session, 120, 24)
	m.selectReviewView(viewGuide)
	m.Row = 1
	searchInput(m, "new 19")
	s := m.searchState()
	if len(s.matches) != 1 || s.documents[s.matches[0].Document].File != 0 {
		t.Fatalf("scope leaked: %v", s.matches)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for i, r := range m.rows() {
		if r.kind == sectionRow && r.section == 1 {
			m.Row = i
			break
		}
	}
	cmd := m.startSearch()
	m.Update(cmd())
	if len(s.matches) != 1 || s.documents[s.matches[0].Document].File != 1 {
		t.Fatal("section change retained old results")
	}
}

func TestDiffSearchDocumentBoundaries(t *testing.T) {
	s := largeTextSession(1, 1)
	s.Inventory.Patches["p"] = []byte("--- a/new\n+++ b/new\n@@ -1 +1 @@\n context\n-old\n+new\n\\ No newline at end of file\n")
	docs, skipped := searchDocuments(context.Background(), s, searchScope{Guide: -1, Section: -1})
	if len(docs) != 3 || skipped != 0 {
		t.Fatalf("documents=%v skipped=%d", docs, skipped)
	}
}

func TestDiffSearchStaleResultsAndModalInput(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	m.selectReviewView(viewFiles)
	searchInput(m, "new")
	old := m.startSearch()
	latest := m.insertSearchText(" 19")
	m.Update(latest())
	m.Update(old())
	if len(m.searchState().matches) != 1 {
		t.Fatal("stale query replaced new results")
	}
	m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.searchState().query != "new 19q" || !m.searchOpen() {
		t.Fatal("shortcut leaked out of search")
	}
	m.Update(tea.PasteMsg{Content: "\nm"})
	if m.searchState().query != "new 19q" {
		t.Fatal("multiline paste accepted")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	before := m.searchState().query
	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd != nil {
		m.Update(cmd())
	}
	if m.searchState().query != before {
		t.Fatal("reopen lost query")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	for range []rune(before) {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	}
	if m.searchState().query != "" {
		t.Fatal("query could not be cleared by editing")
	}

}

func TestDiffSearchWrappedSplitNavigationAndHighlights(t *testing.T) {
	for _, width := range []int{60, 120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s := largeTextSession(1, 1)
			s.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-" + strings.Repeat("before words ", 180) + "NEEDLE\n+" + strings.Repeat("after words ", 180) + "NEEDLE\n")
			m := largeModel(s, width, 24)
			m.selectReviewView(viewFiles)
			m.layout = diffLayoutSideBySide
			searchInput(m, "needle")
			state := m.searchState()
			if len(state.matches) != 2 {
				t.Fatal("match count", len(state.matches))
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if target := m.selectedDiffTarget(); target == nil || target.Side != "LEFT" {
				t.Fatal("lost deletion side", target)
			}
			if !strings.Contains(m.View().Content, "NEEDLE") {
				t.Fatal("wrapped match not revealed")
			}
			cmd := m.openSearch()
			if cmd != nil {
				m.Update(cmd())
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if target := m.selectedDiffTarget(); target == nil || target.Side != "RIGHT" {
				t.Fatal("lost addition side", target)
			}
			m.Update(tea.WindowSizeMsg{Width: 120, Height: 20})
			if !strings.Contains(m.View().Content, "NEEDLE") {
				t.Fatal("resize hid matched column")
			}
			if target := m.selectedDiffTarget(); target == nil || target.Side != "RIGHT" {
				t.Fatal("resize lost target", target)
			}
		})
	}
}

func TestDiffSearchHighlightEscapingAndColorless(t *testing.T) {
	for _, name := range []string{theme.CatppuccinMocha, theme.GitHubLight} {
		m := largeModel(largeTextSession(1, 1), 120, 24)
		m.selectReviewView(viewFiles)
		m.theme, _ = theme.Resolve(name, nil)
		m.styles = stylesFor(m.theme)
		m.colorProfile = colorprofile.TrueColor
		s := m.searchState()
		s.query = "日\t"
		s.scope = m.currentSearchScope()
		raw := "before 日\t after\x1b[31m"
		line := diffLine{styledLine: styledLine{classAdded, "+" + Escape(raw)}, searchID: searchSourceID{m.Session.Inventory.Units[0].ID, 1}, rawSource: raw}
		rendered := m.styleLine(classAdded, m.syntaxText(line, 0, 100, ""))
		if ansi.Strip(rendered) != line.Text {
			t.Fatal("highlight changed escaped source")
		}
		cells := canvasCells(rendered, 100, 1)
		want, _ := m.theme.Color(theme.Warning)
		if !sameCanvasColor(cells.CellAt(8, 0).Style.Bg, want) {
			t.Fatal("match highlight lost to diff style")
		}
		m.colorProfile = colorprofile.NoTTY
		if got := m.syntaxText(line, 0, 100, ""); got != line.Text {
			t.Fatalf("colorless output changed: %q", got)
		}
	}
}

func TestDiffSearchMouseOwnershipAndTinyLayout(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 24)
	m.selectReviewView(viewFiles)
	searchInput(m, "new 19")
	selected := m.Selected
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: 4})
	if m.Selected != selected || !m.searchOpen() {
		t.Fatal("background click leaked")
	}
	for _, size := range [][2]int{{60, 12}, {20, 5}, {1, 1}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View().Content
		if len(strings.Split(view, "\n")) > size[1] {
			t.Fatal("height overflow")
		}
		for _, line := range strings.Split(view, "\n") {
			if visibleWidth(line) > size[0] {
				t.Fatal("width overflow")
			}
		}
	}
}

func BenchmarkDiffSearchQuery(b *testing.B) {
	s := largeTextSession(100, 1667) // 100,020 source lines
	m := largeModel(s, 120, 30)
	m.selectReviewView(viewFiles)
	state := m.searchState()
	state.documents, state.skipped = searchDocuments(context.Background(), s, m.currentSearchScope())
	b.ReportAllocs()
	samples := make([]time.Duration, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		state.query = "absent needle"
		m.Update(m.startSearch()())
		samples[i] = time.Since(start)
	}
	b.StopTimer()
	slices.Sort(samples)
	b.ReportMetric(float64(samples[min(len(samples)-1, len(samples)*95/100)].Nanoseconds()), "p95-ns")
}
func BenchmarkDiffSearchPopover(b *testing.B) {
	m := largeModel(largeTextSession(100, 1667), 120, 30)
	m.selectReviewView(viewFiles)
	searchInput(m, "e")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.searchState().selected = (i * 7) % len(m.searchState().matches)
		_ = m.searchPopover("")
	}
}

func TestDiffSearchLimitsCancellationAndMissingGuide(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	m.selectReviewView(viewFiles)
	m.Session.Inventory.Patches["p"] = []byte("@@ -0,0 +1 @@\n+" + strings.Repeat("a", searchResultLimit+1) + "\n")
	searchInput(m, "a")
	s := m.searchState()
	if len(s.matches) != searchResultLimit || !s.capped || !strings.Contains(m.View().Content, "First 10,000 matches") {
		t.Fatal("result cap not labeled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := newSearchPattern("a").ranges(ctx, strings.Repeat("a", 100000), searchResultLimit); len(got) != 0 {
		t.Fatal("canceled scan continued")
	}
	m.closeSearch()
	m.selectReviewView(viewGuide)
	searchInput(m, "a")
	if len(m.searchState().matches) != 0 || !strings.Contains(m.View().Content, "Select a guide section") {
		t.Fatal("missing guide searched all files")
	}
}

func TestDiffSearchIgnoresReplacedSession(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	m.selectReviewView(viewFiles)
	s := m.searchState()
	s.query = "old"
	old := m.startSearch()
	m.Session = largeTextSession(2, 2)
	searchInput(m, "new 19")
	m.Update(old())
	if len(m.searchState().matches) != 2 {
		t.Fatal("old session replaced current results")
	}
}

func TestDiffSearchContextCountAndCacheImmutability(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 180, 24)
	m.selectReviewView(viewFiles)
	m.layout = diffLayoutSideBySide
	original := append([]diffLine(nil), m.cachedFileDetail(false)...)
	searchInput(m, "context 19")
	if len(m.searchState().matches) != 1 {
		t.Fatal("context was counted on both split sides")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if target := m.selectedDiffTarget(); target == nil || target.Side != "RIGHT" {
		t.Fatal("context selected invalid target")
	}
	_ = m.View()
	if !reflect.DeepEqual(original, m.cachedFileDetail(false)) {
		t.Fatal("search mutated source cache")
	}
}

func TestDiffSearchHeaderMouseAndQueryFocus(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	m.selectReviewView(viewFiles)
	header := strings.Split(ansi.Strip(m.View().Content), "\n")[2]
	start := strings.Index(header, "Find (/)")
	x := visibleWidth(header[:start])
	_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: 2})
	if cmd != nil {
		m.Update(cmd())
	}
	if !m.searchOpen() {
		t.Fatal("visible Find control was not clickable")
	}
	cmd = m.insertSearchText("old")
	m.Update(cmd())
	bounds := m.searchBounds()
	_, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: bounds.Min.X + 3, Y: bounds.Min.Y + 1})
	if cmd != nil {
		m.Update(cmd())
	}
	if !m.searchState().editing || m.searchState().query != "old" {
		t.Fatal("query click failed to focus without clearing")
	}
}

func TestSearchAndCommitHeaderHitboxes(t *testing.T) {
	for _, width := range []int{90, 120, 160} {
		m := largeModel(largeTextSession(1, 1), width, 24)
		m.selectReviewView(viewFiles)
		m.Focus = paneDiff
		header := strings.Split(ansi.Strip(m.View().Content), "\n")[2]
		for _, label := range []string{"Commits [C]", "Find (/)"} {
			start := strings.Index(header, label)
			if start < 0 {
				t.Fatalf("width %d missing %s in %q", width, label, header)
			}
			x := visibleWidth(header[:start])
			if label == "Find (/)" {
				if !image.Pt(x, 2).In(m.searchHeaderBounds()) || m.commitFilterControlContains(x) {
					t.Fatalf("width %d search hitbox overlaps commit control", width)
				}
			} else if !m.commitFilterControlContains(x) || image.Pt(x, 2).In(m.searchHeaderBounds()) {
				t.Fatalf("width %d commit hitbox overlaps search control", width)
			}
		}
	}
}

func TestDiffSearchVimFocusModes(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 24)
	m.selectReviewView(viewFiles)
	_, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd != nil {
		m.Update(cmd())
	}
	if !m.searchOpen() || m.fileFilterEditing {
		t.Fatal("slash must open code search")
	}
	_, cmd = m.Update(tea.PasteMsg{Content: "new 19"})
	if cmd != nil {
		m.Update(cmd())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.searchOpen() {
		t.Fatal("first Escape should select results, not close")
	}
	key(m, 'j')
	if m.searchState().selected != 1 || m.searchState().query != "new 19" {
		t.Fatal("j did not navigate results")
	}
	key(m, 'k')
	if m.searchState().selected != 0 {
		t.Fatal("k did not navigate results")
	}
	key(m, '/')
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if cmd != nil {
		m.Update(cmd())
	}
	if m.searchState().query != "new 19j" {
		t.Fatal("slash did not return to editing")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.searchOpen() {
		t.Fatal("second Escape should dismiss search")
	}
	key(m, 'F')
	if !m.fileFilterEditing {
		t.Fatal("F did not open filename filter")
	}
}

func TestDiffSearchVimNextPreviousMatches(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 24)
	m.selectReviewView(viewFiles)
	searchInput(m, "new 19")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	for _, step := range []struct {
		key      rune
		selected int
	}{{'n', 1}, {'n', 1}, {'N', 0}, {'N', 0}} {
		key(m, step.key)
		if s := m.searchState(); s.selected != step.selected || s.query != "new 19" || !m.searchOpen() {
			t.Fatalf("%c: selected=%d query=%q open=%v", step.key, s.selected, s.query, m.searchOpen())
		}
	}
	key(m, '/')
	for _, r := range "nN" {
		_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		if cmd != nil {
			m.Update(cmd())
		}
	}
	if m.searchState().query != "new 19nN" {
		t.Fatal("n/N must remain literal text while editing")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	key(m, 'n')
	key(m, 'N')
	if m.searchState().selected != 0 {
		t.Fatal("navigation with no matches must keep selection at zero")
	}
}

func TestDiffSearchRepeatFromDiff(t *testing.T) {
	for _, view := range []reviewView{viewFiles, viewGuide} {
		session := largeTextSession(2, 2)
		session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Guide", Sections: []guide.Section{
			{Title: "Matches", UnitIDs: []string{session.Inventory.Units[0].ID, session.Inventory.Units[1].ID}},
		}}}}
		m := largeModel(session, 120, 24)
		m.selectReviewView(view)
		if view == viewGuide {
			m.Row = 1
		}
		searchInput(m, "new 19")
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.searchOpen() || m.Focus != paneDiff {
			t.Fatal("search activation must focus diff")
		}
		key(m, 'n')
		if m.searchState().selected != 1 || m.searchOpen() || !m.searchMatchAtCursor() {
			t.Fatalf("view %v: n did not reveal next match in diff", view)
		}
		key(m, 'N')
		if m.searchState().selected != 0 || m.searchOpen() || !m.searchMatchAtCursor() {
			t.Fatalf("view %v: N did not reveal previous match in diff", view)
		}
	}
}

func TestDiffSearchInsetResultClickAndQuerySlash(t *testing.T) {
	m := largeModel(largeTextSession(2, 2), 120, 24)
	m.selectReviewView(viewFiles)
	searchInput(m, "new 19")
	view := ansi.Strip(m.View().Content)
	if strings.Contains(view, "Clear") || strings.Contains(view, "Close") {
		t.Fatal("removed controls still visible")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.Update(tea.PasteMsg{Content: "ignored"})
	if m.searchState().query != "new 19" {
		t.Fatal("paste changed query while selecting results")
	}
	bounds := m.searchBounds()
	// Heading + first result + second heading + second result.
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: bounds.Min.X + 4, Y: bounds.Min.Y + 7})
	if m.searchOpen() || m.Session.UnitFiles[m.Selected] != 1 {
		t.Fatal("inset result click did not reveal second file")
	}
	cmd := m.openSearch()
	if cmd != nil {
		m.Update(cmd())
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd != nil {
		m.Update(cmd())
	}
	if m.searchState().query != "new 19/" {
		t.Fatal("literal slash not accepted while editing")
	}
}
