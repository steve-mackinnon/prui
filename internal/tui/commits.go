package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"image"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
	"unicode/utf8"
)

// Commit browsing owns its own cursor and never changes main review progress.
type commitState struct {
	selectedSHA                 string
	selected, railOffset, width int
	focus                       pane
	offsets                     map[string]int
	cursors                     map[string]int
	sourceCache                 commitSourceCache
	cache                       commitRenderCache
}
type commitRenderCache struct {
	session    *review.Session
	sha        string
	width      int
	theme      string
	generation uint64
	rows       []diffLine
}

func (m *Model) commitEntries() []commits.Entry {
	if m.Session == nil || m.Session.Commits == nil {
		return nil
	}
	return m.Session.Commits.Entries
}

// SHA is the stable selection identity; the index is derived for navigation.
func (m *Model) commitSelection() {
	entries := m.commitEntries()
	if len(entries) == 0 {
		m.commit.selectedSHA = ""
		m.commit.selected = 0
		return
	}
	for i, e := range entries {
		if e.SHA == m.commit.selectedSHA {
			m.commit.selected = i
			return
		}
	}
	m.commit.selected = 0
	m.commit.selectedSHA = entries[0].SHA
}
func (m *Model) selectCommit(index int) {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return
	}
	m.commit.selected = max(0, min(index, len(entries)-1))
	m.commit.selectedSHA = entries[m.commit.selected].SHA
}
func shortCommitSHA(sha string) string { return sha[:min(12, len(sha))] }
func commitSubject(e commits.Entry) string {
	if e.Subject == "" {
		return "(no subject)"
	}
	return Escape(e.Subject)
}
func commitMeta(e commits.Entry) string {
	author := e.Author
	if author == "" {
		author = "Unknown author"
	}
	return Escape(author) + " · " + shortCommitSHA(e.SHA)
}
func (m *Model) commitListWidth() int {
	if m.Width < 100 {
		return max(1, m.Width-2)
	}
	width := m.commit.width
	if width == 0 {
		width = min(36, m.Width/3)
	}
	return m.clampListWidth(width)
}
func (m *Model) commitDetailWidth() int {
	if m.Width < 100 {
		return max(1, m.Width-2)
	}
	return m.Width - m.commitListWidth() - 3
}
func (m *Model) commitGeometry() workspaceGeometry {
	screen := image.Rect(0, 0, m.Width, m.Height)
	end := 3 + m.bodyHeight()
	if m.Width < 100 {
		r := image.Rect(1, 3, m.Width-1, end).Intersect(screen)
		if m.commit.focus == paneDiff {
			return workspaceGeometry{Detail: r}
		}
		return workspaceGeometry{Rail: r}
	}
	left := m.commitListWidth()
	return workspaceGeometry{Rail: image.Rect(1, 3, left+1, end).Intersect(screen), Detail: image.Rect(left+2, 3, m.Width-1, end).Intersect(screen), Divider: image.Rect(left+1, 3, left+2, end).Intersect(screen)}
}
func (m *Model) commitRail() []listLine {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return []listLine{{row: -1, text: m.commitSafeState()}}
	}
	m.commitSelection()
	selected := m.commit.selected * 2
	height := m.bodyHeight()
	if selected < m.commit.railOffset {
		m.commit.railOffset = selected
	}
	if selected+min(2, height) > m.commit.railOffset+height {
		m.commit.railOffset = selected + min(2, height) - height
	}
	m.commit.railOffset = max(0, min(m.commit.railOffset, max(0, len(entries)*2-height)))
	rows := make([]listLine, 0, height)
	for line := m.commit.railOffset; line < min(len(entries)*2, m.commit.railOffset+height); line++ {
		i := line / 2
		text := "  " + commitMeta(entries[i])
		if m.discussions.loaded || m.readDiscussions != nil {
			text += " · " + m.commitDiscussionCount(entries[i].SHA)
		}
		if line%2 == 0 {
			text = selectionMarker(i == m.commit.selected) + commitSubject(entries[i])
		}
		rows = append(rows, listLine{row: i, text: text})
	}
	return rows
}
func (m *Model) commitSafeState() string {
	if m.Session.Commits == nil {
		return "Commits were not captured for this session."
	}
	if m.Session.Commits.Status == commits.Unavailable {
		return "Commit list unavailable: " + Escape(m.Session.Commits.Reason)
	}
	return "No commits captured for this PR."
}

