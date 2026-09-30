package tui

import (
	"image"

	tea "charm.land/bubbletea/v2"
)

const mouseWheelStep = 3

// Mouse routing obeys the same exclusive surface ownership as keyboard input.
func (m *Model) mouseAvailable() bool {
	if m.Loading || m.Busy || m.Err != nil || m.Composer != nil || m.CommentMenu != nil || m.loadingModal().active {
		return false
	}
	switch m.top() {
	case pageDiscussions:
		return m.Session != nil
	case pageReview:
		return m.Session != nil
	case pagePicker, pageRepositoryPicker, pagePullRequestPicker, pageThemePicker:
		return true
	default:
		return false
	}
}

func (m *Model) mouseUpdate(msg tea.MouseMsg) tea.Cmd {
	if !m.mouseAvailable() {
		m.cancelMouseDrag()
		return nil
	}
	event := msg.Mouse()
	if event.Mod != 0 {
		m.cancelMouseDrag()
		return nil
	}
	if cmd, handled := m.mouseResize(msg); handled {
		return cmd
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		return m.mouseWheel(wheel)
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft || click.X < 0 || click.Y < 0 || click.X >= m.Width || click.Y >= m.Height {
		return nil
	}
	m.pendingCenter = false
	if m.top() == pageDiscussions {
		m.discussionMouseClick(click.Y)
		return nil
	}
	if m.top() == pageReview {
		return m.mouseReviewClick(click.X, click.Y)
	}
	return m.mousePickerClick(click.X, click.Y)
}

func (m *Model) mouseWheel(wheel tea.MouseWheelMsg) tea.Cmd {
	delta := 0
	switch wheel.Button {
	case tea.MouseWheelUp:
		delta = -mouseWheelStep
	case tea.MouseWheelDown:
		delta = mouseWheelStep
	default:
		return nil
	}
	if m.top() == pageDiscussions {
		for i := 0; i < mouseWheelStep; i++ {
			if delta < 0 {
				m.discussionKey("up")
			} else {
				m.discussionKey("down")
			}
		}
		return nil
	}
	if wheel.X < 0 || wheel.Y < 0 || wheel.X >= m.Width || wheel.Y >= m.Height || m.top() != pageReview || m.Session == nil {
		return nil
	}
	p := image.Pt(wheel.X, wheel.Y)
	if m.selectedReviewView() == viewDescription {
		if wheel.Y >= 3 && wheel.Y < 3+m.descriptionBodyHeight() {
			m.DescriptionScroll = max(0, min(m.DescriptionScroll+delta, max(0, len(m.descriptionLines())-m.descriptionBodyHeight())))
		}
		return nil
	}
	if m.selectedReviewView() == viewCommits {
		g := m.commitGeometry()
		if p.In(g.Rail) {
			m.commitMove(delta)
		}
		if p.In(g.Detail) {
			m.commitScroll(delta)
		}
		return nil
	}
	if m.selectedReviewView() != viewChanges {
		return nil
	}
	g := m.workspaceGeometry()
	if p.In(g.Detail) {
		m.pendingCenter = false
		m.scroll(delta)
		return nil
	}
	if p.In(g.Rail) {
		m.pendingCenter = false
		m.move(delta)
		return m.restartGuidePathScroll()
	}
	return nil
}
