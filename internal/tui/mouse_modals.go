package tui

import (
	"image"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Modal input owns the whole viewport, including clicks outside its card. All
// writes go through the same validation and confirmation handlers as keys.
func (m *Model) modalMouseAvailable() bool {
	return m.Loading || m.Busy || m.Err != nil || m.Composer != nil || m.CommentMenu != nil || m.discussions.published != nil || m.discussions.editor != nil || m.top() == pageGuideConsent || m.top() == pageReviewSubmit || m.top() == pageQuitPending || m.top() == pageDraftRecovery || m.top() == pageSuggestionApply
}

func mouseKey(key string) tea.KeyPressMsg {
	switch key {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "shift+enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	}
	if strings.HasPrefix(key, "ctrl+") {
		return tea.KeyPressMsg{Code: []rune(strings.TrimPrefix(key, "ctrl+"))[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
}

// hintMouseKey is only given a trusted, structurally identified control row.
// Source, comment bodies, notices and errors are never searched for actions.
func hintMouseKey(line string, x int, width int, rowStarts ...int) string {
	minStart := 0
	if len(rowStarts) > 0 {
		minStart = rowStarts[0]
	}
	line = ansi.Strip(line)
	start := 0
	for _, part := range strings.Split(line, " · ") {
		end := start + visibleWidth(part)
		if start >= minStart && end <= width && x >= start && x < end {
			for _, key := range []string{"shift+enter", "ctrl+r", "ctrl+d", "ctrl+p", "ctrl+a", "enter", "esc", "tab", "s", "r", "a", "y", "z", "e", "d", "1", "2", "3", "4", "5", "6", "7", "8", "q"} {
				lower := strings.ToLower(part)
				prefix := strings.ToLower(key)
				if strings.HasPrefix(lower, prefix+":") || strings.HasPrefix(lower, prefix+" ") {
					return key
				}
				// The first comment action has a descriptive heading before its key.
				if lower == "comment actions: r reply" {
					return "r"
				}
				if lower == "delete this comment? enter confirm" {
					return "enter"
				}
				if strings.HasPrefix(lower, "react: "+prefix+" ") {
					return key
				}
				if lower == "enter/esc: continue" {
					return "enter"
				}
			}
		}
		start = end + 3
	}
	return ""
}

func actionBodyBounds(width, height int, body string) image.Rectangle {
	rows := len(strings.Split(body, "\n"))
	if width < 20 || height < 8 {
		return image.Rect(0, 0, width, min(rows, height))
	}
	inside := min(96, width-8)
	left, top := max(0, (width-inside-4)/2), max(0, (height-rows-2)/2)
	return image.Rect(left+2, top+1, left+2+inside, min(height, top+1+rows))
}

func (m *Model) mouseModal(msg tea.MouseMsg) (tea.Cmd, bool) {
	if !m.modalMouseAvailable() {
		return nil, false
	}
	m.cancelMouseDrag()
	m.textSelection = textSelection{}
	event := msg.Mouse()
	if event.Mod != 0 || event.X < 0 || event.Y < 0 || event.X >= m.Width || event.Y >= m.Height {
		return nil, true
	}
	click, clicked := msg.(tea.MouseClickMsg)
	clicked = clicked && click.Button == tea.MouseLeft
	wheel, scrolled := msg.(tea.MouseWheelMsg)
	delta := ""
	if scrolled {
		if wheel.Button == tea.MouseWheelUp {
			delta = "up"
		}
		if wheel.Button == tea.MouseWheelDown {
			delta = "down"
		}
	}
	if m.Loading || m.Busy {
		modal := m.loadingModal()
		if clicked && modal.cancelable && m.loadingCancelBounds(modal).Contains(event.X, event.Y) {
			m.cancelCurrentAction()
		}
		return nil, true
	}
	if m.Err != nil {
		if clicked && event.Y == 2 {
			if event.X >= visibleWidth("No complete comparison available. ") && event.X < visibleWidth("No complete comparison available. q: quit") && m.Width >= visibleWidth("No complete comparison available. q: quit") {
				m.cancel()
				return tea.Quit, true
			}
		}
		return nil, true
	}
	// Recovery and quit confirmations take precedence over recovered editors.
	if m.top() == pageQuitPending {
		body := "Discard unsent review drafts and quit?\n\ns: save and quit · enter: discard and quit · esc: keep reviewing"
		if m.ActionError != nil {
			body += "\n! " + Escape(m.ActionError.Error())
		}
		b := actionBodyBounds(m.Width, m.Height, body)
		if clicked && event.Y == b.Min.Y+2 && image.Pt(event.X, event.Y).In(b) {
			if k := hintMouseKey(strings.Split(body, "\n")[2], event.X-b.Min.X, b.Dx()); k != "" {
				return m.pageKey(pageQuitPending, mouseKey(k)), true
			}
		}
		return nil, true
	}
	if m.top() == pageDraftRecovery || m.top() == pageSuggestionApply {
		body := m.draftRecoveryView
		handler := m.draftRecoveryKey
		if m.top() == pageSuggestionApply {
			body = m.suggestionApplyView
			handler = m.suggestionApplyKey
		}
		content := body()
		lines := strings.Split(content, "\n")
		b := actionBodyBounds(m.Width, m.Height, content)
		if !image.Pt(event.X, event.Y).In(b) {
			return nil, true
		}
		if delta != "" {
			for i := 0; i < mouseWheelStep; i++ {
				handler(mouseKey(delta))
			}
			return nil, true
		}
		if clicked && event.Y == b.Min.Y+len(lines)-1 {
			if k := hintMouseKey(lines[len(lines)-1], event.X-b.Min.X, b.Dx()); k != "" {
				return handler(mouseKey(k)), true
			}
		}
		return nil, true
	}
	if m.discussions.published != nil {
		lines := strings.Split(m.publishedView(), "\n")
		if clicked && event.Y == len(lines)-1 {
			if k := hintMouseKey(lines[len(lines)-1], event.X, m.Width); k != "" {
				return m.publishedKey(mouseKey(k)), true
			}
		}
		if clicked && m.discussions.published.action.Resolve == nil {
			e := m.discussions.published
			m.fullEditorMouse(&e.draft, &e.cursor, event.X, event.Y)
		}
		if delta != "" {
			m.moveEditorCursor(&m.discussions.published.draft, &m.discussions.published.cursor, delta)
		}
		return nil, true
	}
	if m.top() == pageGuideConsent {
		return m.guideConsentMouse(msg), true
	}
	if m.top() == pageReviewSubmit {
		return m.reviewFormMouse(msg), true
	}
	var handler func(tea.KeyPressMsg) tea.Cmd
	switch {
	case m.Composer != nil:
		handler = m.commentComposerKey
	case m.CommentMenu != nil:
		handler = m.commentActionKey
	case m.discussions.editor != nil:
		handler = m.generalCommentKey
	}
	if handler == nil {
		return nil, true
	}
	if m.discussions.editor != nil && m.top() == pageDiscussions {
		lines := strings.Split(m.generalCommentView(), "\n")
		if clicked && event.Y == len(lines)-1 {
			if k := hintMouseKey("Enter: post · shift+enter: newline · ctrl+r: refresh · esc: discard", event.X, m.Width); k != "" {
				return handler(mouseKey(k)), true
			}
		}
	} else {
		footerY := len(strings.Split(m.reviewView(), "\n")) - 1
		if clicked && event.Y == footerY {
			if k := hintMouseKey(m.modalReviewHints(), event.X, m.Width); k != "" {
				return handler(mouseKey(k)), true
			}
		}
	}
	if clicked {
		if m.top() == pageReview && m.selectedReviewView() == viewDescription {
			return m.overviewEditorMouse(event.X, event.Y, handler), true
		}
		if m.discussions.editor != nil && m.top() == pageDiscussions {
			e := m.discussions.editor
			m.fullEditorMouse(&e.draft, &e.cursor, event.X, event.Y)
		} else {
			m.inlineEditorMouse(event.X, event.Y)
		}
		if m.Composer != nil {
			m.composerTypeMouse(click.X, click.Y)
		}
	}
	if delta != "" {
		switch {
		case m.Composer != nil:
			m.moveEditorCursor(&m.Composer.Draft, &m.Composer.Cursor, delta)
			m.ensureInlineEditorVisible()
		case m.CommentMenu != nil && m.CommentMenu.mode == commentActionReply:
			m.moveEditorCursor(&m.CommentMenu.Draft, &m.CommentMenu.Cursor, delta)
			m.ensureReplyEditorVisible()
		case m.discussions.editor != nil:
			m.moveEditorCursor(&m.discussions.editor.draft, &m.discussions.editor.cursor, delta)
		}
	}
	return nil, true
}

// Cursor movement scrolls long editors through the existing caret viewport.
func (m *Model) moveEditorCursor(body *string, cursor *int, direction string) {
	runes := []rune(*body)
	*cursor = min(max(0, *cursor), len(runes))
	for step := 0; step < mouseWheelStep; step++ {
		start := *cursor
		for start > 0 && runes[start-1] != '\n' {
			start--
		}
		col := *cursor - start
		if direction == "up" {
			if start == 0 {
				*cursor = 0
				return
			}
			end := start - 1
			prior := end
			for prior > 0 && runes[prior-1] != '\n' {
				prior--
			}
			*cursor = min(prior+col, end)
		} else {
			end := *cursor
			for end < len(runes) && runes[end] != '\n' {
				end++
			}
			if end == len(runes) {
				*cursor = end
				return
			}
			next := end + 1
			end = next
			for end < len(runes) && runes[end] != '\n' {
				end++
			}
			*cursor = min(next+col, end)
		}
	}
}

func (m *Model) composerTypeMouse(x, y int) {
	if m.Composer.CommitSHA != "" || m.selectedReviewView() == viewCommits {
		return
	}
	g := m.workspaceGeometry()
	if !image.Pt(x, y).In(g.Detail) {
		return
	}
	rows := m.displayDetail()
	first := -1
	for i, row := range rows {
		if row.editor {
			first = i
			break
		}
	}
	if first < 0 || y-g.Detail.Min.Y+m.offset() != first {
		return
	}
	local := x - g.Detail.Min.X
	if m.cursorActive {
		local -= 2
	}
	if !m.sideBySideEnabled() {
		local += m.Horizontal
	}
	offset := 0
	current := commentType(m.Composer)
	for _, mode := range m.commentTypes() {
		label := mode
		if mode == current {
			label = "[" + mode + "]"
		}
		end := offset + visibleWidth(label)
		if local >= offset && local < end {
			for attempts := 0; attempts < 3 && commentType(m.Composer) != mode; attempts++ {
				m.cycleCommentType(false)
			}
			return
		}
		offset = end + 3
	}
}

// A small rectangle adapter keeps loading controls tied to their renderer's
// fixed card and notice wrapping rather than scanning arbitrary notice text.
type mouseControlBounds struct{ image.Rectangle }

func (r mouseControlBounds) Contains(x, y int) bool { return image.Pt(x, y).In(r.Rectangle) }
func (m *Model) loadingCancelBounds(modal loadingModal) mouseControlBounds {
	if m.Width < visibleWidth("esc: cancel") || m.Height <= 2 {
		return mouseControlBounds{}
	}
	if m.Width < 20 || m.Height <= 6 {
		return mouseControlBounds{image.Rect(0, 2, min(m.Width, visibleWidth("esc: cancel")), min(3, m.Height))}
	}
	width := min(60, max(36, m.Width-4))
	width = min(width, m.Width)
	inside := max(1, width-4)
	top := max(0, (m.Height-7)/2)
	notices := len(strings.Split(ansi.Wrap(Escape(modal.notice), max(1, inside-4), ""), "\n"))
	notices = min(notices, m.Height-top-6)
	left := max(0, (m.Width-width)/2) + 4
	row := top + 4 + notices
	return mouseControlBounds{image.Rect(left, row, min(m.Width, left+visibleWidth("esc: cancel")), min(m.Height, row+1))}
}

func (m *Model) guideConsentMouse(msg tea.MouseMsg) tea.Cmd {
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return nil
	}
	body := m.guideConsentView()
	paragraphs := strings.Split(body, "\n")
	inside := max(1, min(68, m.Width-6))
	rows := 2
	lengths := make([]int, len(paragraphs))
	for i, p := range paragraphs {
		lengths[i] = len(strings.Split(ansi.Wrap(p, inside, ""), "\n"))
		rows += lengths[i]
	}
	left, top := max(0, (m.Width-inside-4)/2)+2, max(0, (m.Height-min(rows, m.Height))/2)+1
	if m.Width < 20 || m.Height < 8 {
		left, top = 0, 0
		for i := range lengths {
			lengths[i] = 1
		}
	}
	if click.X < left || click.X >= min(m.Width, left+inside) {
		return nil
	}
	row := top
	for i, n := range lengths {
		if click.Y >= row && click.Y < row+n {
			switch i {
			case 2:
				m.guideFocus = 1
				return m.guideConsentKey("right")
			case 3:
				m.guideFocus = 2
				m.guideModelEditing = true
			case 9:
				m.guideFocus = 0
				return m.guideConsentKey("enter")
			case 10:
				if click.X-left < visibleWidth("Tab / ↑↓ move") {
					return m.guideConsentKey("tab")
				}
				return m.guideConsentKey("esc")
			case 11:
				if click.X-left < 2 {
					return m.guideConsentKey("left")
				}
				return m.guideConsentKey("right")
			}
		}
		row += n
	}
	return nil
}

func (m *Model) reviewFormMouse(msg tea.MouseMsg) tea.Cmd {
	if m.ReviewForm == nil || m.Session == nil {
		return nil
	}
	form := *m
	if m.Width >= 20 && m.Height >= 8 {
		form.Width = min(96, m.Width-8)
		form.Height = min(22, m.Height-2)
	}
	body := form.reviewFormView()
	lines := strings.Split(body, "\n")
	b := actionBodyBounds(m.Width, m.Height, body)
	event := msg.Mouse()
	if !image.Pt(event.X, event.Y).In(b) {
		return nil
	}
	f := m.ReviewForm
	if w, ok := msg.(tea.MouseWheelMsg); ok && !f.Confirm {
		direction := ""
		if w.Button == tea.MouseWheelUp {
			direction = "up"
		}
		if w.Button == tea.MouseWheelDown {
			direction = "down"
		}
		if direction != "" {
			return m.reviewFormKey(mouseKey(direction))
		}
		return nil
	}
	c, ok := msg.(tea.MouseClickMsg)
	if !ok || c.Button != tea.MouseLeft {
		return nil
	}
	row, x := c.Y-b.Min.Y, c.X-b.Min.X
	if row == len(lines)-1 {
		if k := hintMouseKey(reviewFormFooter(f), x, b.Dx()); k != "" {
			return m.reviewFormKey(mouseKey(k))
		}
		return nil
	}
	if f.Confirm {
		return nil
	}
	choices := 2
	if form.Height >= 9 {
		choices++
	}
	if form.Height >= 12 {
		choices++
	}
	if form.Height <= 8 {
		choices = 0
		if form.Height >= 7 {
			choices++
		}
		if form.Height >= 6 || form.Height == 5 && m.ActionError == nil {
			choices++
		}
	}
	if row >= choices && row < choices+len(reviewEvents) {
		f.Event = row - choices
		f.Focus = 0
		return nil
	}
	pendingTop, pendingStart, pendingCount := form.reviewPendingMouseRows()
	if row >= pendingTop && row < pendingTop+pendingCount {
		f.Focus = 2
		f.Selected = pendingStart + row - pendingTop
		return nil
	}
	if row >= choices+len(reviewEvents) {
		f.Focus = 1
		summaryTop := choices + len(reviewEvents)
		if form.Height >= 13 {
			summaryTop++
		}
		summaryTop++ // heading
		bordered := form.Height >= 14 && form.Width >= 20
		if bordered {
			summaryTop++
		}
		end := pendingTop - 1
		if bordered {
			end--
		}
		if row >= summaryTop && row < end {
			bodyRows := strings.Split(f.Body, "\n")
			before := string([]rune(f.Body)[:min(f.Cursor, len([]rune(f.Body)))])
			cursorLine := strings.Count(before, "\n")
			limit := end - summaryTop
			start := max(0, min(cursorLine-limit+1, len(bodyRows)-limit))
			if at, ok := editorCursorAt(f.Body, f.Cursor, 1<<20, true, true, x-4, row-summaryTop+start); ok {
				f.Cursor = at
			}
		}
	}
	return nil
}

// Reproduce the form's structural row allocation; pending bodies are never
// interpreted as control labels or used to locate their own hit regions.
func (m *Model) reviewPendingMouseRows() (top, start, count int) {
	f := m.ReviewForm
	if f == nil || f.Confirm || m.Height <= 8 {
		return 0, 0, 0
	}
	lines := 1
	if m.Height >= 9 {
		lines++
	}
	if m.Height >= 12 {
		lines++
	}
	lines += 1 + len(reviewEvents)
	if m.Height >= 13 {
		lines++
	}
	lines++ // summary heading
	reserved := 2
	if len(m.Pending) > 0 {
		reserved++
	}
	if !m.Session.Inventory.Complete {
		reserved++
	}
	if m.ActionError != nil {
		reserved += 2
	}
	bordered := m.Height >= 14 && m.Width >= 20
	if bordered {
		reserved += 2
	}
	bodyLimit := max(1, min(3, m.Height-lines-reserved))
	bodyLines := min(len(strings.Split(f.Body, "\n")), bodyLimit)
	if m.Height >= 14 {
		bodyLines = bodyLimit
	}
	lines += bodyLines
	if bordered {
		lines += 2
	}
	lines++ // pending heading
	top = lines
	reserved = 1
	if !m.Session.Inventory.Complete {
		reserved++
	}
	if m.ActionError != nil {
		reserved += 2
	}
	available := max(0, m.Height-lines-reserved)
	start = max(0, f.Selected-available+1)
	count = min(available, max(0, len(m.Pending)-start))
	return
}

func (m *Model) modalReviewHints() string {
	full := *m
	full.Width = max(m.Width, 4096)
	return full.reviewHints()
}

// Map the same escaped, wrapped text used by editors back to rune offsets.
// This maps text position only; it never treats body text as UI commands.
func editorCursorAt(body string, cursor, width int, hardwrap, multiline bool, x, row int) (int, bool) {
	if row < 0 || width < 1 {
		return 0, false
	}
	runes := []rune(body)
	cursor = min(max(cursor, 0), len(runes))
	var stream strings.Builder
	offsets := []int{}
	appendText := func(text string, offset int) {
		stream.WriteString(text)
		for range len(text) {
			offsets = append(offsets, offset)
		}
	}
	for i, r := range runes {
		if i == cursor {
			appendText("█", i)
		}
		if r == '\n' && multiline {
			appendText("\n", i)
		} else {
			appendText(Escape(string(r)), i)
		}
	}
	if cursor == len(runes) {
		appendText("█", cursor)
	}
	raw := stream.String()
	wrapped := ansi.Wrap(raw, width, "")
	if hardwrap {
		wrapped = ansi.Hardwrap(raw, width, true)
	}
	rows := strings.Split(wrapped, "\n")
	if row >= len(rows) {
		return 0, false
	}
	at := 0
	for n, line := range rows {
		rel := strings.Index(raw[at:], line)
		if rel < 0 {
			return 0, false
		}
		at += rel
		if n == row {
			col := 0
			for offset := 0; offset < len(line); {
				r, size := utf8.DecodeRuneInString(line[offset:])
				width := visibleWidth(string(r))
				if x < col+width {
					return offsets[min(at+offset, len(offsets)-1)], true
				}
				col += width
				offset += size
			}
			end := at + len(line)
			if end < len(offsets) {
				return offsets[end], true
			}
			return len(runes), true
		}
		at += len(line)
		if at < len(raw) && raw[at] == '\n' {
			at++
		}
	}
	return 0, false
}

func (m *Model) fullEditorMouse(body *string, cursor *int, x, y int) {
	if y < 2 {
		return
	}
	runes := []rune(*body)
	at := min(max(0, *cursor), len(runes))
	prefix := strings.Split(ansi.Wrap(Escape(string(runes[:at]))+"█", max(1, m.Width), ""), "\n")
	start := max(0, len(prefix)-max(1, m.Height-4))
	if next, ok := editorCursorAt(*body, *cursor, max(1, m.Width), false, false, x, y-2+start); ok {
		*cursor = next
	}
}

func (m *Model) inlineEditorMouse(x, y int) {
	var body *string
	var cursor *int
	indent := 0
	if m.Composer != nil {
		body = &m.Composer.Draft
		cursor = &m.Composer.Cursor
	} else if m.CommentMenu != nil && m.CommentMenu.mode == commentActionReply {
		body = &m.CommentMenu.Draft
		cursor = &m.CommentMenu.Cursor
		indent = 4
	}
	if body == nil {
		return
	}
	g := m.workspaceGeometry()
	rows := m.displayDetail()
	offset := m.offset()
	gutter := 0
	if m.cursorActive {
		gutter = 2
	}
	if m.selectedReviewView() == viewCommits {
		g = m.commitGeometry()
		rows = m.commitRows()
		offset = m.commitOffset()
		gutter = 2
	}
	if !image.Pt(x, y).In(g.Detail) {
		return
	}
	first := -1
	for i, row := range rows {
		if row.editor {
			if first < 0 {
				first = i
			}
		}
	}
	if first < 0 {
		return
	}
	if m.Composer == nil {
		indent = max(0, len(rows[first].Text)-len(strings.TrimLeft(rows[first].Text, " "))-2)
	}
	editor := m.inlineEditorLinesFor(*body, *cursor, indent)
	if m.Composer != nil {
		editor = m.inlineEditorLines()
	}
	// The body box is always the trailing editor, after selectors and previews.
	basic := m.inlineEditorLinesFor(*body, *cursor, indent)
	bodyTop := first + len(editor) - len(basic) + 1
	if m.Composer == nil {
		bodyTop = first + 1
	}
	logicalRow := y - g.Detail.Min.Y + offset - bodyTop
	if logicalRow < 0 || logicalRow >= len(basic)-2 {
		return
	}
	local := x - g.Detail.Min.X - gutter - (4 + indent)
	if !m.sideBySideEnabled() {
		local += m.Horizontal
	}
	if local < 0 || local >= m.overlayInnerWidth(indent) {
		return
	}
	if next, ok := editorCursorAt(*body, *cursor, m.overlayInnerWidth(indent), true, true, local, logicalRow); ok {
		*cursor = next
		m.editorCursorVisible = true
	}
}

// Locate editor lines from their semantic ownership, then mirror the overview's
// second wrapping pass. Neither surrounding activity nor draft text is scanned
// for controls.
func (m *Model) overviewEditorMouse(x, y int, handler func(tea.KeyPressMsg) tea.Cmd) tea.Cmd {
	if y < 3 || y >= 3+m.overviewHeight() {
		return nil
	}
	rows := m.overviewRows()
	at := m.DescriptionScroll + y - 3
	if at < 0 || at >= len(rows) || rows[at].action != "editor" {
		return nil
	}
	first := at
	for first > 0 && rows[first-1].action == "editor" {
		first--
	}
	inset := 0
	if m.Width >= 6 {
		inset = 2
	}
	local := x - inset
	wrapWidth := max(1, m.Width-4)
	if local < 0 || local >= wrapWidth {
		return nil
	}
	lines := m.overviewComposerLines()
	physical := at - first
	for logical, line := range lines {
		wrapped := strings.Split(ansi.Wrap(line, wrapWidth, ""), "\n")
		if physical >= len(wrapped) {
			physical -= len(wrapped)
			continue
		}
		// Consecutive wrapped fragments consume the original line monotonically.
		offset := 0
		for part := 0; part <= physical; part++ {
			next := strings.Index(line[offset:], wrapped[part])
			if next < 0 {
				return nil
			}
			offset += next
			if part == physical {
				break
			}
			offset += len(wrapped[part])
		}
		columnStart := visibleWidth(line[:offset])
		column := columnStart + local
		if logical == len(lines)-1 {
			footer := "Enter: post · shift+enter: newline · ctrl+r: refresh · esc: cancel"
			if m.discussions.editor != nil {
				footer = "Enter: post · shift+enter: newline · ctrl+r: refresh · esc: discard"
			}
			if key := hintMouseKey(footer, column, min(m.Width, columnStart+visibleWidth(wrapped[physical])), columnStart); key != "" {
				return handler(mouseKey(key))
			}
			return nil
		}
		if m.discussions.editor != nil {
			e := m.discussions.editor
			m.fullEditorMouse(&e.draft, &e.cursor, column, logical)
			return nil
		}
		if menu := m.CommentMenu; menu != nil && menu.mode == commentActionReply {
			basic := m.inlineReplyEditorLines(0)
			bodyRow := logical - 2 // title followed by box border
			if bodyRow >= 0 && bodyRow < len(basic)-2 && column >= 4 {
				if cursor, ok := editorCursorAt(menu.Draft, menu.Cursor, m.overlayInnerWidth(0), true, true, column-4, bodyRow); ok {
					menu.Cursor = cursor
					m.editorCursorVisible = true
				}
			}
		}
		return nil
	}
	return nil
}