// Source rows retain only the selected immutable snapshot. Overlay invalidation
// must not discard parsed patches on editor ticks or discussion refreshes.
type commitSourceCache struct {
	session *review.Session
	sha     string
	diff    *commits.Diff
	rows    []diffLine
}

func (m *Model) commitSourceRows(e commits.Entry) []diffLine {
	c := &m.commit.sourceCache
	if c.session == m.Session && c.sha == e.SHA && c.diff == e.Diff {
		return c.rows
	}
	rows := body(classTitle, commitSubject(e))
	rows = append(rows, body(classMetadata, commitMeta(e))...)
	comparison := "Compared with empty tree (root commit)"
	if len(e.Parents) > 0 {
		comparison = "Compared with " + shortCommitSHA(e.Parents[0])
		if len(e.Parents) > 1 {
			comparison = "Compared with first parent · " + shortCommitSHA(e.Parents[0])
		}
	}
	rows = append(rows, body(classMetadata, comparison)...)
	if e.Status != commits.Captured || e.Diff == nil {
		rows = append(rows, body(classWarning, "Commit diff unavailable: "+Escape(e.Reason))...)
	} else {
		if !e.Diff.Complete {
			rows = append(rows, body(classWarning, "INCOMPLETE commit diff; some content unavailable.")...)
		}
		if len(e.Diff.Files) == 0 && e.Diff.Complete {
			rows = append(rows, body(classMetadata, "No tree changes.")...)
		}
		rows = append(rows, commitDiffRowsFor(e.Diff, m.Session.Inventory.Comparison.Metadata.Identity, e.SHA)...)
	}
	*c = commitSourceCache{session: m.Session, sha: e.SHA, diff: e.Diff, rows: rows}
	m.commit.cache = commitRenderCache{}
	return rows
}

func (m *Model) commitRows() []diffLine {
	entries := m.commitEntries()
	if len(entries) == 0 {
		m.commit.sourceCache = commitSourceCache{}
		m.commit.cache = commitRenderCache{}
		return body(classMetadata, m.commitSafeState())
	}
	m.commitSelection()
	e := entries[m.commit.selected]
	sourceRows := m.commitSourceRows(e)
	c := &m.commit.cache
	if m.Composer == nil && c.session == m.Session && c.sha == e.SHA && c.width == m.commitDetailWidth() && c.theme == m.theme.Name && c.generation == m.discussionGeneration() {
		return c.rows
	}
	// Allocate an overlay slice only when the selected commit has an overlay.
	// Never append into sourceRows: its spare capacity belongs to the source cache.
	var rows []diffLine
	for i, row := range sourceRows {
		if rows != nil {
			rows = append(rows, row)
		}
		if row.target == nil {
			continue
		}
		overlay := m.commitDiscussionLines(*row.target)
		if m.Composer != nil && m.Composer.CommitSHA == e.SHA && m.Composer.Target == *row.target {
			overlay = append(overlay, body(classMetadata, fmt.Sprintf("Comment on %s · %s:%d · %s", shortCommitSHA(e.SHA), Escape(row.target.Path), row.target.Line, row.target.Side))...)
			overlay = append(overlay, m.inlineEditorLines()...)
		}
		if len(overlay) > 0 {
			if rows == nil {
				rows = make([]diffLine, i+1, len(sourceRows)+len(overlay))
				copy(rows, sourceRows[:i+1])
			}
			rows = append(rows, overlay...)
		}
	}
	if rows == nil {
		rows = sourceRows
	}
	// Transient editors must never enter the reusable overlay cache.
	if m.Composer == nil {
		*c = commitRenderCache{session: m.Session, sha: e.SHA, width: m.commitDetailWidth(), theme: m.theme.Name, generation: m.discussionGeneration(), rows: rows}
	}
	return rows
}

