package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/source"
)

// Negative keys keep issue-comment reactions separate from review-comment IDs.
func discussionReactionID(t source.Discussion) int64 {
	if len(t.Comments) == 0 || t.Retained || t.Comments[0].Retained || t.Kind == "Review" {
		return 0
	}
	if t.Kind == "PR comment" {
		id, _ := strconv.ParseInt(strings.TrimPrefix(t.ID, "PR comment:"), 10, 64)
		if id > 0 {
			return -id
		}
		return 0
	}
	return t.Comments[0].ID
}

func (m *Model) commentFooterControls(id int64, reply bool) []pageMouseControl {
	var controls []pageMouseControl
	if id != 0 && m.submitCommentAction != nil {
		counts := m.commentReactionCounts(id)
		for _, content := range []string{"+1", "-1", "heart"} {
			controls = append(controls, pageMouseControl{fmt.Sprintf("[%s %d]", m.reactionLabel(content), counts[content]), "react:" + content})
		}
	}
	if reply {
		controls = append(controls, pageMouseControl{"[↩ Reply]", "r"})
	}
	return controls
}
func (m *Model) discussionFooterControls(t source.Discussion) []pageMouseControl {
	id := discussionReactionID(t)
	if id == 0 || !m.discussions.snapshot.CurrentVerified || id > 0 && m.discussionFilesReason(t) != "" {
		return nil
	}
	reply := id < 0 && m.submitGeneralComment != nil || id > 0 && m.submitCommentAction != nil && m.discussionFilesReason(t) == ""
	return m.commentFooterControls(id, reply)
}
func (m *Model) discussionFooterClick(t source.Discussion, key string) tea.Cmd {
	if key == "r" {
		if m.top() == pageReview {
			cmd, _ := m.overviewKey("r")
			return cmd
		}
		if t.Kind == "PR comment" {
			return m.discussionKey("r")
		}
		m.pop()
		m.openOverviewDiscussions()
		m.openOverviewReply(t)
		return nil
	}
	id := discussionReactionID(t)
	if id == 0 || !strings.HasPrefix(key, "react:") || m.submitCommentAction == nil {
		return nil
	}
	menu := &commentActionMenu{CommentID: id, mode: commentActionReact}
	if id < 0 {
		menu.CommentID, menu.General = -id, true
	} else {
		menu.Target, menu.Author = t.Comments[0].Target, t.Comments[0].Author
	}
	m.CommentMenu = menu
	return m.footerReaction(strings.TrimPrefix(key, "react:"))
}
func (m *Model) footerReaction(content string) tea.Cmd {
	for i, value := range source.ReviewCommentReactions() {
		if content == value {
			return m.commentActionKey(tea.KeyPressMsg{Code: rune('1' + i)})
		}
	}
	return nil
}

func (m *Model) selectedCommentFooter(line diffLine) string {
	for _, c := range m.Comments {
		if c.ID != line.commentID || c.HistoricalProjection || c.Retained {
			continue
		}
		controls := m.commentFooterControls(c.ID, m.submitCommentAction != nil)
		if len(controls) == 0 {
			return line.Text
		}
		prefix := line.Text[:len(line.Text)-len(strings.TrimLeft(line.Text, " "))]
		width := visibleWidth(line.Text) - visibleWidth(prefix) - 2
		text := clip(pageMouseFooter(controls, " "), width)
		return prefix + "+" + text + strings.Repeat("-", max(0, width-visibleWidth(text))) + "+"
	}
	return line.Text
}

func (m *Model) diffFooterClick(line diffLine, x int) (tea.Cmd, bool) {
	if !line.commentFooter || m.CommentMenu != nil {
		return nil, false
	}
	_, selected := m.cursorAnchor()
	if selected != line.commentID {
		return nil, false
	}
	text := m.selectedCommentFooter(line)
	for _, c := range m.Comments {
		if c.ID != line.commentID || c.HistoricalProjection || c.Retained {
			continue
		}
		for _, control := range m.commentFooterControls(c.ID, m.submitCommentAction != nil) {
			at := strings.Index(text, control.label)
			if at < 0 {
				continue
			}
			left := visibleWidth(text[:at])
			pan := 0
			if !m.sideBySideEnabled() {
				pan = m.Horizontal
			}
			available := m.detailWidth() - visibleWidth(m.lineNumberPrefix(line))
			if m.cursorActive {
				available -= 2
			}
			if left < pan || left+visibleWidth(control.label) > pan+available {
				continue
			}
			if x < left || x >= left+visibleWidth(control.label) {
				continue
			}
			if !m.openCommentActionMenu() {
				return nil, true
			}
			if control.key == "r" {
				return m.commentActionKey(tea.KeyPressMsg{Code: 'r'}), true
			}
			m.CommentMenu.mode = commentActionReact
			return m.footerReaction(strings.TrimPrefix(control.key, "react:")), true
		}
	}
	return nil, false
}

func (m *Model) commentReactionCounts(id int64) map[string]int {
	counts := map[string]int{}
	addBase := func(base source.ReactionCounts) {
		for i, token := range source.ReviewCommentReactions() {
			counts[token] = base[i]
		}
	}
	if id < 0 {
		for _, event := range m.discussions.snapshot.Snapshot.Events {
			if event.ID == fmt.Sprintf("PR comment:%d", -id) {
				addBase(event.ReactionCounts)
				break
			}
		}
	} else {
		for _, c := range m.Comments {
			if c.ID == id {
				addBase(c.ReactionCounts)
				break
			}
		}
	}
	for _, reaction := range m.CommentReactions[id] {
		counts[reaction.Content]++
	}
	return counts
}

// Keep each chip intact when a narrow conversation card needs multiple rows.
func commentFooterRows(controls []pageMouseControl, width int) []string {
	var rows []string
	row := ""
	for _, control := range controls {
		if row != "" && visibleWidth(row)+1+visibleWidth(control.label) > width {
			rows = append(rows, row)
			row = ""
		}
		if row != "" {
			row += " "
		}
		row += clip(control.label, width)
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}
