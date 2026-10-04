package tui

import (
	"bytes"
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"prui/internal/inventory"
	"prui/internal/review"
)

const searchResultLimit = 10000

type searchRange struct{ Start, End int }
type searchSourceID struct {
	Unit string
	Row  int
}
type searchDocument struct {
	ID                   searchSourceID
	Text                 string
	File, Unit, Old, New int
	Marker               byte
}
type searchMatch struct {
	Document int
	Range    searchRange
}
type searchScope struct {
	GuideView      bool
	Guide, Section int
	Inventory      bool
}
type diffSearchState struct {
	session                           *review.Session
	scope                             searchScope
	query                             string
	cursor, selected, scroll, control int
	open, pending, capped             bool
	previousFocus                     pane
	generation                        uint64
	cancel                            context.CancelFunc
	documents                         []searchDocument
	matches                           []searchMatch
	skipped                           int
}
type diffSearchResult struct {
	state      *diffSearchState
	generation uint64
	documents  []searchDocument
	matches    []searchMatch
	skipped    int
	capped     bool
}

func foldSearchRune(r rune) rune {
	smallest := r
	for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
		if next < smallest {
			smallest = next
		}
	}
	return smallest
}

// A folded KMP pattern keeps long lines and repetitive queries linear-time.
type searchPattern struct {
	runes   []rune
	failure []int
}

func newSearchPattern(query string) searchPattern {
	p := searchPattern{}
	if !utf8.ValidString(query) {
		return p
	}
	p.runes = []rune(query)
	p.failure = make([]int, len(p.runes))
	for i := range p.runes {
		p.runes[i] = foldSearchRune(p.runes[i])
	}
	for i, j := 1, 0; i < len(p.runes); i++ {
		for j > 0 && p.runes[i] != p.runes[j] {
			j = p.failure[j-1]
		}
		if p.runes[i] == p.runes[j] {
			j++
		}
		p.failure[i] = j
	}
	return p
}
func (p searchPattern) each(ctx context.Context, text string, visit func(searchRange) bool) {
	if len(p.runes) == 0 || !utf8.ValidString(text) {
		return
	}
	matched, index := 0, 0
	for at, r := range text {
		if index%1024 == 0 && ctx.Err() != nil {
			return
		}
		folded := foldSearchRune(r)
		for matched > 0 && folded != p.runes[matched] {
			matched = p.failure[matched-1]
		}
		if folded == p.runes[matched] {
			matched++
		}
		if matched == len(p.runes) {
			end := at + utf8.RuneLen(r)
			start := end
			for range p.runes {
				_, size := utf8.DecodeLastRuneInString(text[:start])
				start -= size
			}
			if !visit(searchRange{start, end}) {
				return
			}
			matched = 0
		}
		index++
	}
}
func (p searchPattern) ranges(ctx context.Context, text string, limit int) []searchRange {
	if limit <= 0 {
		return nil
	}
	var result []searchRange
	p.each(ctx, text, func(r searchRange) bool { result = append(result, r); return len(result) < limit })
	return result
}
func literalSearchRanges(text, query string, limit int) []searchRange {
	return newSearchPattern(query).ranges(context.Background(), text, limit)
}

func (m *Model) currentSearchScope() searchScope {
	scope := searchScope{Guide: -1, Section: -1, Inventory: m.Inventory, GuideView: m.selectedReviewView() == viewGuide && !m.Inventory}
	if !m.Inventory && m.selectedReviewView() == viewGuide {
		rows := m.rows()
		if len(rows) > 0 {
			r := rows[min(max(m.Row, 0), len(rows)-1)]
			scope.Guide, scope.Section = r.guide, r.section
		}
	}
	return scope
}

func (m *Model) existingSearch() *diffSearchState {
	slot := 0
	if m.selectedReviewView() == viewGuide {
		slot = 1
	}
	return m.search[slot]
}

func (m *Model) searchState() *diffSearchState {
	slot := 0
	if m.selectedReviewView() == viewGuide {
		slot = 1
	}
	s := m.search[slot]
	if s == nil || s.session != m.Session {
		if s != nil && s.cancel != nil {
			s.cancel()
		}
		s = &diffSearchState{session: m.Session, scope: m.currentSearchScope()}
		m.search[slot] = s
	}
	return s
}

