package tui

import (
	"charm.land/lipgloss/v2"
	"strings"
	"testing"
)

func TestCommitCursorUsesMarkerPreservingRowStyle(t *testing.T) {
	m := commitModel(t)
	m.commitSelection()
	m.commit.focus = paneDiff
	rows := m.commitRows()
	for i, row := range rows {
		if row.Class == classAdded {
			m.commit.cursors = map[string]int{m.commit.selectedSHA: i}
			break
		}
	}
	m.styles = map[lineClass]lipgloss.Style{classSelectionFocused: lipgloss.NewStyle().Transform(func(s string) string { return "SELECTED:" + s })}
	got := m.commitsView()
	markedSource := false
	for _, line := range strings.Split(got, "\n") {
		cells := strings.Split(line, "│")
		if len(cells) > 2 && strings.HasPrefix(cells[2], cursorMarker(true)) && strings.Contains(cells[2], "+First commit") {
			markedSource = true
		}
	}
	if strings.Contains(got, "SELECTED:") || !markedSource {
		t.Fatalf("cursor should use marker, not selection style: %s", got)
	}
}

func TestLineNumbersToggleAcrossFilesAndCommits(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	row := diffLine{styledLine: styledLine{Class: classAdded, Text: "+hello"}, newLine: 42}
	assertNumber := func(want bool) {
		t.Helper()
		got := m.presentUnifiedDiffLine(row, 0, 80, "").Text
		if strings.Contains(got, "42") != want {
			t.Fatalf("number visible=%v: %q", want, got)
		}
	}
	assertNumber(true)
	key(m, 'L')
	assertNumber(false)
	key(m, '4')
	assertNumber(false)
	key(m, 'L')
	assertNumber(true)
	key(m, '2')
	assertNumber(true)
}

func TestLineNumberToggleKeepsRawCoordinatesAndSplitTargets(t *testing.T) {
	m := commitModel(t)
	row := diffLine{styledLine: styledLine{Class: classAdded, Text: "+hello"}, newLine: 42}
	cell := &diffCell{line: &row, number: 42}
	before := m.renderSideBySideCell(cell, 30, 0)
	key(m, 'L')
	after := m.renderSideBySideCell(cell, 30, 0)
	if !strings.Contains(before, "42") || strings.Contains(after, "42") || !strings.Contains(after, "+ hello") {
		t.Fatalf("split toggle: %q -> %q", before, after)
	}
	if row.newLine != 42 || row.Text != "+hello" {
		t.Fatal("toggle changed source")
	}
	structural := diffLine{styledLine: styledLine{Class: classHunk, Text: "@@ -1 +2 @@"}}
	if got := m.lineNumberPrefix(structural); got != "" {
		t.Fatalf("structural prefix=%q", got)
	}
}

func TestLineNumberToggleRestoresWrappedCursorTarget(t *testing.T) {
	s := screenSession()
	s.Inventory.Patches["patch"] = []byte("@@ -1 +1,2 @@\n-old\n+" + strings.Repeat("long source words ", 12) + "\n+last\n")
	m := largeModel(s, 100, 18)
	m.Focus, m.cursorActive = paneDiff, true
	for i, row := range m.displayDetail() {
		if row.newLine == 2 {
			m.setCursor(i)
			break
		}
	}
	target, _ := m.cursorAnchor()
	if target == nil || target.Line != 2 {
		t.Fatal("missing cursor target")
	}
	before := len(m.displayDetail())
	key(m, 'L')
	got, _ := m.cursorAnchor()
	if got == nil || *got != *target {
		t.Fatalf("toggle retargeted cursor: %#v -> %#v", target, got)
	}
	if len(m.displayDetail()) >= before {
		t.Fatal("fixture did not exercise changed wrapping")
	}
}
