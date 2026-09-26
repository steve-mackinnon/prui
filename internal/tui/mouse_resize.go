package tui

import (
	tea "charm.land/bubbletea/v2"
	"image"
)

// dividerDrag is transient workspace state, never a review-tab preference.
type dividerDrag struct {
	active     bool
	grabOffset int
}

func (m *Model) cancelMouseDrag() { m.drag = dividerDrag{} }

// mouseResize consumes divider gestures before selection sees them. Coordinates
// outside the terminal cancel the gesture; the next fresh press starts anew.
func (m *Model) mouseResize(msg tea.MouseMsg) (tea.Cmd, bool) {
	event := msg.Mouse()
	wasDragging := m.drag.active
	if m.Width < 100 || m.top() != pageReview || m.selectedReviewView() != viewChanges {
		m.cancelMouseDrag()
		return nil, wasDragging
	}
	if _, released := msg.(tea.MouseReleaseMsg); released {
		m.cancelMouseDrag()
		return nil, wasDragging
	}
	if !image.Pt(event.X, event.Y).In(image.Rect(0, 0, m.Width, m.Height)) {
		m.cancelMouseDrag()
		return nil, wasDragging
	}
	switch msg.(type) {
	case tea.MouseClickMsg:
		m.cancelMouseDrag()
		divider := m.workspaceGeometry().Divider
		if event.Button == tea.MouseLeft && event.Mod == 0 && image.Pt(event.X, event.Y).In(divider) {
			m.drag = dividerDrag{active: true, grabOffset: event.X - divider.Min.X}
			return nil, true
		}
		// A new press also terminates a stale drag, without becoming a selection.
		return nil, wasDragging
	case tea.MouseMotionMsg:
		if !m.drag.active {
			return nil, false
		}
		if event.Button != tea.MouseLeft || event.Mod != 0 {
			m.cancelMouseDrag()
			return nil, true
		}
		width := m.clampListWidth(event.X - m.drag.grabOffset)
		if width == m.listWidth() {
			return nil, true
		}
		target, commentID := m.cursorAnchor()
		m.listWidthPreference = width
		m.restoreCursorAnchor(target, commentID)
		m.setOffset(m.clampOffset(m.offset()))
		if m.cursorActive {
			m.ensureCursorVisible()
		}
		m.ensureReplyEditorVisible()
		return m.restartGuidePathScroll(), true
	}
	return nil, wasDragging
}
