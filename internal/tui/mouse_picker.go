package tui

import (
	tea "charm.land/bubbletea/v2"
)

func (m *Model) mousePickerAvailable() bool {
	if m.Busy || m.Loading || m.Composer != nil || m.CommentMenu != nil {
		return false
	}
	switch m.top() {
	case pagePicker, pageRepositoryPicker, pagePullRequestPicker, pageThemePicker:
		return true
	default:
		return false
	}
}

// mousePickerClick selects only. Opening a review or persisting a theme remains
// an explicit keyboard activation through the existing picker handlers.
func (m *Model) mousePickerClick(x, y int) tea.Cmd {
	if !m.mousePickerAvailable() || x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return nil
	}
	var picker *pickerState
	var headers, rows, details int
	switch m.top() {
	case pagePicker:
		picker, headers, rows = &m.SessionPicker, 3, len(m.Entries)
	case pageRepositoryPicker:
		picker, headers, rows = &m.RepositoryPicker, 2, len(m.Repositories)
	case pagePullRequestPicker:
		headers, rows = 1, len(m.PullRequests)
		if m.Session != nil {
			headers, rows = 2, len(m.switcherResults())
		}
		g := m.prPickerGeometry(headers)
		preview := g.previewHeight > 0 && y >= g.previewTop && y < g.previewTop+g.previewHeight
		if g.split {
			preview = preview && x >= g.leftWidth+3
		}
		if preview {
			m.PullRequestPicker.previewFocus = true
			return nil
		}
		if y < g.listTop || y >= g.listTop+g.listHeight || g.split && x >= g.leftWidth {
			return nil
		}
		start := max(0, m.PullRequestPicker.Index-g.listHeight+1)
		index := start + y - g.listTop
		if index < rows {
			m.PullRequestPicker.Index = index
			m.PullRequestPicker.previewFocus = false
			m.PullRequestPicker.previewScroll = 0
		}
		return nil
	case pageThemePicker:
		left, top, width, available := themePickerBounds(m.Width, m.Height)
		index, hit := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index).itemAt(y - top - 1)
		if available && x > left && x < left+width-1 && hit {
			rows := themePickerRows()
			if rows[index].index >= 0 {
				m.ThemePicker.Index = rows[index].index
			}
		}
		return nil
	default:
		return nil
	}
	if index, ok := layoutPicker(m.Height, headers, rows, details, picker.Index).itemAt(y); ok {
		picker.Index = index
	}
	return nil
}
