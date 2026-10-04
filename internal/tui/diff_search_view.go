package tui

import (
	"context"
	"fmt"
	"image"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

type searchResultRow struct {
	text  string
	match int
}

func (m *Model) searchHeaderBounds() image.Rectangle {
	if m.Width < 100 {
		if m.Focus != paneDiff {
			return image.Rectangle{}
		}
		label, _ := m.fileFilterHeader()
		if m.selectedReviewView() == viewGuide {
			label = "Guide"
		}
		if m.Inventory {
			label = "Full inventory (i)"
		}
		start := 2 + visibleWidth(label+" · ")
		if start+13 > m.Width-1 {
			return image.Rectangle{}
		}
		return image.Rect(start, 2, start+13, 3)
	}
	left := m.listWidth() + 3
	return image.Rect(left, 2, min(m.Width-1, left+13), 3)
}
func (m *Model) searchBounds() image.Rectangle {
	w, h := min(68, m.Width), min(24, m.Height)
	return image.Rect(max(0, m.Width-w), 0, m.Width, h)
}
func (m *Model) searchResultRows(width int) []searchResultRow {
	s := m.searchState()
	counts := map[int]int{}
	for _, match := range s.matches {
		counts[s.documents[match.Document].File]++
	}
	var rows []searchResultRow
	previous := -1
	for i, match := range s.matches {
		d := s.documents[match.Document]
		if d.File != previous {
			f := m.Session.Inventory.Files[d.File]
			heading := fmt.Sprintf("%s  %s (%d)", fileName(f), fileDirectory(f), counts[d.File])
			rows = append(rows, searchResultRow{m.styleLine(classFileHeader, clip(heading, width)), -1})
			previous = d.File
		}
		if i < s.selected-max(1, m.Height) || i > s.selected+max(1, m.Height) {
			rows = append(rows, searchResultRow{"", i})
			continue
		}
		side, line := "R", d.New
		if line == 0 {
			side, line = "L", d.Old
		}
		prefix := fmt.Sprintf("%s %s%d %c ", selectionMarker(i == s.selected), side, line, d.Marker)
		// Bound snippet work to nearby source, even for a megabyte-long line.
		rawStart, rawEnd := match.Range.Start, match.Range.End
		for n := 0; n < max(4, (width-visibleWidth(prefix))/3) && rawStart > 0; n++ {
			_, size := utf8.DecodeLastRuneInString(d.Text[:rawStart])
			rawStart -= size
		}
		for n := 0; n < width && rawEnd < len(d.Text); n++ {
			_, size := utf8.DecodeRuneInString(d.Text[rawEnd:])
			rawEnd += size
		}
		text := Escape(d.Text[rawStart:rawEnd])
		start := len(Escape(d.Text[rawStart:match.Range.Start]))
		end := start + len(Escape(d.Text[match.Range.Start:match.Range.End]))
		offset := max(0, utf8.RuneCountInString(text[:start])-max(4, (width-visibleWidth(prefix))/3))
		runes := []rune(text)
		byteStart := len(string(runes[:min(offset, len(runes))]))
		snippet := clip(text[byteStart:], max(0, width-visibleWidth(prefix)))
		a, b := max(0, start-byteStart), min(len(snippet), end-byteStart)
		if a < b {
			snippet = snippet[:a] + m.searchStyle(i == s.selected).Render(snippet[a:b]) + snippet[b:]
		}
		rows = append(rows, searchResultRow{prefix + snippet, i})
	}
	return rows
}
func (m *Model) searchStyle(active bool) lipgloss.Style {
	style := lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion)
	if m.colorProfile <= colorprofile.Ascii {
		return style
	}
	if c, ok := m.theme.Color(theme.Warning); ok {
		style = style.Background(c)
	}
	if c, ok := m.theme.Color(theme.Background); ok {
		style = style.Foreground(c)
	}
	if active {
		style = style.Underline(true).Bold(true)
	}
	return style
}
func (m *Model) searchPopover(background string) string {
	s := m.searchState()
	bounds := m.searchBounds()
	w, h := bounds.Dx(), bounds.Dy()
	inside := max(1, w-4)
	query := []rune(s.query)
	cursor := min(s.cursor, len(query))
	input := Escape(string(query[:cursor])) + "▏" + Escape(string(query[cursor:]))
	if w < 24 || h < 9 {
		return strings.Join([]string{clip("Find: "+input, w), clip("Resize for results · Esc closes", w)}, "\n")
	}
	scope := "Files · saved diff text only"
	if m.Inventory {
		scope = "Inventory · saved diff text only"
	} else if m.selectedReviewView() == viewGuide {
		scope = "Current section · saved diff text only"
	}
	if s.skipped > 0 {
		scope += fmt.Sprintf(" · %d files skipped", s.skipped)
	}
	summary := "Type to search diff text"
	switch {
	case m.selectedReviewView() == viewGuide && !m.Inventory && s.scope.Section < 0:
		summary = "Select a guide section to search"
	case s.pending:
		summary = "Searching…"
	case s.query != "" && len(s.matches) == 0:
		summary = "No matches in saved diff text"
	case s.capped:
		summary = "First 10,000 matches; refine your query"
	case len(s.matches) > 0:
		files := map[int]bool{}
		for _, match := range s.matches {
			files[s.documents[match.Document].File] = true
		}
		matchLabel, fileLabel := "matches", "files"
		if len(s.matches) == 1 {
			matchLabel = "match"
		}
		if len(files) == 1 {
			fileLabel = "file"
		}
		summary = fmt.Sprintf("%d %s in %d %s", len(s.matches), matchLabel, len(files), fileLabel)
	}
	clear, closeLabel := "Clear", "Close"
	if s.control == 1 {
		clear = "› Clear"
	}
	if s.control == 2 {
		closeLabel = "› Close"
	}
	content := []string{"╭" + strings.Repeat("─", w-2) + "╮", modalLine("Find: "+clipSearchInput(input, inside-6), inside), modalLine(clear+"  |  "+closeLabel, inside), modalLine(scope, inside), modalLine(summary, inside)}
	rows := m.searchResultRows(inside)
	capacity := h - 7
	selectedRow := 0
	for i, row := range rows {
		if row.match == s.selected {
			selectedRow = i
			break
		}
	}
	if selectedRow < s.scroll {
		s.scroll = selectedRow
	}
	if selectedRow >= s.scroll+capacity {
		s.scroll = selectedRow - capacity + 1
	}
	s.scroll = min(s.scroll, max(0, len(rows)-capacity))
	for i := 0; i < capacity; i++ {
		text := ""
		if at := s.scroll + i; at < len(rows) {
			text = rows[at].text
		}
		content = append(content, modalLine(text, inside))
	}
	content = append(content, modalLine("↑↓ results · Enter jump · Tab controls · Esc", inside), "╰"+strings.Repeat("─", w-2)+"╯")
	canvas := lipgloss.NewCanvas(m.Width, m.Height)
	return canvas.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(strings.Join(viewportLines(background, m.Width, m.Height), "\n")), lipgloss.NewLayer(strings.Join(content, "\n")).X(bounds.Min.X))).Render()
}
func clipSearchInput(input string, width int) string {
	cursor := strings.Index(input, "▏")
	if cursor < 0 {
		return clip(input, width)
	}
	column := visibleWidth(input[:cursor])
	start := max(0, column-width+1)
	return ansi.Cut(input, start, start+max(1, width))
}
func (m *Model) searchMouse(msg tea.MouseMsg) tea.Cmd {
	s := m.searchState()
	p := msg.Mouse()
	bounds := m.searchBounds()
	if !image.Pt(p.X, p.Y).In(bounds) {
		return nil
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		if wheel.Button == tea.MouseWheelDown {
			s.selected = min(max(0, len(s.matches)-1), s.selected+3)
		}
		if wheel.Button == tea.MouseWheelUp {
			s.selected = max(0, s.selected-3)
		}
		return nil
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return nil
	}
	if click.Y == 2 {
		if click.X-bounds.Min.X < 12 {
			s.query = ""
			s.cursor = 0
			return m.startSearch()
		}
		m.closeSearch()
		return nil
	}
	if click.Y == 1 {
		s.control = 0
		s.cursor = utf8.RuneCountInString(s.query)
		return nil
	}
	rows := m.searchResultRows(max(1, bounds.Dx()-4))
	i := click.Y - 5 + s.scroll
	if click.Y >= 5 && click.Y < bounds.Dy()-2 && i >= 0 && i < len(rows) && rows[i].match >= 0 {
		s.selected = rows[i].match
		m.activateSearchMatch()
	}
	return nil
}

