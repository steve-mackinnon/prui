package tui

import (
	"errors"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

// A drag is a gesture, while rangeStart/rangeEnd are the tab's committed
// semantic selection. Cancelling a gesture restores its prior selection.
type sourceRangeDrag struct {
	active               bool
	start, end           source.ReviewCommentTarget
	priorStart, priorEnd *source.ReviewCommentTarget
	priorKeyboard        bool
	startPoint           image.Point
}

func copyTarget(t *source.ReviewCommentTarget) *source.ReviewCommentTarget {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

func (m *Model) cancelSourceDrag() {
	if m.sourceDrag.active {
		m.rangeStart = copyTarget(m.sourceDrag.priorStart)
		m.rangeEnd = copyTarget(m.sourceDrag.priorEnd)
		m.rangeKeyboard = m.sourceDrag.priorKeyboard
	}
	m.sourceDrag = sourceRangeDrag{}
}

// sourceTargetAt maps a terminal cell to a canonical patch target. Its row
// index is useful only for locating the cursor; the target owns the selection.
func (m *Model) sourceTargetAt(x, y int) (*source.ReviewCommentTarget, int) {
	if m.Session == nil || m.top() != pageReview || !m.diffReviewView() || m.commitFilter.subset || m.searchOpen() || !m.mouseAvailable() {
		return nil, -1
	}
	g := m.workspaceGeometry().Detail
	if !image.Pt(x, y).In(g) {
		return nil, -1
	}
	lines := m.displayDetail()
	i := min(m.offset(), max(0, len(lines)-1)) + y - g.Min.Y
	if i < 0 || i >= len(lines) {
		return nil, -1
	}
	line := lines[i]
	if line.sideBySide != nil {
		if line.sideBySide.full != nil {
			return nil, -1
		}
		old, next := splitCellBounds(m.detailWidth())
		local := x - g.Min.X
		var cell *diffCell
		if local >= old.Min.X && local < old.Max.X {
			cell = line.sideBySide.old
		}
		if local >= next.Min.X && local < next.Max.X {
			cell = line.sideBySide.new
		}
		if cell == nil || cell.line == nil || cell.line.target == nil {
			return nil, -1
		}
		return copyTarget(cell.line.target), i
	}
	if line.target == nil {
		return nil, -1
	}
	return copyTarget(line.target), i
}

func (m *Model) setSourceRangeEnd(end source.ReviewCommentTarget) error {
	if m.rangeStart == nil {
		return errors.New("range origin unavailable")
	}
	if _, err := m.completeCommentRange(end); err != nil {
		return err
	}
	m.rangeEnd = &end
	m.ActionError = nil
	return nil
}

func (m *Model) focusSourceTarget(target *source.ReviewCommentTarget, row int) {
	if target == nil || row < 0 {
		return
	}
	m.Focus = paneDiff
	m.cursorActive = true
	m.setCursor(row)
	m.setSelectedDiffTarget(target)
	if m.fileView() {
		m.syncFileToLine(row)
	}
}

func (m *Model) mouseSourceRange(msg tea.MouseMsg) (tea.Cmd, bool) {
	event := msg.Mouse()
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
		if event.Mod == 0 {
			if _, linked := m.mouseURL(msg); linked {
				return nil, false
			}
		}
		target, row := m.sourceTargetAt(click.X, click.Y)
		if target == nil {
			return nil, false
		}
		if event.Mod == tea.ModShift {
			if m.rangeStart == nil {
				m.rangeStart = copyTarget(target)
			}
			m.rangeKeyboard = false
			if err := m.setSourceRangeEnd(*target); err != nil {
				m.ActionError = err
			} else {
				m.focusSourceTarget(target, row)
			}
			return nil, true
		}
		if event.Mod != 0 {
			return nil, false
		}
		m.sourceDrag = sourceRangeDrag{active: true, start: *target, end: *target, priorStart: copyTarget(m.rangeStart), priorEnd: copyTarget(m.rangeEnd), startPoint: image.Pt(click.X, click.Y)}
		m.sourceDrag.priorKeyboard = m.rangeKeyboard
		m.rangeStart, m.rangeEnd = copyTarget(target), copyTarget(target)
		m.rangeKeyboard = false
		return nil, true
	}
	if !m.sourceDrag.active {
		return nil, false
	}
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if event.Button != tea.MouseLeft {
			m.cancelSourceDrag()
			return nil, true
		}
		if target, _ := m.sourceTargetAt(event.X, event.Y); target != nil {
			if err := m.setSourceRangeEnd(*target); err == nil {
				m.sourceDrag.end = *target
			}
		}
		return nil, true
	case tea.MouseReleaseMsg:
		drag := m.sourceDrag
		if target, _ := m.sourceTargetAt(event.X, event.Y); target != nil {
			if err := m.setSourceRangeEnd(*target); err == nil {
				drag.end = *target
			}
		}
		m.sourceDrag = sourceRangeDrag{}
		for row, line := range m.displayDetail() {
			if diffLineHasTarget(line, drag.end) {
				m.focusSourceTarget(&drag.end, row)
				break
			}
		}
		// Preserve ordinary click navigation and double click activation.
		if drag.start == drag.end && image.Pt(event.X, event.Y) == drag.startPoint {
			return m.mouseReviewClick(event.X, event.Y), true
		}
		return nil, true
	}
	return nil, false
}

func (m *Model) selectedSourceText() (string, error) {
	t, err := m.selectedCanonicalRange()
	if err != nil {
		return "", err
	}
	start := t.Line
	if t.StartLine > 0 {
		start = t.StartLine
	}
	var out []string
	seen := make(map[int]bool)
	for unit := range m.Session.Inventory.Units {
		for _, row := range unitLines(m.Session, unit) {
			cells := []diffLine{row}
			if row.sideBySide != nil {
				cells = nil
				if row.sideBySide.old != nil && row.sideBySide.old.line != nil {
					cells = append(cells, *row.sideBySide.old.line)
				}
				if row.sideBySide.new != nil && row.sideBySide.new.line != nil {
					cells = append(cells, *row.sideBySide.new.line)
				}
			}
			for _, cell := range cells {
				candidate := cell.target
				if t.Side == "LEFT" && cell.oldTarget != nil {
					candidate = cell.oldTarget
				}
				if candidate == nil || candidate.Path != t.Path || candidate.Side != t.Side || candidate.CommitID != t.CommitID || candidate.Line < start || candidate.Line > t.Line || seen[candidate.Line] {
					continue
				}
				out = append(out, cell.rawSource)
				seen[candidate.Line] = true
			}
		}
	}
	if len(seen) != t.Line-start+1 {
		return "", errors.New("selected source lines are unavailable")
	}
	return strings.Join(out, "\n"), nil
}

func sourceRangeContainsTarget(selection *source.ReviewCommentTarget, candidate *source.ReviewCommentTarget) bool {
	if selection == nil || candidate == nil || selection.Identity != candidate.Identity || selection.CommitID != candidate.CommitID || selection.Path != candidate.Path || selection.Side != candidate.Side {
		return false
	}
	start := selection.Line
	if selection.StartLine > 0 {
		start = selection.StartLine
	}
	return candidate.Line >= start && candidate.Line <= selection.Line
}

func (m *Model) copySelectedSource() tea.Cmd {
	text, err := m.selectedSourceText()
	if err != nil {
		m.ActionError = err
		return nil
	}
	return m.copyText(text)
}
