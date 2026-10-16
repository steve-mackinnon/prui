package tui

import "prui/internal/source"

// diffPositionPolicy keeps the existing event-specific viewport behavior explicit
// at each caller. Geometry, persistence and path-scroll commands stay there.
type diffPositionPolicy uint8

const (
	positionWindowResize diffPositionPolicy = iota
	positionKeyboardResize
	positionMouseResize
	positionGutterToggle
	positionDisplayToggle
)

// preserveDiffPosition owns the ordering of semantic capture and restoration for
// Files/Guide projections. A raw target retains its split side; captured source
// rows retain source coordinates without becoming commentable. Search visibility
// and editor visibility have intentionally different precedence for each event.
func (m *Model) preserveDiffPosition(policy diffPositionPolicy, change func()) {
	keepSearch := m.searchMatchAtCursor()
	restoreSource := m.sourceCursorRestorer()
	target, commentID := m.cursorAnchor()
	var overviewRelative int
	var preserveOverview bool
	if policy == positionWindowResize {
		overviewRelative, preserveOverview = m.overviewPosition()
	}
	change()
	m.restoreCursorAnchor(target, commentID)
	if restoreSource != nil {
		restoreSource()
	}
	searchLast := policy == positionWindowResize || policy == positionGutterToggle
	if keepSearch && !searchLast {
		m.revealSearchMatch()
	}
	switch policy {
	case positionWindowResize, positionKeyboardResize:
		// A semantic anchor keeps its saved reading offset across split fallback.
		// Keyboard resizing also exempts an active search match from fallback.
		if m.diffLayout() != diffLayoutSideBySide && target == nil && commentID == 0 && restoreSource == nil && (policy == positionWindowResize || !keepSearch) {
			m.cursorInViewport(1)
		}
	case positionMouseResize:
		m.setOffset(m.clampOffset(m.offset()))
		if m.cursorActive {
			m.ensureCursorVisible()
		}
	}
	if policy != positionDisplayToggle {
		m.ensureReplyEditorVisible()
	}
	if preserveOverview {
		m.restoreOverviewPosition(overviewRelative)
	}
	if keepSearch && searchLast {
		m.revealSearchMatch()
	}
}

// sourceCursorRestorer preserves a read-only captured source row across layout
// projections without assigning it a GitHub comment target.
func (m *Model) sourceCursorRestorer() func() {
	if m.navigation.mode == "" && !m.guideExpanded || m.selectedDiffTarget() != nil {
		return nil
	}
	cursor := m.cursor()
	if cursor < 0 {
		return nil
	}
	line := m.displayDetail()[cursor]
	if line.commentID > 0 {
		return nil
	}
	if line.sideBySide != nil {
		if line.sideBySide.new != nil {
			line = *line.sideBySide.new.line
		} else if line.sideBySide.old != nil {
			line = *line.sideBySide.old.line
		}
	}
	if line.searchID.Unit == "" {
		return nil
	}
	id, offset := line.searchID, line.sourceOffset
	return func() {
		fallback := -1
		for i, row := range m.displayDetail() {
			cells := []diffLine{row}
			if row.sideBySide != nil {
				cells = nil
				if row.sideBySide.new != nil {
					cells = append(cells, *row.sideBySide.new.line)
				}
				if row.sideBySide.old != nil {
					cells = append(cells, *row.sideBySide.old.line)
				}
			}
			for _, cell := range cells {
				if cell.searchID == id {
					fallback = i
					if cell.sourceOffset >= offset {
						m.setCursor(i)
						return
					}
				}
			}
		}
		if fallback >= 0 {
			m.setCursor(fallback)
		}
	}
}

// restoreCursorAnchor keeps a semantic selection stable while a resize or
// layout toggle changes the number of display rows above it.
func (m *Model) restoreCursorAnchor(target *source.ReviewCommentTarget, commentID int64) {
	if target == nil && commentID == 0 {
		return
	}
	for i, line := range m.displayDetail() {
		if target != nil && diffLineHasTarget(line, *target) {
			m.setCursor(i)
			m.setSelectedDiffTarget(target)
			return
		}
		if target == nil && commentID > 0 && line.commentID == commentID {
			m.setCursor(i)
			return
		}
	}
}
