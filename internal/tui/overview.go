package tui

import (
	"fmt"
	"maps"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
	"prui/internal/theme"
)

// Rows carry semantic activity/action identities so wrapping never changes a
// mouse target or the activity selected by keyboard navigation.
type overviewRow struct{ text, id, action string }
type discussionFilesReturn struct {
	selected, row, horizontal int
	focus                     pane
	filter                    string
	filterEditing             bool
	navigation                codeNavigation
	subset                    bool
	inventory                 bool
	scroll, cursor            map[int]int
	targets                   map[int]source.ReviewCommentTarget
	cursorActive              bool
}

func (m *Model) overviewHeight() int { return max(1, m.Height-3-m.reviewFooterRows()) }
func (m *Model) overviewRows() []overviewRow {
	var rows []overviewRow
	appendText := func(text, id, action string) {
		for _, line := range strings.Split(ansi.Wrap(text, max(1, m.Width-4), ""), "\n") {
			rows = append(rows, overviewRow{line, id, action})
		}
	}
	appendText("Description · captured when review opened", "", "")
	if branch := m.Session.Inventory.Comparison.Metadata.TargetBranch; branch != "" {
		appendText("Target branch: "+Escape(branch), "", "")
	}
	for _, line := range m.descriptionLines() {
		appendText(line, "", "")
	}
	appendText("", "", "")
	appendText("Discussions", "", "section")
	d := &m.discussions
	if d.notice != "" {
		appendText(d.notice, "", "")
	}
	if !d.loaded {
		label := "Discussions unavailable · c: refresh"
		if m.readDiscussions == nil {
			label = "Discussions unavailable · offline or unsupported"
		}
		appendText(label, "", "")
		if m.discussions.editor != nil {
			for _, line := range m.overviewComposerLines() {
				appendText(line, "composer", "editor")
			}
		}
		return m.overviewCards(rows)
	}
	if !d.snapshot.CurrentVerified {
		appendText("Discussions from live PR · snapshot differs", "", "")
		appendText(Escape(d.snapshot.Reason), "", "")
	}
	if !d.snapshot.Snapshot.Complete {
		appendText("Loaded activity · partial/incomplete", "", "")
		appendText(Escape(d.snapshot.Snapshot.Reason), "", "")
	}
	entries := m.discussionEntries()
	if len(entries) == 0 {
		label := "No discussions."
		if !d.snapshot.Snapshot.Complete {
			label = "No activity loaded."
		}
		appendText(label, "", "")
	}
	for i, t := range entries {
		expanded := d.overviewExpanded[t.ID]
		marker := "  "
		if d.overviewFocus && i == d.selected {
			marker = "> "
		}
		author := "[deleted]"
		if len(t.Comments) > 0 && t.Comments[0].Author != "" {
			author = t.Comments[0].Author
		}
		status := strings.TrimSuffix(discussionStatus(t), " · ")
		if t.Resolved != nil && !*t.Resolved && t.Kind != "PR comment" && t.Kind != "Review" {
			status += " · Unresolved"
		}
		appendText(marker+"@"+Escape(author)+" · "+status, t.ID, "detail")
		if expanded {
			actions := m.overviewDiscussionActions(t)
			if len(actions) > 0 {
				appendText("Actions · "+strings.Join(actions, " · "), t.ID, "detail")
			}
		}
		for _, line := range m.discussionCodeContext(t) {
			appendText(line, t.ID, "detail")
		}
		comments := t.Comments
		if expanded { // A reply's detail includes its complete loaded thread.
			for _, thread := range d.snapshot.Snapshot.Threads {
				for _, c := range thread.Comments {
					if len(t.Comments) > 0 && c.ID != 0 && c.ID == t.Comments[0].ID {
						comments = thread.Comments
						break
					}
				}
			}
		}
		for _, c := range comments {
			if expanded || len(comments) > 1 {
				prefix := "  "
				if len(t.Comments) > 0 && c.ID == t.Comments[0].ID {
					prefix = "> "
				}
				appendText(prefix+"@"+Escape(c.Author), t.ID, "detail")
			}
			for _, line := range m.conversationLines(c.Body, expanded) {
				appendText(line, t.ID, "detail")
			}
		}
		if t.Kind != "PR comment" && t.Kind != "Review" {
			reason := m.discussionFilesReason(t)
			if reason == "" {
				appendText("Open in Files [f]", t.ID, "files")
			} else {
				appendText("Open in Files unavailable · "+reason, t.ID, "detail")
			}
			if m.overviewOriginalAvailable(t) {
				appendText("View original commit [o]", t.ID, "original")
			}
		}
		if expanded && t.URL != "" {
			appendText(Escape(t.URL), t.ID, "detail")
		}
		if m.discussions.editor != nil && m.discussions.editor.replyTo != "" && t.ID == m.discussions.editor.replyTo || m.CommentMenu != nil && m.CommentMenu.mode == commentActionReply && len(t.Comments) > 0 && t.Comments[0].ID == m.CommentMenu.CommentID {
			for _, line := range m.overviewComposerLines() {
				appendText(line, t.ID, "editor")
			}
		}
		appendText("", t.ID, "detail")
	}
	if m.discussions.editor != nil && (m.discussions.editor.replyTo == "" || len(entries) == 0) {
		for _, line := range m.overviewComposerLines() {
			appendText(line, "composer", "editor")
		}
	}
	return m.overviewCards(rows)
}

