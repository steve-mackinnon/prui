package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

func TestEditorCursorAtWholeGraphemes(t *testing.T) {
	for _, cluster := range []string{"🇨🇦", "👍🏽", "✈️", "e\u0301", "界"} {
		for _, overlay := range []bool{false, true} {
			for _, wrapped := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/overlay=%t/wrapped=%t", cluster, overlay, wrapped), func(t *testing.T) {
					prefix, row, width := "", 0, 100
					if wrapped {
						prefix, row, width = "abcd", 1, 4
					}
					body := prefix + cluster + "X"
					for _, cursor := range []int{len(prefix), utf8.RuneCountInString(body)} {
						shift := 0
						if !overlay && cursor == len(prefix) {
							shift = 1
						}
						layout := editorCursorLayout{width: width, hardwrap: true, multiline: true, overlayCaret: overlay}
						for x := shift; x < shift+ansi.StringWidth(cluster); x++ {
							got, ok := editorCursorAt(body, cursor, layout, x, row)
							if !ok || got != len(prefix) {
								t.Fatalf("cursor %d cluster cell %d: got %d, %t; want %d", cursor, x, got, ok, len(prefix))
							}
						}
						got, ok := editorCursorAt(body, cursor, layout, shift+ansi.StringWidth(cluster), row)
						want := len(prefix) + utf8.RuneCountInString(cluster)
						if !ok || got != want {
							t.Fatalf("cursor %d X: got %d, %t; want %d", cursor, got, ok, want)
						}
					}
				})
			}
		}
	}
}

func TestMouseInlineGraphemeFromRenderedCells(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = kindsSession()
	m.Width, m.Height = 120, 28
	m.Files, m.Selected, m.Focus, m.cursorActive = true, 1, paneDiff, true
	metadata := m.Session.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: metadata.Identity, CommitID: metadata.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	draft := "👍🏽X"
	m.Composer = &commentComposer{Target: target, Draft: draft, Cursor: 3, PendingIndex: -1}
	m.ensureInlineEditorVisible()
	// Derive the screen cell from actual rendered output, independently of the mapper.
	found := false
	for y, line := range strings.Split(ansi.Strip(m.reviewView()), "\n") {
		if at := strings.Index(line, draft); at >= 0 {
			x := ansi.StringWidth(line[:at] + "👍🏽")
			modalClick(m, x, y)
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rendered draft missing")
	}
	if m.Composer.Cursor != 2 {
		t.Fatalf("X click moved to rune %d; want 2", m.Composer.Cursor)
	}
	if m.Composer.Draft != draft || m.Composer.Target != target || m.Busy {
		t.Fatal("mouse click changed draft, target or submission state")
	}
}

func TestEditorCursorAtEscapedJoiner(t *testing.T) {
	body := "👩‍💻X"
	layout := editorCursorLayout{width: 100, hardwrap: true, multiline: true, overlayCaret: true}
	for _, tt := range []struct{ x, want int }{{2, 1}, {7, 1}, {8, 2}, {9, 2}, {10, 3}} {
		got, ok := editorCursorAt(body, 4, layout, tt.x, 0)
		if !ok || got != tt.want {
			t.Fatalf("escaped joiner cell %d: got %d, %t; want %d (display %q)", tt.x, got, ok, tt.want, Escape(body))
		}
	}
}
