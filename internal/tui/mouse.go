package tui

import tea "charm.land/bubbletea/v2"

// Mouse routing obeys the same exclusive surface ownership as keyboard input.
func (m *Model) mouseAvailable() bool {
	if m.Loading || m.Busy || m.Err != nil || m.Composer != nil || m.CommentMenu != nil || m.loadingModal().active {
		return false
	}
	switch m.top() {
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
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft || click.X < 0 || click.Y < 0 || click.X >= m.Width || click.Y >= m.Height {
		return nil
	}
	m.pendingCenter = false
	if m.top() == pageReview {
		return m.mouseReviewClick(click.X, click.Y)
	}
	return m.mousePickerClick(click.X, click.Y)
}
