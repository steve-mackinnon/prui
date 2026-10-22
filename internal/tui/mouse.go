package tui

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const mouseWheelStep = 3

// Mouse routing obeys the same exclusive surface ownership as keyboard input.
func (m *Model) mouseAvailable() bool {
	if m.Loading || m.Busy || m.Err != nil || m.Composer != nil || m.CommentMenu != nil || m.discussions.editor != nil || m.discussions.published != nil || m.loadingModal().active {
		return false
	}
	switch m.top() {
	case pageReadiness, pageIssueContext, pageDiscussions, pageActions:
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
	m.linkMouseValid = false
	serial := m.mouseClickSerial
	defer func() {
		if click, ok := msg.(tea.MouseClickMsg); ok {
			deferredOverview := m.textSelection.active && m.top() == pageReview && m.selectedReviewView() == viewDescription
			if click.Button != tea.MouseLeft || m.mouseClickSerial == serial && !deferredOverview {
				m.resetMouseActivation()
			}
		}
	}()
	event := msg.Mouse()
	if event.X < 0 || event.Y < 0 || event.X >= m.Width || event.Y >= m.Height || event.Mod != 0 {
		m.resetMouseActivation()
		m.cancelMouseDrag()
		m.textSelection = textSelection{}
		return nil
	}
	switch msg.(type) {
	case tea.MouseWheelMsg:
		m.resetMouseActivation()
	case tea.MouseMotionMsg:
		if event.Button != tea.MouseNone {
			m.resetMouseActivation()
		}
	}
	if cmd, handled := m.mouseModal(msg); handled {
		m.resetMouseActivation()
		m.cancelMouseDrag()
		m.textSelection = textSelection{}
		return cmd
	}
	if m.top() == pageReadiness || m.top() == pageIssueContext {
		if cmd, handled := m.mouseTextSelection(msg); handled {
			return cmd
		}
		if cmd, handled := m.mouseURL(msg); handled {
			if m.textSelection.active {
				m.textSelection.click = cmd
				return nil
			}
			return cmd
		}
	}
	if cmd, handled := m.mousePage(msg); handled {
		return cmd
	}
	if !m.mouseAvailable() {
		m.cancelMouseDrag()
		m.textSelection = textSelection{}
		return nil
	}
	if m.searchOpen() {
		m.textSelection = textSelection{}
		return m.searchMouse(msg)
	}
	if event.Mod != 0 {
		m.cancelMouseDrag()
		m.textSelection = textSelection{}
		return nil
	}
	if m.top() == pageActions {
		m.textSelection = textSelection{}
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft {
			return m.actionsMouseClick(click.X, click.Y)
		}
		return nil
	}
	if m.commitFilter.open {
		m.textSelection = textSelection{}
		if cmd, handled := m.commitFilterMouse(msg); handled {
			return cmd
		}
	}
	if cmd, handled := m.mouseResize(msg); handled {
		m.resetMouseActivation()
		m.textSelection = textSelection{}
		return cmd
	}
	if cmd, handled := m.mouseTextSelection(msg); handled {
		return cmd
	}
	if cmd, handled := m.commitFilterMouse(msg); handled {
		return cmd
	}
	if cmd, handled := m.mouseURL(msg); handled {
		if m.textSelection.active {
			m.textSelection.click = cmd
			return nil
		}
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
	if m.top() == pageReadiness {
		return nil
	}
	if m.top() == pageIssueContext {
		return nil
	}
	if m.top() == pageDiscussions {
		footerY := strings.Count(m.discussionsView(), "\n")
		if click.Y == footerY {
			return m.discussionKey(pageMouseFooterKey(m.discussionMouseControls(), " · ", click.X, m.Width))
		}
		entries := m.discussionEntries()
		if m.discussions.detail && m.discussions.selected >= 0 && m.discussions.selected < len(entries) {
			entry := entries[m.discussions.selected]
			if controls := m.discussionFooterControls(entry); len(controls) > 0 && click.Y == footerY-1 {
				return m.discussionFooterClick(entry, pageMouseFooterKey(controls, " ", click.X, m.Width))
			}
		}
		header, start := m.discussionListStart()
		index := start + click.Y - header
		m.discussionMouseClick(click.Y)
		entries = m.discussionEntries()
		if !m.discussions.detail && click.Y >= header && click.Y < m.Height-1 && index >= 0 && index < len(entries) && m.mouseActivate("discussion/"+entries[index].ID) {
			return m.discussionKey("enter")
		}
		return nil
	}
	if m.searchAvailable() && image.Pt(click.X, click.Y).In(m.searchHeaderBounds()) {
		return m.openSearch()
	}
	if m.top() == pageReview {
		return m.mouseReviewClick(click.X, click.Y)
	}
	return m.mousePickerClick(click.X, click.Y)
}

func (m *Model) mouseWheel(wheel tea.MouseWheelMsg) tea.Cmd {
	m.resetMouseActivation()
	if wheel.X < 0 || wheel.Y < 0 || wheel.X >= m.Width || wheel.Y >= m.Height {
		return nil
	}
	if wheel.Button == tea.MouseWheelLeft || wheel.Button == tea.MouseWheelRight {
		if m.top() == pageReview && m.diffReviewView() && image.Pt(wheel.X, wheel.Y).In(m.workspaceGeometry().Detail) {
			delta := mouseWheelStep
			if wheel.Button == tea.MouseWheelLeft {
				delta = -delta
			}
			m.Horizontal = max(0, m.Horizontal+delta)
		}
		return nil
	}
	var delta int
	switch wheel.Button {
	case tea.MouseWheelUp:
		delta = -mouseWheelStep
	case tea.MouseWheelDown:
		delta = mouseWheelStep
	default:
		return nil
	}
	if m.top() == pageRepositoryPicker || m.top() == pagePicker || m.top() == pageThemePicker {
		key := "down"
		if delta < 0 {
			key = "up"
		}
		for i := 0; i < mouseWheelStep; i++ {
			m.pageKey(m.top(), tea.KeyPressMsg{Code: map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown}[key]})
		}
		return nil
	}
	if m.top() == pagePullRequestPicker {
		headers := 1
		if m.Session != nil {
			headers = 2
		}
		g := m.prPickerGeometry(headers)
		if wheel.X < 0 || wheel.Y < 0 || wheel.X >= m.Width || wheel.Y >= m.Height {
			return nil
		}
		if wheel.Y >= g.previewTop && wheel.Y < g.previewTop+g.previewHeight && (!g.split || wheel.X >= g.leftWidth+3) {
			if pr, ok := m.selectedPickerPR(); ok {
				lines := m.prPickerDetail(pr)
				m.PullRequestPicker.previewScroll = max(0, min(m.PullRequestPicker.previewScroll+delta, max(0, len(lines)-g.previewHeight)))
			}
		} else if wheel.Y >= g.listTop && wheel.Y < g.listTop+g.listHeight && (!g.split || wheel.X < g.leftWidth) {
			count := len(m.PullRequests)
			if m.Session != nil {
				count = len(m.switcherResults())
			}
			m.PullRequestPicker.Index = max(0, min(m.PullRequestPicker.Index+delta, max(0, count-1)))
			m.PullRequestPicker.previewScroll = 0
		}
		return nil
	}
	if m.top() == pageReadiness {
		m.readiness.scroll = max(0, m.readiness.scroll+delta)
		return nil
	}
	if m.top() == pageIssueContext {
		if wheel.Y >= 1 && wheel.Y < m.Height-1 {
			m.issues.scroll = max(0, m.issues.scroll+delta)
		}
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
		if wheel.Y >= 3 && wheel.Y < 3+m.overviewHeight() {
			m.DescriptionScroll = max(0, min(m.DescriptionScroll+delta, max(0, len(m.overviewRows())-m.overviewHeight())))
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
	if !m.diffReviewView() {
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
