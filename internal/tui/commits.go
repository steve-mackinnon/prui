package tui

import (
	"bytes"
	"fmt"
	"image"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/review"
	"strings"
)

// Commit browsing owns no source cursors, comment anchors or review progress.
type commitState struct {
	selectedSHA                 string
	selected, railOffset, width int
	focus                       pane
	offsets                     map[string]int
	cache                       commitRenderCache
}
type commitRenderCache struct {
	session *review.Session
	sha     string
	width   int
	theme   string
	rows    []diffLine
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
func (m *Model) commitRows() []diffLine {
	entries := m.commitEntries()
	if len(entries) == 0 {
		return body(classMetadata, m.commitSafeState())
	}
	m.commitSelection()
	e := entries[m.commit.selected]
	c := &m.commit.cache
	if c.session == m.Session && c.sha == e.SHA && c.width == m.commitDetailWidth() && c.theme == m.theme.Name {
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
		rows = append(rows, commitDiffRows(e.Diff)...)
	}
	*c = commitRenderCache{session: m.Session, sha: e.SHA, width: m.commitDetailWidth(), theme: m.theme.Name, rows: rows}
	return rows
}

// This pure renderer accepts structured immutable commit data and deliberately
// never constructs a review Session or creates a PR comment target.
func commitDiffRows(d *commits.Diff) []diffLine {
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
				old, new := u.OldRange.Start, u.NewRange.Start
				for _, raw := range bytes.Split(d.Patches[u.PatchReference], []byte{'\n'}) {
					if isGitFilePreamble(raw) {
						continue
					}
					text := Escape(string(raw))
					class := classifyPatch(text)
					if o, n, ok := hunkStarts(raw); ok {
						old, new = o, n
					} else if len(raw) > 0 {
						switch raw[0] {
						case '+':
							text = fmt.Sprintf("     %4d %s", new, text)
							new++
						case '-':
							text = fmt.Sprintf("%4d      %s", old, text)
							old++
						case ' ':
							text = fmt.Sprintf("%4d %4d %s", old, new, text)
							old++
							new++
						}
					}
					rows = append(rows, diffLine{styledLine: styledLine{Class: class, Text: text}})
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
			m.commitScroll(delta)
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
	header := ""
	footer := ""
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
			r, rc = rows[i].Text, rows[i].Class
		}
		if m.Width < 100 {
			if focus == paneDiff {
				lines = append(lines, m.frameBodyLine(r, rc, right, true))
			} else {
				lines = append(lines, m.frameBodyLine(l, lc, left, true))
			}
		} else {
			border := m.styleLine(classPaneBorder, "│")
			lines = append(lines, border+m.frameBodyLine(l, lc, left, false)+border+m.frameBodyLine(r, rc, right, false)+border)
		}
	}
	return header + "\n" + strings.Join(lines, "\n") + "\n" + footer
}
