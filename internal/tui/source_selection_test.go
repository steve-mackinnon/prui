package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

func sourceCellPoint(t *testing.T, m *Model, side string, number int) (int, int) {
	t.Helper()
	g := m.workspaceGeometry().Detail
	for i, row := range m.displayDetail() {
		if i < m.offset() || i >= m.offset()+g.Dy() {
			continue
		}
		if row.sideBySide != nil && row.sideBySide.full == nil {
			old, next := splitCellBounds(m.detailWidth())
			for _, cell := range []struct {
				x    int
				line *diffCell
			}{{g.Min.X + old.Min.X + 1, row.sideBySide.old}, {g.Min.X + next.Min.X + 1, row.sideBySide.new}} {
				if cell.line != nil && cell.line.line != nil && cell.line.line.target != nil && cell.line.line.target.Side == side && cell.line.line.target.Line == number {
					return cell.x, g.Min.Y + i - m.offset()
				}
			}
		} else if row.target != nil && row.target.Side == side && row.target.Line == number {
			return g.Min.X + 1, g.Min.Y + i - m.offset()
		}
	}
	t.Fatalf("target %s:%d unavailable", side, number)
	return 0, 0
}

func TestSourceShiftClickKeepsOriginAcrossMouseWheelScroll(t *testing.T) {
	m := rangeTestModel()
	m.Height = 10
	var patch strings.Builder
	patch.WriteString("@@ -1,0 +1,18 @@\n")
	var sourceLines []string
	for line := 1; line <= 18; line++ {
		text := fmt.Sprintf("line%02d", line)
		patch.WriteString("+" + text + "\n")
		sourceLines = append(sourceLines, text)
	}
	m.Session.Inventory.Patches["p"] = []byte(patch.String())
	m.fileCache = fileDetailCache{}
	m.diffWrapCache = diffWrapCache{}
	m.diffOverlayCache = diffOverlayCache{}
	x1, y1 := sourceCellPoint(t, m, "RIGHT", 1)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	for range 10 {
		m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: x1, Y: y1})
	}
	if m.offset() == 0 {
		t.Fatal("mouse wheel did not scroll the source viewport")
	}
	endLine := 0
	for row, line := range m.displayDetail() {
		if row < m.offset() || row >= m.offset()+m.bodyHeight() || line.target == nil || line.target.Side != "RIGHT" {
			continue
		}
		endLine = max(endLine, line.target.Line)
	}
	if endLine <= 1 {
		t.Fatal("scroll did not reveal a later canonical source line")
	}
	xEnd, yEnd := sourceCellPoint(t, m, "RIGHT", endLine)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: xEnd, Y: yEnd, Mod: tea.ModShift})
	target, err := m.selectedCanonicalRange()
	if err != nil || target.StartLine != 1 || target.Line != endLine {
		t.Fatalf("scrolled Shift-click range = %+v, %v", target, err)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	if got, want := clipboardText(cmd), strings.Join(sourceLines[:endLine], "\n"); got != want {
		t.Fatalf("scrolled range clipboard = %q, want %q", got, want)
	}
}

func TestSourceDragAndShiftClickSelectCanonicalRangeWithoutCopy(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := rangeTestModel()
		if split {
			m.layout = diffLayoutSideBySide
		}
		x1, y1 := sourceCellPoint(t, m, "RIGHT", 1)
		x3, y3 := sourceCellPoint(t, m, "RIGHT", 3)
		_, press := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x3, Y: y3})
		_, motion := m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x1, Y: y1})
		_, release := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x1, Y: y1})
		if press != nil || motion != nil || release != nil {
			t.Fatal("source range gesture emitted clipboard or other command")
		}
		if m.clipboardToast {
			t.Fatal("source release displayed a copy confirmation without explicit Copy")
		}
		rangeTarget, err := m.selectedCanonicalRange()
		if err != nil || rangeTarget.StartLine != 1 || rangeTarget.Line != 3 || rangeTarget.Side != "RIGHT" {
			t.Fatalf("drag range=%+v err=%v", rangeTarget, err)
		}
		if text, err := m.selectedSourceText(); err != nil || !strings.Contains(text, "first\nsecond\nthird") {
			t.Fatalf("source copy=%q err=%v", text, err)
		}
		_, copyCmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
		if got := clipboardText(copyCmd); got != "first\nsecond\nthird" {
			t.Fatalf("explicit copy = %q, want raw source without diff markers or line numbers", got)
		}
		m.selectReviewView(viewGuide)
		if text, err := m.selectedSourceText(); err != nil || !strings.Contains(text, "first\nsecond\nthird") {
			t.Fatalf("view switch changed canonical copy=%q err=%v", text, err)
		}
		m.selectReviewView(viewFiles)
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
		m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x1, Y: y1})
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x3, Y: y3, Mod: tea.ModShift})
		rangeTarget, err = m.selectedCanonicalRange()
		if err != nil || rangeTarget.StartLine != 1 || rangeTarget.Line != 3 {
			t.Fatalf("shift range=%+v err=%v", rangeTarget, err)
		}
	}
}

