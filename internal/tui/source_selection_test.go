package tui

import (
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
		rangeTarget, err := m.selectedCanonicalRange()
		if err != nil || rangeTarget.StartLine != 1 || rangeTarget.Line != 3 || rangeTarget.Side != "RIGHT" {
			t.Fatalf("drag range=%+v err=%v", rangeTarget, err)
		}
		if text, err := m.selectedSourceText(); err != nil || !strings.Contains(text, "first\nsecond\nthird") {
			t.Fatalf("source copy=%q err=%v", text, err)
		}
		if m.copySelectedSource() == nil {
			t.Fatal("explicit source copy did not emit a command")
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