func searchDocuments(ctx context.Context, session *review.Session, scope searchScope) ([]searchDocument, int) {
	if session == nil || (scope.GuideView && scope.Guide < 0) {
		return nil, 0
	}
	var units []int
	if scope.Guide >= 0 && !scope.Inventory {
		if session.Guides == nil || scope.Section < 0 {
			return nil, 0
		}
		indexes := make(map[string]int, len(session.Inventory.Units))
		for i, u := range session.Inventory.Units {
			indexes[u.ID] = i
		}
		for _, id := range session.Guides.Items[scope.Guide].Sections[scope.Section].UnitIDs {
			if i, ok := indexes[id]; ok {
				units = append(units, i)
			}
		}
	} else {
		for _, slice := range session.Slices {
			units = append(units, slice.Units...)
		}
	}
	var documents []searchDocument
	seen := map[string]bool{}
	skipped := map[int]bool{}
	for _, i := range units {
		if ctx.Err() != nil {
			return nil, 0
		}
		u := session.Inventory.Units[i]
		if seen[u.ID] {
			continue
		}
		seen[u.ID] = true
		file := session.UnitFiles[i]
		if u.Kind != inventory.TextHunk {
			if u.Kind != inventory.FileMetadata {
				skipped[file] = true
			}
			continue
		}
		patch := session.Inventory.Patches[u.PatchReference]
		if len(patch) == 0 {
			skipped[file] = true
			continue
		}
		old, new, inHunk := 0, 0, false
		for row, raw := range bytes.Split(patch, []byte{'\n'}) {
			if row%256 == 0 && ctx.Err() != nil {
				return nil, 0
			}
			if o, n, ok := hunkStarts(raw); ok {
				old, new, inHunk = o, n, true
				continue
			}
			if !inHunk || len(raw) == 0 {
				continue
			}
			d := searchDocument{ID: searchSourceID{u.ID, row}, File: file, Unit: i, Marker: raw[0]}
			switch raw[0] {
			case '+':
				d.New = new
				new++
			case '-':
				d.Old = old
				old++
			case ' ':
				d.Old, d.New = old, new
				old++
				new++
			default:
				continue
			}
			if !utf8.Valid(raw[1:]) {
				skipped[file] = true
				continue
			}
			d.Text = string(raw[1:])
			documents = append(documents, d)
		}
	}
	// File grouping is stable even when a guide revisits a file later.
	var grouped []searchDocument
	order := []int{}
	byFile := map[int][]searchDocument{}
	for _, d := range documents {
		if _, ok := byFile[d.File]; !ok {
			order = append(order, d.File)
		}
		byFile[d.File] = append(byFile[d.File], d)
	}
	for _, f := range order {
		grouped = append(grouped, byFile[f]...)
	}
	return grouped, len(skipped)
}

func (m *Model) startSearch() tea.Cmd {
	s := m.searchState()
	if s.cancel != nil {
		s.cancel()
	}
	scope := m.currentSearchScope()
	if s.scope != scope {
		s.scope = scope
		s.documents = nil
	}
	s.generation++
	generation := s.generation
	s.matches = nil
	s.selected = 0
	s.scroll = 0
	s.capped = false
	if s.query == "" {
		s.pending = false
		return nil
	}
	s.pending = true
	ctx, cancel := context.WithCancel(m.ctx)
	s.cancel = cancel
	docs, skipped, session, query := s.documents, s.skipped, s.session, s.query
	return func() tea.Msg {
		if docs == nil {
			docs, skipped = searchDocuments(ctx, session, scope)
		}
		result := diffSearchResult{state: s, generation: generation, documents: docs, skipped: skipped}
		pattern := newSearchPattern(query)
		for i, d := range docs {
			if ctx.Err() != nil {
				return nil
			}
			for _, r := range pattern.ranges(ctx, d.Text, searchResultLimit+1-len(result.matches)) {
				result.matches = append(result.matches, searchMatch{i, r})
				if len(result.matches) > searchResultLimit {
					result.matches = result.matches[:searchResultLimit]
					result.capped = true
					return result
				}
			}
		}
		return result
	}
}

func (m *Model) applySearchResult(r diffSearchResult) {
	for _, s := range m.search {
		if s == r.state && s.session == m.Session && s.generation == r.generation && s.scope == m.currentSearchScope() {
			s.documents, s.matches, s.skipped, s.capped, s.pending = r.documents, r.matches, r.skipped, r.capped, false
		}
	}
}

func (m *Model) searchAvailable() bool {
	return m.Session != nil && m.top() == pageReview && m.diffReviewView() && !m.Busy && !m.Loading && m.Composer == nil && m.CommentMenu == nil && !m.fileFilterEditing
}
func (m *Model) searchOpen() bool {
	s := m.existingSearch()
	return m.searchAvailable() && s != nil && s.session == m.Session && s.open
}
func (m *Model) openSearch() tea.Cmd {
	s := m.searchState()
	s.open = true
	s.previousFocus = m.Focus
	s.control = 0
	if s.scope == m.currentSearchScope() && !s.pending && (s.query == "" || s.documents != nil) {
		return nil
	}
	return m.startSearch()
}
func (m *Model) closeSearch() { s := m.searchState(); s.open = false; m.Focus = s.previousFocus }
func (m *Model) closeSearchPopovers() {
	for _, s := range m.search {
		if s != nil {
			s.open = false
		}
	}
}

