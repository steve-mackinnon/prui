package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestDiffWrappingKeepsTargetsAndLongTokens(t *testing.T) {
	target := &source.ReviewCommentTarget{Path: "file.go", Side: "RIGHT", Line: 9}
	line := diffLine{styledLine: styledLine{classAdded, "+alpha beta gamma delta"}, target: target}
	rows := wrapDiffLines([]diffLine{line}, 13)
	if len(rows) < 2 {
		t.Fatal("long source line was clipped instead of wrapped")
	}
	for _, row := range rows {
		if row.target != target || visibleWidth(row.Text) > 13 {
			t.Fatalf("invalid wrapped source row: %#v", row)
		}
	}
	if !strings.Contains(rows[len(rows)-1].Text, "delta") {
		t.Fatal("lost trailing source")
	}
	token := strings.Repeat("x", 80)
	line.Text = "+" + token
	rows = wrapDiffLines([]diffLine{line}, 13)
	if len(rows) != 1 || rows[0].Text != line.Text {
		t.Fatal("long token must remain available for horizontal scrolling")
	}
}

func TestSplitDiffWrappingKeepsBothSides(t *testing.T) {
	old := diffLine{styledLine: styledLine{classRemoved, "-alpha beta gamma delta"}}
	new := diffLine{styledLine: styledLine{classAdded, "+one two three four five six seven"}}
	row := diffRow{old: &diffCell{line: &old, number: 3}, new: &diffCell{line: &new, number: 3}}
	rows := wrapDiffLines([]diffLine{{sideBySide: &row}}, 43)
	if len(rows) < 2 {
		t.Fatal("split source line was not wrapped")
	}
	m := New(context.Background(), nil)
	rendered := m.renderSideBySideViewport(rows, 43, 0, -1, nil)
	var text string
	for _, line := range rendered {
		text += line.Text + "\n"
		if visibleWidth(line.Text) > 43 {
			t.Fatal("split row exceeds pane")
		}
	}
	if !strings.Contains(text, "delta") || !strings.Contains(text, "seven") {
		t.Fatalf("lost source tails: %s", text)
	}
}

func TestWrappedDiffViewportScrollsAndAttachesOverlayOnce(t *testing.T) {
	for _, split := range []bool{false, true} {
		s := largeTextSession(1, 1)
		s.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+" + strings.Repeat("word ", 45) + "TAIL\n+" + strings.Repeat("x", 180) + "SCROLLEND\n")
		m := largeModel(s, 180, 60)
		if split {
			m.layout = diffLayoutSideBySide
		}
		m.Focus = paneDiff
		m.cursorActive = true
		rows := m.displayDetail()
		var target *source.ReviewCommentTarget
		for _, line := range rows {
			if line.target != nil && line.target.Line == 1 && line.target.Side == "RIGHT" {
				target = line.target
				break
			}
		}
		if target == nil {
			t.Fatal("lost source target")
		}
		m.Comments = []source.ReviewComment{{ID: 42, Target: *target, Body: "overlay-once"}}
		view := m.reviewView()
		if !strings.Contains(view, "TAIL") || strings.Count(view, "overlay-once") != 1 {
			t.Fatalf("wrapping/overlay failed, split=%v:\n%s", split, view)
		}
		for i := 0; i < 18; i++ {
			namedKey(m, tea.KeyRight)
		}
		if !strings.Contains(m.reviewView(), "SCROLLEND") {
			t.Fatalf("horizontal scroll lost overflow, split=%v", split)
		}
		namedKey(m, tea.KeyHome)
		if m.Horizontal != 0 {
			t.Fatal("home did not reset horizontal scroll")
		}
	}
}
