package tui

import (
	"fmt"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

// mouseReviewClick selects visible semantic items and activates double clicks
// through the same guarded handlers used by keyboard input.
func (m *Model) mouseReviewClick(x, y int) tea.Cmd {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height || m.Session == nil {
		return nil
	}
	if cmd, handled := m.reviewHeaderClick(x, y); handled {
		return cmd
	}
	if m.rangeStart != nil && y == len(strings.Split(m.reviewView(), "\n"))-1 {
		suffix := "enter: compose · esc: cancel"
		full := *m
		full.Width = max(m.Width, 4096)
		hints := ansi.Strip(full.reviewHints())
		// Only the fixed trailing controls supply actions. Earlier error text
		// contributes geometry but can never supply an action or a label.
		prefix := strings.TrimSuffix(hints, suffix)
		if len(prefix) != len(hints) {
			key := pageMouseFooterKey([]pageMouseControl{{"enter: compose", "enter"}, {"esc: cancel", "esc"}}, " · ", x-visibleWidth(prefix), m.Width-visibleWidth(prefix))
			if key != "" {
				return m.mouseKey(key)
			}
		}
		return nil
	}
	if m.ActionError == nil && !m.ReviewSubmitted && m.rangeStart == nil && y == len(strings.Split(m.reviewView(), "\n"))-1 && m.Height >= 10 {
		// This row contains only locally generated navigation hints. Never
		// interpret source, error text, or remote discussion bodies as controls.
		hints := ansi.Strip(m.reviewHints())
		offset := 0
		for _, part := range strings.Split(hints, " · ") {
			end := offset + visibleWidth(part)
			if x >= offset && x < end && end <= m.Width {
				switch {
				case strings.HasPrefix(part, "Space:"):
					m.openActions()
					return nil
				case strings.HasPrefix(part, "Tab:"):
					return m.mouseKey("tab")
				case strings.HasPrefix(part, "enter:"):
					return m.mouseKey("enter")
				case strings.HasPrefix(part, "esc:"):
					return m.mouseKey("esc")
				}
			}
			offset = end + 3
		}
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
		rows := m.overviewRows()
		index := m.DescriptionScroll + y - 3
		if y >= 3 && y < 3+m.overviewHeight() && index >= 0 && index < len(rows) && rows[index].action == "actions" {
			m.discussions.overviewCursor, m.discussions.overviewCursorActive = index, true
			m.selectOverviewCursor(rows)
			for _, entry := range m.discussionEntries() {
				if entry.ID != rows[index].id {
					continue
				}
				plain := ansi.Strip(rows[index].text)
				for _, label := range m.overviewDiscussionActions(entry) {
					start := strings.Index(plain, label)
					if start < 0 {
						continue
					}
					left := visibleWidth(plain[:start])
					right := left + visibleWidth(label)
					if x >= left && x < right && right <= m.Width {
						key, _, _ := strings.Cut(label, ":")
						cmd, _ := m.overviewKey(key)
						return cmd
					}
				}
			}
			return nil
		}
		m.overviewMouseClick(y)
		rows = m.overviewRows()
		index = m.DescriptionScroll + y - 3
		if y >= 3 && y < 3+m.overviewHeight() && index >= 0 && index < len(rows) && rows[index].action == "" && m.discussions.overviewFocus && m.mouseActivate("overview/"+m.discussions.selectedID) {
			cmd, _ := m.overviewKey("enter")
			return cmd
		}
		return nil
	}
	if m.selectedReviewView() == viewCommits {
		g := m.commitGeometry()
		p := image.Pt(x, y)
		if p.In(g.Detail) {
			m.commit.focus = paneDiff
			row := m.commitOffset() + y - g.Detail.Min.Y
			rows := m.commitRows()
			if row >= 0 && row < len(rows) && (rows[row].target != nil || rows[row].commentID > 0) {
				m.commitCursor()
				m.commit.cursors[m.commit.selectedSHA] = row
				identity := fmt.Sprintf("commit/%s/comment/%d", m.commit.selectedSHA, rows[row].commentID)
				if rows[row].target != nil {
					identity = fmt.Sprintf("commit/%s/%v", m.commit.selectedSHA, *rows[row].target)
				}
				if m.mouseActivate(identity) {
					return m.mouseKey("enter")
				}
			}
		}
		if p.In(g.Rail) {
			rows := m.commitRail()
			i := y - g.Rail.Min.Y
			if i < len(rows) && rows[i].row >= 0 {
				m.selectCommit(rows[i].row)
				m.commit.focus = paneList
				m.commitOffset()
				if m.mouseActivate("commit/" + m.commit.selectedSHA) {
					return m.mouseKey("enter")
				}
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
		if m.mouseActivate(fmt.Sprintf("rail/%d/%d/%d", m.Selected, m.Row, m.ContextView)) {
			return m.mouseKey("enter")
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
	identity := fmt.Sprintf("diff/%d/%d/%v/%d", m.Selected, m.Row, target, line.commentID)
	if target != nil {
		identity = fmt.Sprintf("diff/%d/%d/%v", m.Selected, m.Row, *target)
	}
	if m.mouseActivate(identity) {
		return m.mouseKey("enter")
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
	prs := "  PRs [P]"
	if m.Width < 90 {
		labels = []string{"Overview", "Files", "Guide", "Commits"}
		prs = "PRs"
	}
	for i, label := range labels {
		prefix := "  "
		if m.Width < 90 {
			prefix = ""
		}
		if reviewViews[i] == m.selectedReviewView() {
			prefix = "› "
		}
		labels[i] = prefix + label
	}
	return append([]string{prs}, labels...)
}
