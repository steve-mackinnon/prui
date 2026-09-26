package tui

import (
	tea "charm.land/bubbletea/v2"
	"pr-review/internal/theme"
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
		picker, headers, rows = &m.PullRequestPicker, 1, len(m.PullRequests)
		if rows > 0 {
			details = len(m.pullRequestDetail(m.PullRequests[max(0, min(picker.Index, rows-1))]))
		}
		if m.Session != nil {
			results := m.switcherResults()
			headers, rows, details = 2, len(results), 0
			if rows > 0 {
				selected := results[max(0, min(picker.Index, rows-1))]
				for _, pr := range m.PullRequests {
					if pr.Identity == selected.identity {
						details = len(m.pullRequestDetail(pr))
						break
					}
				}
			}
		}
	case pageThemePicker:
		left, top, width, available := themePickerBounds(m.Width, m.Height)
		index := y - top - 2
		if available && x > left && x < left+width-1 && index >= 0 && index < len(theme.BuiltInNames()) {
			m.ThemePicker.Index = index
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