// Decorate physical rows without losing the identities used by mouse actions.
func (m *Model) overviewCards(rows []overviewRow) []overviewRow {
	if m.Width < 6 {
		return rows
	}
	var out []overviewRow
	width := m.Width - 4
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].id == rows[start].id && rows[end].action != "section" {
			end++
		}
		// The section heading and status notices separate the description from activity.
		if rows[start].id == "" && rows[start].action == "section" {
			out = append(out, overviewRow{text: strings.Repeat("━", m.Width)}, rows[start])
			start++
			continue
		}
		selected := rows[start].id != "" && m.discussions.overviewFocus && rows[start].id == m.discussions.selectedID
		border, _ := m.theme.Color(theme.Border)
		bg := lipgloss.Color("#161e29")
		if m.theme.Name == theme.Light {
			bg = lipgloss.Color("#eef2f6")
		}
		if selected {
			border, _ = m.theme.Color(theme.FocusedBorder)
		}
		style := lipgloss.NewStyle().Background(bg)
		if selected {
			selection, _ := m.theme.Color(theme.Selection)
			bg = selection
			style = style.Background(selection)
		}
		edge := lipgloss.NewStyle().Foreground(border)
		if m.colorProfile <= colorprofile.Ascii {
			style = lipgloss.NewStyle()
			edge = lipgloss.NewStyle()
		}
		top, bottom := "┌", "└"
		label := strings.Repeat("─", m.Width-2)
		if selected && m.Width >= 12 {
			label = "─ Selected " + strings.Repeat("─", max(0, m.Width-13))
		}
		id := rows[start].id
		out = append(out, overviewRow{edge.Render(top + label + "┐"), id, "detail"})
		for _, row := range rows[start:end] {
			text := ansi.Truncate(row.text, width, "")
			text += strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
			body := style.Render(" " + text + " ")
			if m.colorProfile > colorprofile.Ascii {
				// Markdown resets and explicit code backgrounds must inherit the card surface.
				cells := themeCanvasBuffer(body, m.Width-2, 1)
				for x := 0; x < m.Width-2; x++ {
					cell := cells.CellAt(x, 0)
					if cell.Width > 0 {
						cell.Style.Bg = bg
					}
				}
				body = cells.Render()
			}
			row.text = edge.Render("│") + body + edge.Render("│")
			out = append(out, row)
		}
		out = append(out, overviewRow{edge.Render(bottom + strings.Repeat("─", m.Width-2) + "┘"), id, "detail"}, overviewRow{})
		start = end
	}
	return out
}

