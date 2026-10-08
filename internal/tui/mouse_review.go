package tui

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

// mouseReviewClick selects only a visible semantic item. It never activates
// guide actions, editors, or remote operations.
func (m *Model) mouseReviewClick(x, y int) tea.Cmd {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height || m.Session == nil {
		return nil
	}
	if y == 1 {
		start := 0
		for i, label := range m.contextTabLabels() {
			width := visibleWidth(label)
			if x >= start && x < start+width {
				if i == 0 {
					return m.openSwitcher()
				}
				m.selectReviewView(reviewViews[i-1])
				return m.restartGuidePathScroll()
			}
			start += width + 2
		}
		return nil
	}
	if m.selectedReviewView() == viewDescription {
		m.overviewMouseClick(y)
		return nil
	}
	if m.selectedReviewView() == viewCommits {
		g := m.commitGeometry()
		p := image.Pt(x, y)
		if p.In(g.Detail) {
			m.commit.focus = paneDiff
			row := m.commitOffset() + y - g.Detail.Min.Y
			rows := m.commitRows()
			if row >= 0 && row < len(rows) && rows[row].target != nil {
				m.commitCursor()
				m.commit.cursors[m.commit.selectedSHA] = row
			}
		}
		if p.In(g.Rail) {
			rows := m.commitRail()
			i := y - g.Rail.Min.Y
			if i < len(rows) && rows[i].row >= 0 {
				m.selectCommit(rows[i].row)
				m.commit.focus = paneList
				m.commitOffset()
			}
		}
		return nil
	}
	if !m.diffReviewView() || len(m.Session.Inventory.Units) == 0 {
		return nil
	}
	if m.fileView() && y == 2 {
		_, control := m.fileFilterHeader()
		if image.Pt(x, y).In(control) {
			m.openFileFilter()
			return m.restartGuidePathScroll()
		}
		return nil
	}
	g := m.workspaceGeometry()
	p := image.Pt(x, y)
	if p.In(g.Rail) {
		list, _ := m.reviewListPresentation()
		i := y - g.Rail.Min.Y
		if i >= len(list) || list[i].row < 0 || x-g.Rail.Min.X >= visibleWidth(list[i].text) {
			return nil
		}
		row := list[i].row
		m.Focus = paneList
		if rows := m.navigable(); rows != nil {
			m.moveRow(rows, row-m.Row)
		} else if m.Inventory {
			m.move(row - m.Selected)
		} else {
			m.selectFile(row)
		}
		return m.restartGuidePathScroll()
	}
	if !p.In(g.Detail) {
		return nil
	}
	lines := m.displayDetail()
	i := min(m.offset(), max(0, len(lines)-1)) + y - g.Detail.Min.Y
	if i < 0 || i >= len(lines) {
		return nil
	}
	line := lines[i]
	target := line.target
	if line.sideBySide != nil && line.sideBySide.full == nil {
		old, new := splitCellBounds(m.detailWidth())
		local := x - g.Detail.Min.X
		var cell *diffCell
		if local >= old.Min.X && local < old.Max.X {
			cell = line.sideBySide.old
		}
		if local >= new.Min.X && local < new.Max.X {
			cell = line.sideBySide.new
		}
		target = nil
		if cell != nil && cell.line != nil {
			target = cell.line.target
		}
	}
	if line.sideBySide == nil {
		text := line.Text
		if !m.sideBySideEnabled() {
			runes := []rune(text)
			text = string(runes[min(max(0, m.Horizontal), len(runes)):])
		}
		gutter := 0
		if m.cursorActive {
			gutter = 2
		}
		gutter += visibleWidth(m.lineNumberPrefix(line))
		local := x - g.Detail.Min.X
		if local >= gutter+visibleWidth(text) {
			return nil
		}
		if line.commentID > 0 && local < gutter+visibleWidth(text)-visibleWidth(strings.TrimLeft(text, " ")) {
			return nil
		}
	}
	if target == nil && line.commentID == 0 {
		return nil
	}
	m.setCursor(i)
	if target != nil {
		m.setSelectedDiffTarget(target)
	}
	m.Focus, m.cursorActive = paneDiff, true
	if m.fileView() {
		m.syncFileToLine(i)
	}
	return m.restartGuidePathScroll()
}

// Split source cells reserve the same cursor gutter on either side. The
// separator and any odd trailing cell have no source target.
func splitCellBounds(width int) (image.Rectangle, image.Rectangle) {
	cellWidth := max(0, (width-3)/2)
	return image.Rect(0, 0, cellWidth, 1), image.Rect(cellWidth+3, 0, cellWidth*2+3, 1)
}

func diffLineHasTarget(line diffLine, target source.ReviewCommentTarget) bool {
	if line.sideBySide != nil {
		for _, candidate := range rowTargets(*line.sideBySide) {
			if candidate == target {
				return true
			}
		}
	}
	return line.target != nil && *line.target == target || line.oldTarget != nil && *line.oldTarget == target
}

func (m *Model) setSelectedDiffTarget(target *source.ReviewCommentTarget) {
	if guide, ok := m.activeGuide(); ok {
		if m.GuideCursorTarget == nil {
			m.GuideCursorTarget = map[int]source.ReviewCommentTarget{}
		}
		if target == nil {
			delete(m.GuideCursorTarget, guide)
		} else {
			m.GuideCursorTarget[guide] = *target
		}
		return
	}
	key := m.Selected
	if m.fileView() {
		key = -1
	}
	if m.CursorTarget == nil {
		m.CursorTarget = map[int]source.ReviewCommentTarget{}
	}
	if target == nil {
		delete(m.CursorTarget, key)
	} else {
		m.CursorTarget[key] = *target
	}
}

func (m *Model) selectedDiffTarget() *source.ReviewCommentTarget {
	index := m.cursor()
	if index < 0 {
		return nil
	}
	return m.selectedDiffTargetForLine(m.displayDetail()[index])
}

func (m *Model) selectedDiffTargetForLine(line diffLine) *source.ReviewCommentTarget {
	var target source.ReviewCommentTarget
	var ok bool
	if guide, found := m.activeGuide(); found {
		target, ok = m.GuideCursorTarget[guide]
	} else {
		key := m.Selected
		if m.fileView() {
			key = -1
		}
		target, ok = m.CursorTarget[key]
	}
	if m.rangeStart != nil && m.rangeStart.Side == "LEFT" && line.oldTarget != nil {
		return line.oldTarget
	}
	if m.rangeStart != nil && line.sideBySide != nil {
		for _, candidate := range rowTargets(*line.sideBySide) {
			if candidate.Side == m.rangeStart.Side {
				return &candidate
			}
		}
	}
	if ok && diffLineHasTarget(line, target) {
		return &target
	}
	return line.target
}

func (m *Model) clearSelectedDiffTarget() { m.setSelectedDiffTarget(nil) }

// Plain labels are shared by rendering and hit testing, before styling or clipping.
func (m *Model) contextTabLabels() []string {
	labels := []string{"Overview [1]", "Files [2]", "Guide [3]", "Commits [4]"}
	for i, label := range labels {
		prefix := "  "
		if reviewViews[i] == m.selectedReviewView() {
			prefix = "› "
		}
		labels[i] = prefix + label
	}
	return append([]string{"  PRs [P]"}, labels...)
}
