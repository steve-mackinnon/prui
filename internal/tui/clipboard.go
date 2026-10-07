package tui

import tea "charm.land/bubbletea/v2"

func (m *Model) copyItemURL(target string) tea.Cmd {
	if !validBrowserURL(target) {
		m.notice = "No URL available for this item"
		return nil
	}
	m.notice = "URL sent to clipboard"
	return tea.SetClipboard(target)
}

func (m *Model) copyDiscussionURL() tea.Cmd {
	entries := m.discussionEntries()
	i := m.discussions.selected
	if i < 0 || i >= len(entries) {
		return m.copyItemURL("")
	}
	t := entries[i]
	target := t.URL
	if len(t.Comments) > 0 && t.Comments[0].URL != "" {
		target = t.Comments[0].URL
	}
	return m.copyItemURL(target)
}

func (m *Model) copyCommentURL(id int64) tea.Cmd {
	for _, comment := range m.Comments {
		if comment.ID == id {
			return m.copyItemURL(comment.URL)
		}
	}
	for _, thread := range m.discussions.snapshot.Snapshot.Threads {
		for _, comment := range thread.Comments {
			if comment.ID == id {
				return m.copyItemURL(comment.URL)
			}
		}
	}
	return m.copyItemURL("")
}
