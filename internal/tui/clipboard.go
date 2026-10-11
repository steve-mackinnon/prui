package tui

import (
	tea "charm.land/bubbletea/v2"
	"prui/internal/guideconfig"
	"strings"
	"unicode/utf8"
)

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

// textInput identifies the active field and inserts literal text without
// interpreting it as key bindings. The field identity also anchors async reads.
func (m *Model) textInput() (*string, func(string) tea.Cmd) {
	if m.Loading || m.Busy && (m.top() != pagePullRequestPicker || m.Session == nil) {
		return nil, nil
	}
	editor := func(body *string, cursor *int) (*string, func(string) tea.Cmd) {
		return body, func(text string) tea.Cmd {
			if len(*body)+len(text) <= 65536 {
				*body, *cursor = insertEditorText(*body, *cursor, text)
				m.editorCursorVisible = true
				if m.Composer != nil && m.Composer.CommitSHA != "" {
					m.ensureCommitEditorVisible()
				}
			}
			return nil
		}
	}
	singleLine := func(body *string, limit int, valid func(string) bool, changed func()) (*string, func(string) tea.Cmd) {
		return body, func(text string) tea.Cmd {
			if len(*body)+len(text) <= limit && !strings.ContainsAny(text, "\r\n") && valid(text) {
				*body += text
				if changed != nil {
					changed()
				}
			}
			return nil
		}
	}
	switch m.top() {
	case pagePullRequestPicker:
		if m.Session != nil {
			return singleLine(&m.SwitcherQuery, 1024, printableASCII, func() { m.PullRequestPicker.Index = 0; m.PullRequestPicker.previewScroll = 0 })
		}
	case pageInboxFilters:
		if m.inbox.field == 1 {
			return singleLine(&m.inbox.draftOptions.Repository, 128, printableASCII, nil)
		}
		if m.inbox.field == 2 {
			return singleLine(&m.inbox.draftOptions.Author, 128, printableASCII, nil)
		}
	case pageGuideConsent:
		if m.guideFocus == 2 && m.guideChoice.Provider != "" {
			return singleLine(&m.guideChoice.Model, 1024, guideconfig.ValidModel, func() { m.guideModelEditing = true })
		}
	case pageReviewSubmit:
		if f := m.ReviewForm; f != nil && f.Focus == 1 && !f.Confirm && m.draft.attempt == "" {
			return editor(&f.Body, &f.Cursor)
		}
	case pageReview, pageDiscussions:
		if e := m.discussions.published; e != nil {
			if !e.posting && e.action.Resolve == nil {
				return editor(&e.draft, &e.cursor)
			}
			return nil, nil
		}
		if c := m.Composer; c != nil {
			if m.draft.attempt == "" {
				return editor(&c.Draft, &c.Cursor)
			}
			return nil, nil
		}
		if c := m.CommentMenu; c != nil {
			if c.mode == commentActionReply && m.draft.attempt == "" {
				return editor(&c.Draft, &c.Cursor)
			}
			return nil, nil
		}
		if e := m.discussions.editor; e != nil && (m.top() == pageDiscussions || m.selectedReviewView() == viewDescription && m.discussions.overviewFocus) {
			if !e.posting {
				return &e.draft, func(text string) tea.Cmd { m.insertGeneralCommentText(text); return nil }
			}
			return nil, nil
		}
		if m.searchOpen() && m.searchState().editing {
			return &m.searchState().query, m.insertSearchText
		}
		if m.top() == pageReview && m.selectedReviewView() == viewFiles && m.fileView() && m.fileFilterEditing && !m.commitFilter.open && !m.commitFilter.subset && m.discussions.editor == nil {
			return &m.fileFilter, func(text string) tea.Cmd {
				if !strings.ContainsAny(text, "\r\n") && len(m.fileFilter)+len(text) <= 1024 {
					m.fileFilter += text
					m.selectFilteredFile()
				}
				return m.restartGuidePathScroll()
			}
		}
	}
	return nil, nil
}

func printableASCII(text string) bool {
	for _, r := range text {
		if r < ' ' || r > '~' {
			return false
		}
	}
	return true
}

func (m *Model) pasteText(text string) tea.Cmd {
	if text == "" || !utf8.ValidString(text) {
		return nil
	}
	_, insert := m.textInput()
	if insert == nil {
		return nil
	}
	return insert(text)
}