func TestSourceForwardDragSelectsWithoutClipboard(t *testing.T) {
	for _, split := range []bool{false, true} {
		m := rangeTestModel()
		if split {
			m.layout = diffLayoutSideBySide
		}
		x1, y1 := sourceCellPoint(t, m, "RIGHT", 1)
		x3, y3 := sourceCellPoint(t, m, "RIGHT", 3)
		m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
		m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x3, Y: y3})
		_, release := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x3, Y: y3})
		target, err := m.selectedCanonicalRange()
		if err != nil || target.StartLine != 1 || target.Line != 3 || release != nil || m.clipboardToast {
			t.Fatalf("split=%v forward drag range=%+v err=%v release=%v toast=%v", split, target, err, release != nil, m.clipboardToast)
		}
	}
}

func TestSourceKeyboardRangeCopiesOldSideAcrossNarrowFallbackAndPan(t *testing.T) {
	m := rangeTestModel()
	m.layout = diffLayoutSideBySide
	m.Width = sideBySideMinimumWidth - 1 // retained preference must fall back to unified
	selectRawTarget(t, m, "LEFT", 1)
	m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	target, err := m.selectedCanonicalRange()
	if err != nil || target.Side != "LEFT" || target.StartLine != 1 || target.Line != 2 {
		t.Fatalf("old-side keyboard range = %+v, %v", target, err)
	}
	m.Horizontal = 12
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt})
	if got := clipboardText(cmd); got != "one\ntwo" {
		t.Fatalf("old-side copy after horizontal pan = %q", got)
	}
	if m.layout != diffLayoutSideBySide || m.sideBySideEnabled() {
		t.Fatal("narrow fallback changed saved layout preference")
	}
}

func TestSourceSelectionRejectsBoundaryAndCancelsUnfinishedDrag(t *testing.T) {
	m := rangeTestModel()
	x1, y1 := sourceCellPoint(t, m, "LEFT", 1)
	x2, y2 := sourceCellPoint(t, m, "LEFT", 2)
	xRight, yRight := sourceCellPoint(t, m, "RIGHT", 2)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x2, Y: y2, Mod: tea.ModShift})
	before, err := m.selectedCanonicalRange()
	if err != nil || before.StartLine != 1 || before.Line != 2 {
		t.Fatalf("prior range=%+v err=%v", before, err)
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: xRight, Y: yRight, Mod: tea.ModShift})
	after, err := m.selectedCanonicalRange()
	if err != nil || *after != *before || m.ActionError == nil {
		t.Fatalf("cross-side changed range=%+v err=%v notice=%v", after, err, m.ActionError)
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x2, Y: y2})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	after, err = m.selectedCanonicalRange()
	if err != nil || *after != *before {
		t.Fatalf("cancel lost committed range=%+v err=%v", after, err)
	}
}

func TestSwitchingReviewTabsRestoresCommittedRangeDuringDrag(t *testing.T) {
	m := rangeTestModel()
	first := m.Session
	m.openReviewTab(first)
	second := *first
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/other", Number: 2}
	m.openReviewTab(&second)
	m.activateTab(0)
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff

	x1, y1 := sourceCellPoint(t, m, "RIGHT", 1)
	x2, y2 := sourceCellPoint(t, m, "RIGHT", 2)
	x3, y3 := sourceCellPoint(t, m, "RIGHT", 3)
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x2, Y: y2, Mod: tea.ModShift})
	want, err := m.selectedCanonicalRange()
	if err != nil || want.StartLine != 1 || want.Line != 2 {
		t.Fatalf("committed range=%+v err=%v", want, err)
	}
	m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x3, Y: y3})
	m.Update(tea.MouseMotionMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	if !m.sourceDrag.active {
		t.Fatal("source gesture did not start")
	}
	m.activateTab(1)
	if m.tabs[0].review.sourceDrag.active {
		t.Fatal("departing tab retained an unfinished source gesture")
	}
	m.activateTab(0)
	got, err := m.selectedCanonicalRange()
	if err != nil || *got != *want {
		t.Fatalf("tab switch changed committed range: got=%+v want=%+v err=%v", got, want, err)
	}
}

func TestOpeningPendingEditorClearsSourceRangeEndpoint(t *testing.T) {
	m := rangeTestModel()
	selectRawTarget(t, m, "RIGHT", 1)
	start := *m.selectedDiffTarget()
	m.Pending = []source.ReviewComment{{Target: start, Body: "pending note"}}
	selectRawTarget(t, m, "RIGHT", 3)
	end := *m.selectedDiffTarget()
	m.rangeStart, m.rangeEnd = &start, &end
	for i, row := range m.displayDetail() {
		if row.pendingComment == 1 {
			m.setCursor(i)
			break
		}
	}
	m.openCommentComposer()
	if m.Composer == nil || m.Composer.PendingIndex != 0 {
		t.Fatal("pending editor did not open")
	}
	if m.rangeStart != nil || m.rangeEnd != nil || m.rangeKeyboard {
		t.Fatal("pending editor retained a stale source range")
	}
	m.Composer = nil
	selectRawTarget(t, m, "RIGHT", 2)
	target, err := m.selectedCanonicalRange()
	if err != nil || target.Line != 2 || target.StartLine != 0 {
		t.Fatalf("later source target reused old range: %+v, %v", target, err)
	}
}