// This pure renderer accepts structured immutable commit data and deliberately
// never constructs a review Session or creates a PR comment target.
func commitDiffRows(d *commits.Diff) []diffLine { return commitDiffRowsFor(d, source.Identity{}, "") }
func commitDiffRowsFor(d *commits.Diff, identity source.Identity, sha string) []diffLine {
	var rows []diffLine
	unitsByFile := make(map[string][]inventory.ReviewUnit, len(d.Files))
	for _, unit := range d.Units {
		unitsByFile[unit.FileChangeID] = append(unitsByFile[unit.FileChangeID], unit)
	}
	for _, f := range d.Files {
		rows = append(rows, body(classFileHeader, fileDivider(f))...)
		if f.OldMode != f.NewMode {
			rows = append(rows, body(classMetadata, fmt.Sprintf("Mode: %s -> %s", Escape(f.OldMode), Escape(f.NewMode)))...)
		}
		for _, u := range unitsByFile[f.ID] {
			switch u.Kind {
			case inventory.TextHunk:
				for _, row := range textHunkLines(f, u, d.Patches[u.PatchReference], identity, sha, d.Syntax[u.ID]) {
					if t := row.target; t != nil && !validCommitPatchAnchor(sha, []byte(t.Path), t.Line) {
						row.target = nil
					}
					rows = append(rows, row)
				}
			case inventory.Binary:
				rows = append(rows, body(cardClass(u.Kind), "Binary content changed; no text patch.")...)
			case inventory.Gitlink:
				rows = append(rows, body(cardClass(u.Kind), fmt.Sprintf("Submodule pointer: %s -> %s", Escape(f.OldOID), Escape(f.NewOID)))...)
			case inventory.Unavailable:
				rows = append(rows, body(classWarning, "UNAVAILABLE: "+Escape(u.UnavailableReason))...)
			}
		}
	}
	for _, problem := range d.Problems {
		rows = append(rows, body(classWarning, Escape(problem))...)
	}
	return rows
}
func (m *Model) commitOffset() int {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return 0
	}
	m.commitSelection()
	if m.commit.offsets == nil {
		m.commit.offsets = map[string]int{}
	}
	sha := entries[m.commit.selected].SHA
	offset := max(0, min(m.commit.offsets[sha], max(0, len(m.commitRows())-m.bodyHeight())))
	m.commit.offsets[sha] = offset
	return offset
}
func (m *Model) commitScroll(delta int) {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return
	}
	offset := m.commitOffset()
	m.commit.offsets[entries[m.commit.selected].SHA] = max(0, min(offset+delta, max(0, len(m.commitRows())-m.bodyHeight())))
}
func (m *Model) commitMove(delta int) {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return
	}
	m.commitSelection()
	m.selectCommit(m.commit.selected + delta)
	m.commitRail()
	m.commitOffset()
}
func (m *Model) commitKey(key string) {
	switch key {
	case "n":
		m.commitMove(1)
	case "p":
		m.commitMove(-1)
	case "h", "ctrl+h", "esc":
		m.commit.focus = paneList
	case "l", "ctrl+l", "enter":
		if len(m.commitEntries()) > 0 {
			m.commit.focus = paneDiff
		}
	case "[", "]":
		if m.Width >= 100 {
			delta := -2
			if key == "]" {
				delta = 2
			}
			m.commit.width = m.clampListWidth(m.commitListWidth() + delta)
		}
	case "j", "down", "k", "up", "d", "pgdown", "u", "pgup", "home", "end":
		delta := 1
		if key == "k" || key == "up" {
			delta = -1
		}
		if key == "d" || key == "pgdown" {
			delta = m.bodyHeight()
			if m.commit.focus == paneList {
				delta = max(1, m.bodyHeight()/2)
			}
		}
		if key == "u" || key == "pgup" {
			delta = -m.bodyHeight()
			if m.commit.focus == paneList {
				delta = -max(1, m.bodyHeight()/2)
			}
		}
		if key == "home" {
			delta = -int(^uint(0) >> 2)
		}
		if key == "end" {
			delta = int(^uint(0) >> 2)
		}
		if m.commit.focus == paneList {
			m.commitMove(delta)
		} else {
			m.commitCursorMove(delta)
		}
	}
}
func (m *Model) commitsView() string {
	rail := m.commitRail()
	rows := m.commitRows()
	offset := m.commitOffset()
	rows = rows[offset:min(len(rows), offset+m.bodyHeight())]
	leftLabel := fmt.Sprintf("Commits · %d/%d", min(len(m.commitEntries()), m.commit.selected+1), len(m.commitEntries()))
	rightLabel := "Commit diff · enter/l: focus"
	if len(m.commitEntries()) > 0 {
		rightLabel = "Commit diff · " + shortCommitSHA(m.commitEntries()[m.commit.selected].SHA)
	}
	left, right := m.commitListWidth(), m.commitDetailWidth()
	focus := m.commit.focus
	var header, footer string
	if m.Width < 100 {
		label := leftLabel + " · enter/l: diff"
		if focus == paneDiff {
			label = fmt.Sprintf("Commit diff · %d/%d · %s · esc/h: list", m.commit.selected+1, len(m.commitEntries()), shortCommitSHA(m.commit.selectedSHA))
		}
		header = m.styleLine(classPaneHeaderFocused, "┌"+paneHeaderText(label, m.Width-2)+"┐")
		footer = m.styleLine(classPaneBorderFocused, "└"+strings.Repeat("─", max(0, m.Width-2))+"┘")
	} else {
		header = m.styleLine(paneBorderClass(focus == paneList), "┌"+paneHeaderText(leftLabel, left)) + m.styleLine(classPaneBorderFocused, "┬") + m.styleLine(paneBorderClass(focus == paneDiff), paneHeaderText(rightLabel, right)+"┐")
		footer = m.styleLine(classPaneBorder, "└"+strings.Repeat("─", left)+"┴"+strings.Repeat("─", right)+"┘")
	}
	var lines []string
	borders := paneBodyBorders{classPaneBorder, classPaneBorder, classPaneBorder}
	for i := 0; i < m.bodyHeight(); i++ {
		l, r := "", ""
		lc, rc := classPlain, classPlain
		if i < len(rail) {
			l = rail[i].text
			if rail[i].row == m.commit.selected {
				lc = selectedClass(focus == paneList)
			}
		}
		if i < len(rows) {
			row := rows[i]
			numbered := numberedPatchText(row)
			prefix := numbered[:len(numbered)-len(row.Text)]
			r, rc = m.syntaxText(row, 0, right, prefix), row.Class
			if focus == paneDiff && offset+i == m.commitCursor() {
				rc = classSelectionFocused
			}
		}
		lines = append(lines, m.paneBodyRow(styledLine{Class: lc, Text: l}, styledLine{Class: rc, Text: r}, left, right, focus, borders))
	}
	return header + "\n" + strings.Join(lines, "\n") + "\n" + footer
}