// Advertise only actions supported by the loaded item and current capabilities.
func (m *Model) overviewDiscussionActions(t source.Discussion) []string {
	var actions []string
	general := t.Kind == "PR comment" || t.Kind == "Review"
	if !general && m.discussionFilesReason(t) == "" {
		if m.submitCommentAction != nil {
			actions = append(actions, "r: reply")
		}
		actions = append(actions, "f: Files")
	}
	if !general && m.overviewOriginalAvailable(t) {
		actions = append(actions, "o: original")
	}
	if t.Kind == "PR comment" && m.submitGeneralComment != nil {
		actions = append(actions, "r: reply")
	}
	if m.submitPublished != nil && !t.Retained && len(t.Comments) > 0 && !t.Comments[0].Retained {
		c := t.Comments[0]
		if t.Kind != "Review" && c.Author != "" && (t.Kind == "PR comment" || c.ID > 0) {
			switch m.Viewer {
			case c.Author:
				actions = append(actions, "e: edit")
			case "":
				actions = append(actions, "e: check edit access")
			}
		}
		if !general && t.Resolved != nil {
			if *t.Resolved && t.CanUnresolve != nil && *t.CanUnresolve {
				actions = append(actions, "z: reopen")
			}
			if !*t.Resolved && t.CanResolve != nil && *t.CanResolve {
				actions = append(actions, "z: resolve")
			}
		}
	}
	if m.submitGeneralComment != nil {
		actions = append(actions, "n: new PR comment")
	}
	return actions
}

func (m *Model) overviewComposerLines() []string {
	if m.discussions.editor != nil {
		return strings.Split(m.generalCommentView(), "\n")
	}
	var lines []string
	lines = append(lines, "Reply to code thread · posts immediately")
	for _, line := range m.inlineReplyEditorLines(0) {
		lines = append(lines, line.Text)
	}
	return append(lines, "Enter: post · shift+enter: newline · ctrl+r: refresh · esc: cancel")
}
func (m *Model) ensureOverviewEditorVisibleRows(rows []overviewRow) {
	start, end := -1, -1
	for i, row := range rows {
		if row.action == "editor" {
			if start < 0 {
				start = i
			}
			end = i + 1
		}
	}
	if start < 0 {
		return
	}
	height := m.overviewHeight()
	if start < m.DescriptionScroll {
		m.DescriptionScroll = start
	}
	if end > m.DescriptionScroll+height {
		m.DescriptionScroll = max(0, end-height)
	}
}
func (m *Model) openOverviewReply(t source.Discussion) {
	if m.submitCommentAction == nil || m.discussionFilesReason(t) != "" {
		m.discussions.notice = "Thread reply unavailable · open captured current context first"
		return
	}
	comment := t.Comments[0]
	for _, current := range m.Comments {
		if current.ID == comment.ID {
			menu := &commentActionMenu{CommentID: current.ID, ReplyToID: m.topLevelCommentID(current.ID), Target: current.Target, Author: current.Author, mode: commentActionReply}
			if menu.ReplyToID <= 0 || !m.resolveReplyRoot(menu) {
				m.discussions.notice = "Thread reply root unavailable"
				return
			}
			m.CommentMenu = menu
			m.discussions.detail = false
			m.editorCursorVisible = true
			return
		}
	}
}