func (m *Model) searchKey(v tea.KeyPressMsg) tea.Cmd {
	s := m.searchState()
	runes := []rune(s.query)
	s.cursor = min(s.cursor, len(runes))
	switch v.String() {
	case "ctrl+f":
		s.control = 0
		return nil
	case "esc":
		m.closeSearch()
		return nil
	case "tab":
		s.control = (s.control + 1) % 3
		return nil
	case "shift+tab":
		s.control = (s.control + 2) % 3
		return nil
	case "enter":
		switch s.control {
		case 1:
			s.query = ""
			s.cursor = 0
			return m.startSearch()
		case 2:
			m.closeSearch()
		default:
			m.activateSearchMatch()
		}
		return nil
	case "up":
		s.selected = max(0, s.selected-1)
		return nil
	case "down":
		s.selected = min(max(0, len(s.matches)-1), s.selected+1)
		return nil
	case "pgup":
		s.selected = max(0, s.selected-max(1, m.Height-9))
		return nil
	case "pgdown":
		s.selected = min(max(0, len(s.matches)-1), s.selected+max(1, m.Height-9))
		return nil
	case "left":
		s.cursor = max(0, s.cursor-1)
		return nil
	case "right":
		s.cursor = min(len(runes), s.cursor+1)
		return nil
	case "home":
		s.cursor = 0
		return nil
	case "end":
		s.cursor = len(runes)
		return nil
	case "backspace":
		if s.cursor > 0 {
			runes = append(runes[:s.cursor-1], runes[s.cursor:]...)
			s.cursor--
			s.query = string(runes)
		}
	case "delete":
		if s.cursor < len(runes) {
			s.query = string(append(runes[:s.cursor], runes[s.cursor+1:]...))
		}
	default:
		return m.insertSearchText(v.Text)
	}
	s.control = 0
	return m.startSearch()
}
func (m *Model) insertSearchText(text string) tea.Cmd {
	s := m.searchState()
	if text == "" || strings.ContainsAny(text, "\r\n") || !utf8.ValidString(text) || len(s.query)+len(text) > 1024 {
		return nil
	}
	s.query, s.cursor = insertEditorText(s.query, s.cursor, text)
	s.control = 0
	return m.startSearch()
}

func (m *Model) activateSearchMatch() {
	s := m.searchState()
	if s.pending || s.selected < 0 || s.selected >= len(s.matches) {
		return
	}
	match := s.matches[s.selected]
	doc := s.documents[match.Document]
	m.closeSearch()
	m.Focus = paneDiff
	if s.scope.Guide >= 0 && !m.Inventory {
		delete(m.collapsed.guides, s.scope.Guide)
		delete(m.collapsed.sections, [2]int{s.scope.Guide, s.scope.Section})
		for i, r := range m.rows() {
			if r.guide == s.scope.Guide && r.section == s.scope.Section && r.file == doc.File {
				m.Row = i
				break
			}
		}
	}
	if m.fileView() {
		visible := false
		for _, f := range m.filteredFiles() {
			if f == doc.File {
				visible = true
			}
		}
		if !visible {
			m.fileFilter = ""
			m.notice = "Filename filter cleared to reveal search result"
		}
	}
	m.Selected = doc.Unit
	m.revealSearchMatch()
}

func (m *Model) revealSearchMatch() {
	s := m.searchState()
	if s.selected < 0 || s.selected >= len(s.matches) {
		return
	}
	match := s.matches[s.selected]
	doc := s.documents[match.Document]
	wanted := 1 + len(Escape(doc.Text[:match.Range.Start]))
	wantedEnd := 1 + len(Escape(doc.Text[:match.Range.End]))
	for i, line := range m.displayDetail() {
		cells := []diffLine{line}
		if line.sideBySide != nil {
			cells = nil
			if line.sideBySide.new != nil {
				cells = append(cells, *line.sideBySide.new.line)
			}
			if line.sideBySide.old != nil {
				cells = append(cells, *line.sideBySide.old.line)
			}
		}
		for _, cell := range cells {
			if cell.searchID != doc.ID || wantedEnd <= cell.sourceOffset+1 || wanted >= cell.sourceOffset+len(cell.Text) {
				continue
			}
			m.setCursor(i)
			m.setSelectedDiffTarget(cell.target)
			m.cursorActive = cell.target != nil
			m.setOffset(max(0, i-max(1, m.bodyHeight()/3)))
			position := min(len(cell.Text), max(0, wanted-cell.sourceOffset))
			m.Horizontal = max(0, utf8.RuneCountInString(cell.Text[:position])-8)
			return
		}
	}
}

// Preserve the active occurrence only when it is still the diff cursor's
// location; ordinary scrolling and navigation remain under reviewer control.
func (m *Model) searchMatchAtCursor() bool {
	if m.Session == nil || !m.diffReviewView() || !m.cursorActive {
		return false
	}
	s := m.existingSearch()
	if s == nil || s.open || s.pending || s.selected < 0 || s.selected >= len(s.matches) {
		return false
	}
	match := s.matches[s.selected]
	doc := s.documents[match.Document]
	cursor := m.cursor()
	lines := m.displayDetail()
	if cursor < 0 || cursor >= len(lines) {
		return false
	}
	line := lines[cursor]
	if line.searchID == doc.ID {
		return true
	}
	if line.sideBySide != nil {
		for _, cell := range []*diffCell{line.sideBySide.old, line.sideBySide.new} {
			if cell != nil && cell.line != nil && cell.line.searchID == doc.ID {
				return true
			}
		}
	}
	return false
}
