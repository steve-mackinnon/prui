package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestCommentComposerViewRendersFrozenTargetEscapedMultilineDraftAndControls(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = kindsSession()
	m.Composer = &commentComposer{
		Target: source.ReviewCommentTarget{Path: "dir/unsafe\x1b[file.go", Side: "RIGHT", Line: 42},
		Draft:  "first line\nsecond\x1b[31m line",
		Cursor: len([]rune("first ")),
	}
	m.editorCursorVisible = true
	view := ansi.Strip(strings.Join(func() []string {
		lines := m.inlineEditorLines()
		out := make([]string, len(lines))
		for i := range lines {
			out[i] = lines[i].Text
		}
		return out
	}(), "\n"))
	for _, want := range []string{
		"+",
		"first ▏line",
		"second\\x1b[31m line",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer view missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"[comment", "ctrl+enter", "esc: discard"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("editor chrome unexpectedly contains %q:\n%s", unwanted, view)
		}
	}
}

func TestCommentBindingsAdvertiseComposerEntryAndControls(t *testing.T) {
	help := renderHealth()
	for _, want := range []string{
		"enter: focus the diff; open a line editor or selected-comment action menu",
		"enter: submit inline comment editor",
		"shift+enter: newline in inline comment editor",
		"backspace/delete: delete previous/following rune in comment editor",
		"c: refresh ephemeral inline review comments",
		"r/a/d: reply, react, or delete a selected comment (delete requires ownership and confirmation)",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestLayoutBindingDocumentsDefaultAndNarrowFallback(t *testing.T) {
	help := renderHealth()
	want := "S: toggle side-by-side detail (unified default; falls back below 160 columns)"
	if !strings.Contains(help, want) {
		t.Fatalf("help missing %q:\n%s", want, help)
	}
}

func TestSideBySideProjection(t *testing.T) {
	target := func(side string, line int) *source.ReviewCommentTarget {
		return &source.ReviewCommentTarget{Side: side, Line: line}
	}
	line := func(class lineClass, text string, target *source.ReviewCommentTarget) diffLine {
		return diffLine{styledLine: styledLine{Class: class, Text: text}, target: target}
	}

	rows := projectSideBySideRows([]diffLine{
		line(classHunk, "@@ -10,5 +20,5 @@", nil),
		line(classContext, " shared", target("RIGHT", 20)),
		line(classRemoved, "-old one", target("LEFT", 11)),
		line(classRemoved, "-old two", target("LEFT", 12)),
		line(classAdded, "+new one", target("RIGHT", 21)),
		line(classAdded, "+new two", target("RIGHT", 22)),
		line(classAdded, "+new three", target("RIGHT", 23)),
		line(classContext, " tail", target("RIGHT", 24)),
		line(classRemoved, "-gone", target("LEFT", 14)),
		line(classContext, `\ No newline at end of file`, nil),
		line(classHunk, "@@ -30 +40,3 @@", nil),
		line(classContext, " second hunk", target("RIGHT", 40)),
		line(classAdded, "+standalone", target("RIGHT", 41)),
		line(classAdded, Escape("+\x1b]52;c;hostile\a"), target("RIGHT", 42)),
	})

	type cellWant struct {
		text   string
		number int
		side   string
		class  lineClass
	}
	type rowWant struct {
		old, new  *cellWant
		full      string
		fullClass lineClass
	}
	want := []rowWant{
		{full: "@@ -10,5 +20,5 @@", fullClass: classHunk},
		{old: &cellWant{text: " shared", number: 10, class: classContext}, new: &cellWant{text: " shared", number: 20, side: "RIGHT", class: classContext}},
		{old: &cellWant{text: "-old one", number: 11, side: "LEFT", class: classRemoved}, new: &cellWant{text: "+new one", number: 21, side: "RIGHT", class: classAdded}},
		{old: &cellWant{text: "-old two", number: 12, side: "LEFT", class: classRemoved}, new: &cellWant{text: "+new two", number: 22, side: "RIGHT", class: classAdded}},
		{new: &cellWant{text: "+new three", number: 23, side: "RIGHT", class: classAdded}},
		{old: &cellWant{text: " tail", number: 13, class: classContext}, new: &cellWant{text: " tail", number: 24, side: "RIGHT", class: classContext}},
		{old: &cellWant{text: "-gone", number: 14, side: "LEFT", class: classRemoved}},
		{full: `\ No newline at end of file`, fullClass: classContext},
		{full: "@@ -30 +40,3 @@", fullClass: classHunk},
		{old: &cellWant{text: " second hunk", number: 30, class: classContext}, new: &cellWant{text: " second hunk", number: 40, side: "RIGHT", class: classContext}},
		{new: &cellWant{text: "+standalone", number: 41, side: "RIGHT", class: classAdded}},
		{new: &cellWant{text: Escape("+\x1b]52;c;hostile\a"), number: 42, side: "RIGHT", class: classAdded}},
	}
	if len(rows) != len(want) {
		t.Fatalf("projected %d rows, want %d: %#v", len(rows), len(want), rows)
	}
	for i, expected := range want {
		got := rows[i]
		if expected.full != "" {
			if got.full == nil || got.full.Text != expected.full || got.full.Class != expected.fullClass || got.old != nil || got.new != nil {
				t.Errorf("row %d = %#v, want full %q", i, got, expected.full)
			}
			continue
		}
		check := func(side string, got *diffCell, expected *cellWant) {
			t.Helper()
			if expected == nil {
				if got != nil {
					t.Errorf("row %d %s cell = %#v, want blank", i, side, got)
				}
				return
			}
			if got == nil || got.line == nil || got.line.Text != expected.text || got.line.Class != expected.class || got.number != expected.number {
				t.Errorf("row %d %s cell = %#v, want text=%q number=%d", i, side, got, expected.text, expected.number)
				return
			}
			if expected.side == "" {
				if got.line.target != nil {
					t.Errorf("row %d %s target = %#v, want nil", i, side, got.line.target)
				}
			} else if got.line.target == nil || got.line.target.Side != expected.side || got.line.target.Line != expected.number {
				t.Errorf("row %d %s target = %#v, want %s line %d", i, side, got.line.target, expected.side, expected.number)
			}
		}
		check("old", got.old, expected.old)
		check("new", got.new, expected.new)
	}
}

func TestSideBySideRenderAtMinimumWidth(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 160, 12
	m.Focus, m.cursorActive = paneDiff, true
	m.layout = diffLayoutSideBySide

	view := ansi.Strip(m.reviewViewForLayout(true))
	for _, want := range []string{"@@ -1 +1 @@", "    1 - old greeting", "│ ›     1 + hello world"} {
		if !strings.Contains(view, want) {
			t.Fatalf("split view missing %q:\n%s", want, view)
		}
	}
	if !containsLine(view, "old greeting", "hello world") {
		t.Fatalf("split source cells did not share a display row:\n%s", view)
	}
	if !containsLine(view, "› ", "old greeting", "hello world") {
		t.Fatalf("split view did not render the active diff cursor:\n%s", view)
	}
}

func TestSideBySideFallbackBelowMinimumWidth(t *testing.T) {
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	m.Loading = false
	m.Session = screenSession()
	m.Width, m.Height = 159, 12

	view := ansi.Strip(m.reviewViewForLayout(true))
	if !strings.Contains(view, "side-by-side needs 160 columns") {
		t.Fatalf("narrow fallback notice missing:\n%s", view)
	}
	if !strings.Contains(view, "-old greeting") || !strings.Contains(view, "+hello world") {
		t.Fatalf("narrow fallback did not keep unified diff:\n%s", view)
	}
	if containsLine(view, "old greeting", "hello world") {
		t.Fatalf("narrow fallback rendered split source cells:\n%s", view)
	}
}

func containsLine(text string, parts ...string) bool {
	for _, line := range strings.Split(text, "\n") {
		matches := true
		for _, part := range parts {
			matches = matches && strings.Contains(line, part)
		}
		if matches {
			return true
		}
	}
	return false
}

func TestSplitMouseSelectionMarksOnlySelectedCell(t *testing.T) {
	s := screenSession()
	s.Inventory.Files[0].OldPath = []byte("main.go")
	m := largeModel(s, 160, 16)
	m.layout = diffLayoutSideBySide
	g := m.workspaceGeometry()
	for index, line := range m.displayDetail() {
		if line.sideBySide == nil || len(rowTargets(*line.sideBySide)) != 2 {
			continue
		}
		m.mouseReviewClick(g.Detail.Min.X, g.Detail.Min.Y+index)
		rendered := m.renderProjectedSideBySideDetail(m.displayDetail(), m.detailWidth(), 0)[index].Text
		old, new := splitCellBounds(m.detailWidth())
		if !strings.HasPrefix(rendered, cursorMarker(true)) {
			t.Fatalf("old marker absent: %q", rendered)
		}
		if strings.Contains(rendered[new.Min.X:], cursorMarker(true)) {
			t.Fatalf("new cell marked: %q", rendered)
		}
		if visibleWidth(rendered) > m.detailWidth() || old.Dx() != new.Dx() {
			t.Fatal("split bounds overflow")
		}
		return
	}
	t.Fatal("missing paired row")
}