func (m *Model) overviewView() string {
	rows := m.overviewRows()
	height := m.overviewHeight()
	if m.discussions.editor != nil || m.CommentMenu != nil && m.CommentMenu.mode == commentActionReply {
		m.ensureOverviewEditorVisibleRows(rows)
	}
	m.DescriptionScroll = max(0, min(m.DescriptionScroll, max(0, len(rows)-height)))
	lines := []string{m.styleLine(classTitle, "Overview · Discussions [D]")}
	for i, row := range rows[m.DescriptionScroll:min(len(rows), m.DescriptionScroll+height)] {
		text := row.text
		if m.discussions.editor == nil && m.CommentMenu == nil && m.discussions.overviewCursorActive && m.DescriptionScroll+i == m.discussions.overviewCursor {
			text = "▸" + ansi.Cut(text, 1, m.Width)
		}
		lines = append(lines, text)
	}
	for len(lines) < height+1 {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
func (m *Model) revealOverviewActivity() {
	rows := m.overviewRows()
	for i, row := range rows {
		if row.id == m.discussions.selectedID && row.id != "" || row.action == "section" && m.discussions.selectedID == "" {
			m.DescriptionScroll = i
			m.discussions.overviewCursor = i
			m.discussions.overviewCursorActive = true
			return
		}
	}
}
func (m *Model) openOverviewDiscussions() {
	already := m.selectedReviewView() == viewDescription && m.discussions.overviewFocus
	m.selectReviewView(viewDescription)
	m.discussions.overviewFocus = true
	entries := m.discussionEntries()
	if len(entries) > 0 {
		m.discussions.selected = max(0, min(m.discussions.selected, len(entries)-1))
		m.discussions.selectedID = entries[m.discussions.selected].ID
	}
	if !already {
		m.revealOverviewActivity()
	}
}
func (m *Model) toggleOverviewExpansion() {
	d := &m.discussions
	if d.overviewExpanded == nil {
		d.overviewExpanded = map[string]bool{}
	}
	d.overviewExpanded[d.selectedID] = !d.overviewExpanded[d.selectedID]
	// Collapsing from deep inside a card keeps the cursor attached to that card.
	rows := m.overviewRows()
	first, last := -1, -1
	for i, row := range rows {
		if row.id == d.selectedID {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first >= 0 {
		d.overviewCursor = max(first, min(d.overviewCursor, last))
		if d.overviewCursor < m.DescriptionScroll {
			m.DescriptionScroll = d.overviewCursor
		}
	}
}

func (m *Model) overviewKey(key string) (tea.Cmd, bool) {
	d := &m.discussions
	switch key {
	case "y":
		if d.overviewFocus {
			return m.copyDiscussionURL(), true
		}
	case "tab", "shift+tab":
		entries := m.discussionEntries()
		delta := 1
		if key == "shift+tab" {
			delta = -1
		}
		if len(entries) > 0 {
			if !d.overviewFocus {
				d.selected = 0
			} else {
				d.selected = (d.selected + delta + len(entries)) % len(entries)
			}
			d.overviewFocus = true
			d.selectedID = entries[d.selected].ID
			d.detail = false
			m.revealOverviewActivity()
		}
		return nil, true
	case "enter":
		if d.overviewFocus && len(m.discussionEntries()) > 0 {
			m.toggleOverviewExpansion()
		}
		return nil, true
	case "esc":
		if d.overviewFocus && d.overviewExpanded[d.selectedID] {
			m.toggleOverviewExpansion()
		} else {
			d.overviewFocus = false
		}
		return nil, true
	case "f":
		if d.overviewFocus {
			m.openDiscussionFiles()
			return nil, true
		}
	case "o":
		if d.overviewFocus {
			entries := m.discussionEntries()
			if len(entries) > 0 {
				t := entries[d.selected]
				saved := m.commit
				saved.offsets = maps.Clone(saved.offsets)
				saved.cursors = maps.Clone(saved.cursors)
				if m.viewOriginalDiscussion(t.OriginalAnchor, t.OriginalCommitID) {
					d.returnCommit = &saved
					d.returnView = viewDescription
					d.returnOverview = true
				}
			}
			return nil, true
		}
	case "n", "r", "e", "z":
		if d.overviewFocus || key == "n" {
			// Reuse submit handlers while composing directly beside the selected activity.
			if key == "r" && len(m.discussionEntries()) > 0 {
				t := m.discussionEntries()[d.selected]
				if t.Kind != "PR comment" && t.Kind != "Review" {
					m.openOverviewReply(t)
					return nil, true
				}
			}
			wasDetail := d.detail
			d.detail = true
			cmd := m.discussionKey(key)
			d.detail = wasDetail
			if d.editor != nil {
				d.detail = false
				d.overviewFocus = true
				if key == "n" {
					m.DescriptionScroll = max(0, len(m.overviewRows())-m.overviewHeight())
				}
			}
			return cmd, true
		}
	}
	var delta int
	switch key {
	case "j", "down", "n":
		delta = 1
	case "k", "up", "p":
		delta = -1
	case "J", "shift+j":
		delta = diffStep
	case "K", "shift+k":
		delta = -diffStep
	case "d", "pgdown":
		m.pageOverviewCursor(m.overviewHeight())
		return nil, true
	case "u", "pgup":
		m.pageOverviewCursor(-m.overviewHeight())
		return nil, true
	case "home":
		m.moveOverviewCursor(-len(m.overviewRows()))
		return nil, true
	case "end":
		m.moveOverviewCursor(len(m.overviewRows()))
		return nil, true
	default:
		return nil, false
	}
	m.moveOverviewCursor(delta)
	return nil, true
}

// Page navigation moves the viewport and cursor by the same clamped amount.
func (m *Model) pageOverviewCursor(delta int) {
	rows := m.overviewRows()
	d := &m.discussions
	if !d.overviewCursorActive {
		d.overviewCursor = m.DescriptionScroll
		d.overviewCursorActive = true
	}
	previous := m.DescriptionScroll
	m.DescriptionScroll = max(0, min(previous+delta, max(0, len(rows)-m.overviewHeight())))
	d.overviewCursor = max(0, min(d.overviewCursor+m.DescriptionScroll-previous, len(rows)-1))
	m.selectOverviewCursor(rows)
}

func (m *Model) moveOverviewCursor(delta int) {
	d := &m.discussions
	rows := m.overviewRows()
	if !d.overviewCursorActive {
		d.overviewCursor = m.DescriptionScroll
		d.overviewCursorActive = true
	}
	d.overviewCursor = max(0, min(d.overviewCursor+delta, len(rows)-1))
	// Scroll when row navigation reaches the viewport edge.
	height := m.overviewHeight()
	if d.overviewCursor >= m.DescriptionScroll+height-1 {
		m.DescriptionScroll = d.overviewCursor - height + 2
	}
	if d.overviewCursor < m.DescriptionScroll {
		m.DescriptionScroll = d.overviewCursor
	}
	m.selectOverviewCursor(rows)
}

func (m *Model) selectOverviewCursor(rows []overviewRow) {
	d := &m.discussions
	d.overviewFocus = false
	if len(rows) > 0 {
		for i, entry := range m.discussionEntries() {
			if entry.ID == rows[d.overviewCursor].id {
				d.selected, d.selectedID, d.overviewFocus = i, entry.ID, true
				break
			}
		}
	}
}

func (m *Model) overviewMouseClick(y int) {
	if y == 2 {
		m.openOverviewDiscussions()
		return
	}
	rows := m.overviewRows()
	i := m.DescriptionScroll + y - 3
	if y < 3 || y >= 3+m.overviewHeight() || i < 0 || i >= len(rows) {
		return
	}
	m.discussions.overviewCursor, m.discussions.overviewCursorActive = i, true
	row := rows[i]
	if row.action == "section" {
		m.openOverviewDiscussions()
		return
	}
	m.selectOverviewCursor(rows)
	switch row.action {
	case "files":
		m.openDiscussionFiles()
	case "original":
		m.overviewKey("o")
	case "detail":
		m.overviewKey("enter")
	}
}

func (m *Model) currentDiscussionTarget(t source.Discussion) *source.ReviewCommentTarget {
	if !m.discussions.snapshot.CurrentVerified || t.Retained || t.CurrentAnchor == nil || t.Outdated != nil && *t.Outdated {
		return nil
	}
	target := *t.CurrentAnchor
	target.CommitID = m.Session.Inventory.Comparison.Metadata.HeadSHA
	if target.Identity != m.Session.Inventory.Comparison.Metadata.Identity {
		return nil
	}
	return &target
}
func (m *Model) discussionFilesReason(t source.Discussion) string {
	if !m.discussions.snapshot.CurrentVerified {
		return "Comparison differs"
	}
	if t.Retained {
		return "Stale retained activity"
	}
	if t.Outdated != nil && *t.Outdated {
		return "Outdated; current Files location unavailable"
	}
	target := m.currentDiscussionTarget(t)
	if target == nil {
		return "Current Files location unavailable"
	}
	if len(t.Comments) == 0 || t.Comments[0].ID == 0 {
		return "Comment unavailable"
	}
	for _, c := range m.Comments {
		if c.ID == t.Comments[0].ID && c.Target == *target {
			return ""
		}
	}
	return "Comment not captured in Files"
}
func (m *Model) discussionCodeContext(t source.Discussion) []string {
	if t.Kind == "PR comment" || t.Kind == "Review" {
		return nil
	}
	contexts := m.overviewCodeContexts()
	target := m.currentDiscussionTarget(t)
	ctx := contexts.current
	label := "captured PR diff"
	at, found := ctx.locate(target)
	if !found {
		target = t.OriginalAnchor
		label = "captured original commit"
		ctx = m.overviewOriginalContext(contexts, t.OriginalCommitID)
		at, found = ctx.locate(target)
	}
	if found && target.SubjectType == "file" {
		return []string{Escape(target.Path) + " · File comment · " + label}
	}
	if found {
		out := []string{Escape(target.Path) + " · " + label}
		for j := max(0, at-3); j < min(len(ctx.rows), at+4); j++ {
			r := ctx.rows[j]
			sameFile := false
			for _, candidate := range sourceLineTargets(r) {
				if candidate.Path == target.Path {
					sameFile = true
					break
				}
			}
			if !sameFile {
				continue
			}
			marker := "  "
			if discussionRowInRange(r, *target) {
				marker = "> "
			}
			out = append(out, fmt.Sprintf("%s%s %s %s", marker, overviewLineNumber(r.oldLine), overviewLineNumber(r.newLine), m.sourceText(r, 0, max(1, m.Width-12), "", theme.Foreground)))
		}
		return out
	}

	if t.DiffHunk != "" {
		label := "Original commit unavailable"
		if t.OriginalCommitID != "" {
			label = "Original commit not captured"
			for _, e := range m.commitEntries() {
				if e.SHA == t.OriginalCommitID {
					label = "Original context unavailable"
					break
				}
			}
		}
		return []string{label, "Historical snippet", Escape(t.DiffHunk)}
	}
	return []string{"Code context unavailable"}
}
func (m *Model) openDiscussionFiles() {
	entries := m.discussionEntries()
	if len(entries) == 0 {
		return
	}
	t := entries[m.discussions.selected]
	if reason := m.discussionFilesReason(t); reason != "" {
		m.discussions.notice = reason
		return
	}
	saved := &discussionFilesReturn{selected: m.Selected, row: m.Row, horizontal: m.Horizontal, focus: m.Focus, filter: m.fileFilter, filterEditing: m.fileFilterEditing, navigation: m.navigation, subset: m.commitFilter.subset, inventory: m.Inventory, scroll: maps.Clone(m.Scroll), cursor: maps.Clone(m.Cursor), targets: maps.Clone(m.CursorTarget), cursorActive: m.cursorActive}
	m.selectReviewView(viewFiles)
	m.commitFilter.subset = false
	m.fileFilter = ""
	m.fileFilterEditing = false
	m.navigation = codeNavigation{}
	m.Focus = paneDiff
	target := m.currentDiscussionTarget(t)
	for f, file := range m.Session.Inventory.Files {
		if target.Path == string(file.NewPath) || target.Path == string(file.OldPath) {
			m.selectFile(f)
			break
		}
	}
	// Full diff comment cards, including ranges and file comments, carry IDs.
	id := t.Comments[0].ID
	for i, line := range m.displayDetail() {
		if line.commentID == id {
			m.setCursor(i)
			m.ensureCursorVisible()
			m.discussions.returnFiles = saved
			return
		}
	}
	m.discussions.returnFiles = saved
	m.restoreDiscussionFiles()
	m.discussions.notice = "Comment not captured in Files"
}
func (m *Model) restoreDiscussionFiles() bool {
	saved := m.discussions.returnFiles
	if saved == nil {
		return false
	}
	m.Selected, m.Row, m.Horizontal = saved.selected, saved.row, saved.horizontal
	m.Focus, m.fileFilter, m.navigation = saved.focus, saved.filter, saved.navigation
	m.fileFilterEditing = saved.filterEditing
	m.Inventory, m.commitFilter.subset = saved.inventory, saved.subset
	m.Scroll, m.Cursor, m.CursorTarget = saved.scroll, saved.cursor, saved.targets
	m.cursorActive = saved.cursorActive
	m.discussions.returnFiles = nil
	m.ContextView = viewDescription
	m.discussions.overviewFocus = true
	return true
}

func discussionRowEndsAt(row diffLine, t source.ReviewCommentTarget) bool {
	t.StartLine, t.StartSide = 0, ""
	return diffLineHasTarget(row, t)
}
func discussionRowInRange(row diffLine, t source.ReviewCommentTarget) bool {
	if t.StartLine == 0 {
		return discussionRowEndsAt(row, t)
	}
	for _, candidate := range sourceLineTargets(row) {
		if candidate.Identity == t.Identity && candidate.CommitID == t.CommitID && candidate.Path == t.Path && candidate.Side == t.Side && t.StartSide == t.Side && candidate.Line >= t.StartLine && candidate.Line <= t.Line {
			return true
		}
	}
	return discussionRowEndsAt(row, t)
}

// File comments belong to the file header, never an invented source line.
func (m *Model) fileDiscussionLines(header diffLine) []diffLine {
	if header.Class != classFileHeader {
		return nil
	}
	var out []diffLine
	for _, file := range m.Session.Inventory.Files {
		if header.Text != fileDivider(file) {
			continue
		}
		for _, c := range m.Comments {
			if c.ParentID == 0 && c.Target.SubjectType == "file" && (c.Target.Path == string(file.NewPath) || c.Target.Path == string(file.OldPath)) {
				out = append(out, m.reviewCommentThread(c, 0)...)
			}
		}
		break
	}
	return out
}

func overviewLineNumber(n int) string {
	if n == 0 {
		return "    "
	}
	return fmt.Sprintf("%4d", n)
}

// Preserve the selected activity's relative viewport position as earlier cards
// wrap differently or new activity arrives above it.
func (m *Model) overviewPosition() (int, bool) {
	if m.top() != pageReview || m.selectedReviewView() != viewDescription || !m.discussions.overviewFocus || m.discussions.selectedID == "" {
		return 0, false
	}
	for i, row := range m.overviewRows() {
		if row.id == m.discussions.selectedID {
			return i - m.DescriptionScroll, true
		}
	}
	return 0, false
}
func (m *Model) restoreOverviewPosition(relative int) {
	rows := m.overviewRows()
	first, last := -1, -1
	for i, row := range rows {
		if row.id == m.discussions.selectedID {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	if m.discussions.overviewCursorActive {
		shift := first - (relative + m.DescriptionScroll)
		m.discussions.overviewCursor = max(first, min(m.discussions.overviewCursor+shift, last))
	}
	m.DescriptionScroll = max(0, first-relative)
}

// Context indexes retain immutable captured rows; scrolling and replies do not
// reparse every patch or scan the entire comparison for each activity card.
type discussionCodeRows struct {
	rows    []diffLine
	anchors map[source.ReviewCommentTarget]int
	files   map[string]bool
}
type overviewCodeContexts struct {
	session   *review.Session
	commits   *commits.Bundle
	current   *discussionCodeRows
	originals map[string]*discussionCodeRows
}

func indexDiscussionCode(rows []diffLine, files []inventory.FileChange) *discussionCodeRows {
	ctx := &discussionCodeRows{rows: rows, anchors: map[source.ReviewCommentTarget]int{}, files: map[string]bool{}}
	for i, row := range rows {
		for _, target := range sourceLineTargets(row) {
			ctx.anchors[target] = i
		}
	}
	for _, file := range files {
		if len(file.NewPath) > 0 {
			ctx.files[string(file.NewPath)] = true
		}
		if len(file.OldPath) > 0 {
			ctx.files[string(file.OldPath)] = true
		}
	}
	return ctx
}
func (ctx *discussionCodeRows) locate(target *source.ReviewCommentTarget) (int, bool) {
	if ctx == nil || target == nil {
		return 0, false
	}
	if target.SubjectType == "file" {
		return 0, ctx.files[target.Path]
	}
	endpoint := *target
	endpoint.StartLine, endpoint.StartSide = 0, ""
	at, ok := ctx.anchors[endpoint]
	return at, ok
}
func (m *Model) overviewCodeContexts() *overviewCodeContexts {
	c := &m.discussions.codeContexts
	if c.session != m.Session || c.commits != m.Session.Commits {
		*c = overviewCodeContexts{session: m.Session, commits: m.Session.Commits, originals: map[string]*discussionCodeRows{}}
	}
	if c.current == nil {
		c.current = indexDiscussionCode(m.cachedFileDetail(false), m.Session.Inventory.Files)
	}
	return c
}
func (m *Model) overviewOriginalContext(c *overviewCodeContexts, sha string) *discussionCodeRows {
	if ctx, ok := c.originals[sha]; ok {
		return ctx
	}
	var ctx *discussionCodeRows
	for _, entry := range m.commitEntries() {
		if entry.SHA == sha && entry.Status == commits.Captured && entry.Diff != nil {
			ctx = indexDiscussionCode(commitDiffRowsFor(entry.Diff, m.Session.Inventory.Comparison.Metadata.Identity, sha), entry.Diff.Files)
			break
		}
	}
	c.originals[sha] = ctx
	return ctx
}

func (m *Model) overviewOriginalAvailable(t source.Discussion) bool {
	if t.OriginalAnchor == nil || t.OriginalAnchor.SubjectType == "file" || t.OriginalAnchor.StartLine != 0 {
		return false
	}
	ctx := m.overviewOriginalContext(m.overviewCodeContexts(), t.OriginalCommitID)
	at, ok := ctx.locate(t.OriginalAnchor)
	return ok && ctx.rows[at].target != nil && *ctx.rows[at].target == *t.OriginalAnchor
}
