package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
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

// A first click selects; a second click on the same semantic item activates
// through the existing picker handler.
func (m *Model) mousePickerClick(x, y int) tea.Cmd {
	if !m.mousePickerAvailable() || x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return nil
	}
	if cmd, handled := m.mousePickerFooter(x, y); handled {
		return cmd
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
		rowHeight := min(2, g.listHeight)
		capacity := max(1, g.listHeight/rowHeight)
		start := max(0, m.PullRequestPicker.Index-capacity+1)
		index := start + (y-g.listTop)/rowHeight
		if y-g.listTop >= capacity*rowHeight {
			return nil
		}
		if index < rows {
			m.PullRequestPicker.Index = index
			m.PullRequestPicker.previewFocus = false
			m.PullRequestPicker.previewScroll = 0
			var identity string
			if m.Session != nil {
				identity = fmt.Sprint(m.switcherResults()[index].identity)
			} else {
				identity = fmt.Sprint(m.PullRequests[index].Identity)
			}
			if m.mouseActivate("pr/" + identity) {
				return m.pullRequestPickerKey("enter")
			}
		}
		return nil
	case pageThemePicker:
		left, top, width, available := themePickerBounds(m.Width, m.Height)
		index, hit := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index).itemAt(y - top - 1)
		if available && x > left && x < left+width-1 && hit {
			rows := themePickerRows()
			if rows[index].index >= 0 {
				m.ThemePicker.Index = rows[index].index
				if m.mouseActivate(fmt.Sprintf("theme/%d", m.ThemePicker.Index)) {
					return m.themePickerKey("enter")
				}
			}
		}
		return nil
	default:
		return nil
	}
	if index, ok := layoutPicker(m.Height, headers, rows, details, picker.Index).itemAt(y); ok {
		picker.Index = index
		var identity string
		if m.top() == pagePicker {
			identity = m.Entries[index].ID
		} else {
			identity = m.Repositories[index].Repository
		}
		if m.mouseActivate(identity) {
			return m.pageKey(m.top(), tea.KeyPressMsg{Code: tea.KeyEnter})
		}
	}
	return nil
}

func (m *Model) mousePickerFooter(x, y int) (tea.Cmd, bool) {
	if m.ActionError != nil {
		return nil, false
	}
	if m.top() == pageThemePicker {
		left, top, width, available := themePickerBounds(m.Width, m.Height)
		if !available {
			if y != 1 {
				return nil, false
			}
			controls := []pageMouseControl{{"↑↓/jk choose", "down"}, {m.themePickerEnterHint(), "enter"}, {"esc/t cancel", "esc"}}
			return m.themePickerKey(pageMouseFooterKey(controls, " · ", x, m.Width)), true
		}
		window := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index)
		sample := 0
		if m.Width >= 60 && m.Height >= 18 {
			sample = 3
		}
		first := top + 2 + window.end - window.start + sample + 1
		label, key := "", ""
		switch y - first {
		case 0:
			label, key = "j/k or ↑/↓: choose", "down"
		case 1:
			label, key = m.themePickerEnterHint(), "enter"
		case 2:
			label, key = "esc/t: cancel", "esc"
		}
		if key != "" && x >= left+4 && x < left+4+visibleWidth(label) && left+4+visibleWidth(label) <= left+width-2 {
			return m.themePickerKey(key), true
		}
		return nil, false
	}
	if y != m.Height-1 {
		return nil, false
	}
	switch m.top() {
	case pagePicker:
		if layoutPicker(m.Height, 3, len(m.Entries), 0, m.SessionPicker.Index).footerRows == 0 {
			return nil, false
		}
	case pageRepositoryPicker:
		if layoutPicker(m.Height, 2, len(m.Repositories), 0, m.RepositoryPicker.Index).footerRows == 0 {
			return nil, false
		}
	case pagePullRequestPicker:
		headers := 1
		if m.Session != nil {
			headers = 2
		}
		g := m.prPickerGeometry(headers)
		if g.listTop+g.listHeight >= m.Height {
			return nil, false
		}
	}
	var controls []pageMouseControl
	switch m.top() {
	case pagePicker:
		controls = []pageMouseControl{{"enter: resume", "enter"}, {"r: reload", "r"}, {"esc: back", "esc"}}
	case pageRepositoryPicker:
		controls = []pageMouseControl{{"enter: open", "enter"}}
		if m.inbox.load != nil {
			controls = append(controls, pageMouseControl{"i: inbox", "i"})
		}
		controls = append(controls, pageMouseControl{"r: reload", "r"}, pageMouseControl{"esc: back", "esc"})
	case pagePullRequestPicker:
		controls = m.prPickerMouseControls()
	default:
		return nil, false
	}
	help := "?: Health & help"
	if m.top() == pagePullRequestPicker && m.Width < 110 {
		help = "?: help"
	}
	controls = append(controls, pageMouseControl{help, "?"})
	key := pageMouseFooterKey(controls, " · ", x, m.Width)
	if key == "" {
		return nil, true
	}
	if key == "?" {
		m.push(pageHelp)
		return nil, true
	}
	if strings.HasPrefix(key, "ctrl+") {
		return m.pullRequestPickerKey(key), true
	}
	return m.pageKey(m.top(), mouseKey(key)), true
}
func (m *Model) prPickerMouseControls() []pageMouseControl {
	controls := []pageMouseControl{{"j/k: select", "down"}, {"tab: preview", "tab"}}
	if m.Width < 80 && m.Session == nil {
		controls = controls[1:]
	}
	if m.PullRequestPicker.previewFocus {
		controls = []pageMouseControl{{"j/k: scroll", "down"}, {"tab: list", "tab"}}
	}
	if m.Session != nil {
		controls = append(controls, pageMouseControl{"type: filter", ""}, pageMouseControl{"enter: switch/open", "enter"})
	} else {
		controls = append(controls, pageMouseControl{"enter: open", "enter"})
	}
	if m.inbox.load != nil {
		controls = append(controls, pageMouseControl{"ctrl+o: inbox", "ctrl+o"})
	}
	label := "esc: back"
	if m.Session != nil {
		label = "esc: cancel"
	}
	return append(controls, pageMouseControl{label, "esc"})
}