func validCommitPatchAnchor(sha string, path []byte, line int) bool {
	return sha != "" && line > 0 && len(path) > 0 && utf8.Valid(path) && !strings.ContainsAny(string(path), "\x00\r\n")
}

func (m *Model) commitCursor() int {
	if m.commit.cursors == nil {
		m.commit.cursors = map[string]int{}
	}
	return m.commit.cursors[m.commit.selectedSHA]
}
func (m *Model) commitCursorMove(delta int) {
	rows := m.commitRows()
	if len(rows) == 0 {
		return
	}
	offset := m.commitOffset()
	current := max(offset, min(m.commitCursor(), offset+m.bodyHeight()-1))
	m.commitScroll(delta)
	cursor := max(0, min(current+delta, len(rows)-1))
	m.commit.cursors[m.commit.selectedSHA] = cursor
	offset = m.commitOffset()
	if cursor < offset {
		m.commit.offsets[m.commit.selectedSHA] = cursor
	}
	if cursor >= offset+m.bodyHeight() {
		m.commit.offsets[m.commit.selectedSHA] = max(0, cursor-m.bodyHeight()+1)
	}
}
func (m *Model) viewOriginalDiscussion(target *source.ReviewCommentTarget, sha string) bool {
	for i, e := range m.commitEntries() {
		if e.SHA != sha {
			continue
		}
		if e.Status != commits.Captured || e.Diff == nil {
			return false
		}
		// Check captured source before navigating so an unavailable anchor retains
		// the discussion detail and its snippet rather than losing its context.
		if target != nil {
			found := false
			for _, row := range commitDiffRowsFor(e.Diff, m.Session.Inventory.Comparison.Metadata.Identity, e.SHA) {
				if row.target != nil && *row.target == *target {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		m.selectCommit(i)
		m.selectReviewView(viewCommits)
		m.commit.focus = paneDiff
		if target != nil {
			for n, row := range m.commitRows() {
				if row.target != nil && *row.target == *target {
					m.commitCursor()
					m.commit.cursors[sha] = n
					m.commit.offsets[sha] = max(0, n-2)
					break
				}
			}
		}
		return true
	}
	return false
}
func (m *Model) openCommitComposer() tea.Cmd {
	rows := m.commitRows()
	cursor := m.commitCursor()
	if cursor >= len(rows) || rows[cursor].target == nil {
		return nil
	}
	target := *rows[cursor].target
	metadata := m.Session.Inventory.Comparison.Metadata
	if target.CommitID != metadata.HeadSHA && !commits.HistoricalCommentTarget(m.Session.Commits, target) {
		m.ActionError = errors.New("commenting on this historical line is unsupported; use an added or context line in a regular commit")
		return nil
	}
	matched := false
	for i := range m.Session.Inventory.Units {
		for _, row := range unitLines(m.Session, i) {
			if row.target != nil && *row.target == target {
				matched = true
			}
		}
	}
	if target.CommitID == metadata.HeadSHA && !matched {
		m.ActionError = errors.New("this commit line is not in the captured PR diff; commenting unavailable")
		return nil
	}
	m.Composer = &commentComposer{Target: target, CommitSHA: target.CommitID, CommitBundle: m.Session.Commits, CommitInventory: &m.Session.Inventory, PendingIndex: -1}
	m.commit.cache = commitRenderCache{}
	m.editorCursorVisible = true
	m.editorCursorGeneration++
	m.ensureCommitEditorVisible()
	return nextEditorCursorTick(m.editorCursorGeneration)
}

// ensureCommitEditorVisible adjusts only the commit viewport. Long editors keep
// the active text cursor visible; shorter editors fit their complete box.
func (m *Model) ensureCommitEditorVisible() {
	c := m.Composer
	if c == nil || c.CommitSHA == "" || c.CommitSHA != m.commit.selectedSHA {
		return
	}
	rows := m.commitRows()
	first, last := -1, -1
	for i, row := range rows {
		if row.editor {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	offset := m.commitOffset()
	height := m.bodyHeight()
	if last-first+1 <= height {
		if last >= offset+height {
			offset = last - height + 1
		}
		if first < offset {
			offset = first
		}
	} else {
		runes := []rune(c.Draft)
		cursor := max(0, min(c.Cursor, len(runes)))
		line := first + 1 + strings.Count(string(runes[:cursor]), "\n")
		if line >= offset+height {
			offset = line - height + 1
		}
		if line < offset {
			offset = line
		}
	}
	m.commit.offsets[c.CommitSHA] = max(0, min(offset, max(0, len(rows)-height)))
}