// Highlight ranges are derived from raw bytes and projected into the escaped
// viewport; no ANSI or screen coordinate participates in source identity.
func (m *Model) searchHighlightedText(line diffLine, horizontal, width int, prefix string) (string, bool) {
	if line.searchID.Unit == "" || !m.diffReviewView() || m.Session == nil {
		return "", false
	}
	s := m.existingSearch()
	if s == nil || s.session != m.Session || s.query == "" || s.scope != m.currentSearchScope() {
		return "", false
	}
	if m.selectedReviewView() == viewGuide && !m.Inventory {
		if s.scope.Guide < 0 || s.scope.Section < 0 {
			return "", false
		}
		found := false
		for _, id := range m.Session.Guides.Items[s.scope.Guide].Sections[s.scope.Section].UnitIDs {
			if id == line.searchID.Unit {
				found = true
				break
			}
		}
		if !found {
			return "", false
		}
	}
	prefix = clip(prefix, width)
	runes := []rune(line.Text)
	start := len(string(runes[:min(max(0, horizontal), len(runes))]))
	text := clip(line.Text[start:], max(0, width-visibleWidth(prefix)))
	var out strings.Builder
	out.WriteString(prefix)
	last := 0
	render := func(a, b int, highlight, active bool) {
		if a >= b {
			return
		}
		piece := line
		piece.searchID = searchSourceID{}
		piece.Text = text[a:b]
		piece.syntax = cropSpans(line.syntax, start+a, start+b, 0)
		styled := m.syntaxText(piece, 0, visibleWidth(piece.Text), "")
		if highlight {
			styled = m.searchStyle(active).Render(styled)
		}
		out.WriteString(styled)
	}
	rawEnd, escapedEnd := 0, 1
	newSearchPattern(s.query).each(context.Background(), line.rawSource, func(match searchRange) bool {
		escapedEnd += len(Escape(line.rawSource[rawEnd:match.Start]))
		a := escapedEnd - line.sourceOffset - start
		escapedEnd += len(Escape(line.rawSource[match.Start:match.End]))
		rawEnd = match.End
		b := escapedEnd - line.sourceOffset - start
		if a >= len(text) {
			return false
		}
		a, b = max(0, a), min(len(text), b)
		if a >= b {
			return true
		}
		active := false
		if s.selected >= 0 && s.selected < len(s.matches) {
			selected := s.matches[s.selected]
			active = s.documents[selected.Document].ID == line.searchID && selected.Range == match
		}
		render(last, a, false, false)
		render(a, b, true, active)
		last = b
		return true
	})
	render(last, len(text), false, false)
	return out.String(), true
}
