package tui

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"prui/internal/commits"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
)

// Filtering owns transient reading state. Canonical source, drafts, progress and
// saved reading positions remain in the review tab, untouched by this surface.
type commitFilterState struct {
	open, subset, computing                 bool
	selected                                map[string]bool
	focus, offset, file, scroll, horizontal int
	generation                              uint64
	readingFocus                            pane
	railOffset                              int
	previousPath                            string
	inventory                               inventory.Inventory
	err                                     error
	cancel                                  context.CancelFunc
	guideSource                             *review.Session
	guideOwners                             map[string]map[int]bool
	sourceCache                             commitFilterSourceCache
}
type commitFilterResult struct {
	state      *reviewTabState
	session    *review.Session
	generation uint64
	inventory  inventory.Inventory
	err        error
}

func (m *Model) commitFilterLabel() string {
	if m.commitFilter.subset {
		return fmt.Sprintf("%d commits", len(m.commitFilter.selected))
	}
	return "All commits"
}
func (m *Model) openCommitFilter() {
	m.commitFilter.open = true
	m.commitFilter.focus = 0
	for i, e := range m.commitEntries() {
		if m.commitFilter.selected[e.SHA] {
			m.commitFilter.focus = i + 1
			break
		}
	}
}
func (m *Model) commitFilterKey(k string) tea.Cmd {
	f := &m.commitFilter
	switch k {
	case "esc", "C":
		f.open = false
	case "j", "down":
		f.focus = min(len(m.commitEntries()), f.focus+1)
	case "k", "up":
		f.focus = max(0, f.focus-1)
	case "home":
		f.focus = 0
	case "end":
		f.focus = len(m.commitEntries())
	case "enter", "space", " ":
		return m.toggleCommitFilter()
	}
	return nil
}
func (m *Model) toggleCommitFilter() tea.Cmd {
	f := &m.commitFilter
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.generation++
	f.computing = false
	if f.file >= 0 && f.file < len(f.inventory.Files) {
		f.previousPath = commitFilterPath(f.inventory.Files[f.file])
	}
	f.inventory = inventory.Inventory{}
	f.err = nil
	if f.focus == 0 {
		f.subset = false
		f.selected = nil
		return nil
	}
	entries := m.commitEntries()
	if f.focus > len(entries) {
		return nil
	}
	if !f.subset {
		f.selected = map[string]bool{}
	}
	f.subset = true
	sha := entries[f.focus-1].SHA
	if f.selected[sha] {
		delete(f.selected, sha)
	} else {
		f.selected[sha] = true
	}
	if m.Session.Commits.Complete && len(f.selected) == len(entries) {
		f.subset = false
		f.selected = nil
		return nil
	}
	if len(f.selected) == 0 {
		return nil
	}
	selected := make([]string, 0, len(f.selected))
	for _, e := range entries {
		if f.selected[e.SHA] {
			selected = append(selected, e.SHA)
		}
	}
	f.computing = true
	ctx, cancel := context.WithTimeout(m.ctx, source.Defaults().Operation)
	f.cancel = cancel
	state, session, generation := m.reviewTabState, m.Session, f.generation
	bundle := m.Session.Commits
	return func() tea.Msg {
		defer cancel()
		inv, err := commits.Compose(ctx, bundle, selected, source.Defaults())
		return commitFilterResult{state: state, session: session, generation: generation, inventory: inv, err: err}
	}
}
func (m *Model) applyCommitFilterResult(r commitFilterResult) {
	if r.state == nil || r.state.Session != r.session || r.state.commitFilter.generation != r.generation || !r.state.commitFilter.subset {
		return
	}
	f := &r.state.commitFilter
	f.computing = false
	f.cancel = nil
	f.err = r.err
	if r.err != nil {
		f.inventory = inventory.Inventory{}
		return
	}
	previous := f.previousPath
	f.inventory = r.inventory
	f.file = 0
	for i, file := range r.inventory.Files {
		if commitFilterPath(file) == previous {
			f.file = i
			break
		}
	}
	f.railOffset = 0
	f.scroll = max(0, f.scroll)
}
func (m *Model) commitFilterPickerBordered() bool { return m.Width >= 12 && m.Height >= 6 }
func (m *Model) commitFilterPickerHeight() int {
	if m.commitFilterPickerBordered() {
		return max(1, m.Height-7)
	}
	return max(1, m.Height-2)
}

// Rendering and hit testing share the card geometry, including short terminals.
func (m *Model) commitFilterPickerBounds() (card, choices image.Rectangle) {
	rows := len(m.commitFilterPickerRows())
	width := max(1, min(76, m.Width-4))
	height, rowStart, padding := rows+2, 1, 0
	if m.commitFilterPickerBordered() {
		height, rowStart, padding = rows+5, 3, 2
	}
	height = min(height, m.Height)
	left, top := max(0, (m.Width-width)/2), max(0, (m.Height-height)/2)
	card = image.Rect(left, top, left+width, top+height)
	choices = image.Rect(left+padding, top+rowStart, left+width-padding, min(top+rowStart+rows, card.Max.Y))
	return card, choices
}
func (m *Model) commitFilterPickerRows() []string {
	f := &m.commitFilter
	count := len(m.commitEntries()) + 1
	height := m.commitFilterPickerHeight()
	if f.focus < f.offset {
		f.offset = f.focus
	}
	if f.focus >= f.offset+height {
		f.offset = f.focus - height + 1
	}
	f.offset = max(0, min(f.offset, max(0, count-height)))
	var rows []string
	for i := f.offset; i < min(count, f.offset+height); i++ {
		checked := !f.subset
		label := "All commits"
		if i > 0 {
			e := m.commitEntries()[i-1]
			checked = f.subset && f.selected[e.SHA]
			label = shortCommitSHA(e.SHA) + " " + commitSubject(e) + " · " + Escape(e.Author)
		}
		box := "[ ] "
		if checked {
			box = "[x] "
		}
		rows = append(rows, selectionMarker(i == f.focus)+box+label)
	}
	return rows
}
func (m *Model) commitFilterPickerView() string {
	rows := m.commitFilterPickerRows()
	text := "Commits [C] · " + m.commitFilterLabel() + "\n"
	if !m.commitFilterPickerBordered() {
		return text + strings.Join(rows, "\n") + "\nEsc/C: close"
	}
	switch {
	case len(m.commitEntries()) == 0:
		text += m.commitSafeState() + "\n"
	case !m.Session.Commits.Complete:
		text += "Incomplete captured list; additional commits may exist.\n"
	default:
		text += "Select commits for one net diff.\n"
	}
	footer := "Space/Enter: toggle · j/k: move · Esc/C: close"
	if m.Width < 54 {
		footer = "Space: toggle · Esc/C: close"
	}
	text += strings.Join(rows, "\n") + "\n" + footer
	return text
}

func (m *Model) commitFilterModalView(background string) string {
	card, _ := m.commitFilterPickerBounds()
	body := m.commitFilterPickerView()
	var rows []string
	if m.commitFilterPickerBordered() {
		inside := max(1, card.Dx()-4)
		rows = append(rows, "╭"+strings.Repeat("─", card.Dx()-2)+"╮")
		for _, line := range strings.Split(body, "\n") {
			rows = append(rows, modalLine(ansi.Truncate(line, inside, "…"), inside))
		}
		rows = append(rows, "╰"+strings.Repeat("─", card.Dx()-2)+"╯")
	} else {
		bodyRows := strings.Split(body, "\n")
		for _, line := range bodyRows[:min(len(bodyRows), card.Dy())] {
			rows = append(rows, ansi.Truncate(line, card.Dx(), "…")+strings.Repeat(" ", max(0, card.Dx()-visibleWidth(line))))
		}
	}
	return lipgloss.NewCanvas(m.Width, m.Height).Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(strings.Join(viewportLines(background, m.Width, m.Height), "\n")),
		lipgloss.NewLayer(m.modalSurface(strings.Join(rows, "\n"))).X(card.Min.X).Y(card.Min.Y),
	)).Render()
}
func (m *Model) filteredReadingKey(k string) bool {
	f := &m.commitFilter
	switch k {
	case "h", "ctrl+h", "esc":
		f.readingFocus = paneList
	case "l", "ctrl+l", "enter":
		f.readingFocus = paneDiff
	case "]":
		m.resizeList(2)
	case "[":
		m.resizeList(-2)
	case "S":
		m.layout = m.layout.toggled()
	case "j", "down", "n", "k", "up", "p", "d", "pgdown", "u", "pgup", "home", "end":
		delta := 1
		if k == "k" || k == "up" || k == "p" {
			delta = -1
		}
		if k == "d" || k == "pgdown" {
			delta = m.bodyHeight()
		}
		if k == "u" || k == "pgup" {
			delta = -m.bodyHeight()
		}
		if k == "home" {
			delta = -int(^uint(0) >> 2)
		}
		if k == "end" {
			delta = int(^uint(0) >> 2)
		}
		if f.readingFocus == paneList {
			m.moveFilteredFile(delta)
			f.scroll = 0
		} else {
			f.scroll = max(0, min(f.scroll+delta, max(0, len(m.filteredRows())-m.bodyHeight())))
		}
	case "right":
		f.horizontal += 8
	case "left":
		f.horizontal = max(0, f.horizontal-8)
	case "m", "e", "i", "tab", "{", "}", "z", "J", "K": // Read-only: no canonical mark, evidence or composition targets.
	default:
		return false
	}
	return true
}
func (m *Model) filteredRows() []diffLine {
	f := &m.commitFilter
	if f.computing {
		return body(classMetadata, "Computing selected commits...")
	}
	if f.err != nil {
		return body(classWarning, "Selected commits unavailable: "+Escape(f.err.Error()))
	}
	if len(f.selected) == 0 {
		return body(classMetadata, "No commits selected")
	}
	if len(f.inventory.Files) == 0 {
		return body(classMetadata, "No net changes in selected commits")
	}
	split := m.diffLayout() == diffLayoutSideBySide && m.Width >= sideBySideMinimumWidth
	c := &f.sourceCache
	if c.rows != nil && c.generation == f.generation && c.file == f.file && c.split == split && c.view == m.selectedReviewView() && c.width == m.detailWidth() {
		return c.rows
	}
	file := f.inventory.Files[max(0, min(f.file, len(f.inventory.Files)-1))]
	d := commits.Diff{Files: []inventory.FileChange{file}, Patches: f.inventory.Patches, Syntax: f.inventory.Syntax, Complete: f.inventory.Complete}
	for _, u := range f.inventory.Units {
		if u.FileChangeID == file.ID {
			d.Units = append(d.Units, u)
		}
	}
	rows := commitDiffRows(&d)
	if m.selectedReviewView() == viewGuide {
		rows = append(m.filteredGuideExplanation(file), rows...)
	}
	for _, e := range m.commitEntries() {
		if f.selected[e.SHA] && len(e.Parents) > 1 {
			rows = append(body(classMetadata, "Selected merge changes are relative to first parent."), rows...)
			break
		}
	}
	if split {
		rows = projectSideBySideDetail(rows)
	}
	*c = commitFilterSourceCache{generation: f.generation, file: f.file, split: split, view: m.selectedReviewView(), width: m.detailWidth(), rows: rows}
	return rows
}
func (m *Model) filteredReviewView(title string) string {
	f := &m.commitFilter
	label := ""
	if len(f.inventory.Files) > 0 {
		label = fmt.Sprintf("file %d/%d", f.file+1, len(f.inventory.Files))
	}
	if m.selectedReviewView() == viewGuide {
		label = "Guide · full PR interpretation"
	}
	frame := *m
	frameState := *m.reviewTabState
	frame.reviewTabState = &frameState
	frame.Focus = f.readingFocus
	rows := m.filteredRows()
	f.scroll = max(0, min(f.scroll, max(0, len(rows)-m.bodyHeight())))
	rows = append([]diffLine(nil), rows[f.scroll:min(len(rows), f.scroll+m.bodyHeight())]...)
	split := m.diffLayout() == diffLayoutSideBySide && m.Width >= sideBySideMinimumWidth
	if split {
		rows = m.renderSideBySideViewport(rows, m.detailWidth(), f.horizontal, -1, nil)
	}
	list := m.filteredList()
	selectedLine := firstDisplayLine(list, f.file)
	if selectedLine < f.railOffset {
		f.railOffset = selectedLine
	}
	if selectedLine >= f.railOffset+m.bodyHeight() {
		f.railOffset = selectedLine - m.bodyHeight() + 1
	}
	f.railOffset = max(0, min(f.railOffset, max(0, len(list)-m.bodyHeight())))
	var lines []string
	for i := 0; i < m.bodyHeight(); i++ {
		left, right := "", ""
		lc, rc := classPlain, classPlain
		index := i + f.railOffset
		if index < len(list) {
			left = list[index].text
			if list[index].row == f.file {
				lc = selectedClass(f.readingFocus == paneList)
			}
		}
		if i < len(rows) {
			right = rows[i].Text
			rc = rows[i].Class
			if !split {
				presented := m.presentUnifiedDiffLine(rows[i], f.horizontal, m.detailWidth(), "")
				right, rc = presented.Text, presented.Class
			}
		}
		lines = append(lines, m.paneBodyRow(styledLine{Class: lc, Text: left}, styledLine{Class: rc, Text: right}, m.listWidth(), m.detailWidth(), f.readingFocus, paneBodyBorders{classPaneBorder, classPaneBorder, classPaneBorder}))
	}
	footer := "Selected commits: reading only; use All commits for source/search or marking files.\nR: review PR · C: filter commits"
	if m.Height < 10 {
		footer = "Reading only · C: commits · R: review PR"
	}
	return title + "\n" + frame.reviewPaneHeader(label) + "\n" + strings.Join(lines, "\n") + "\n" + frame.paneFrameFooter() + "\n" + footer
}
func (m *Model) commitFilterMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	event := msg.Mouse()
	if event.X < 0 || event.Y < 0 || event.X >= m.Width || event.Y >= m.Height {
		return nil, false
	}
	if m.top() != pageReview || !m.diffReviewView() {
		return nil, false
	}
	f := &m.commitFilter
	if f.open {
		_, choices := m.commitFilterPickerBounds()
		if w, ok := msg.(tea.MouseWheelMsg); ok {
			if !image.Pt(w.X, w.Y).In(choices) {
				return nil, true
			}
			delta := 1
			if w.Button == tea.MouseWheelUp {
				delta = -1
			}
			f.focus = max(0, min(len(m.commitEntries()), f.focus+delta))
			return nil, true
		}
		if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
			i := c.Y - choices.Min.Y + f.offset
			if image.Pt(c.X, c.Y).In(choices) && i <= len(m.commitEntries()) && i >= 0 {
				f.focus = i
				return m.toggleCommitFilter(), true
			}
		}
		return nil, true
	}
	if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft && c.Y == 2 && m.commitFilterControlContains(c.X) {
		m.openCommitFilter()
		return nil, true
	}
	if !f.subset {
		return nil, false
	}
	if c, ok := msg.(tea.MouseClickMsg); ok && c.Y == 1 {
		return nil, false
	}
	if cmd, handled := m.mouseResize(msg); handled {
		return cmd, true
	}
	g := m.filteredGeometry()
	if w, ok := msg.(tea.MouseWheelMsg); ok {
		delta := mouseWheelStep
		if w.Button == tea.MouseWheelUp {
			delta = -delta
		}
		if image.Pt(w.X, w.Y).In(g.Rail) {
			m.moveFilteredFile(delta)
			f.scroll = 0
		} else if image.Pt(w.X, w.Y).In(g.Detail) {
			f.scroll = max(0, min(f.scroll+delta, max(0, len(m.filteredRows())-m.bodyHeight())))
		}
	}
	if c, ok := msg.(tea.MouseClickMsg); ok && c.Button == tea.MouseLeft {
		if image.Pt(c.X, c.Y).In(g.Rail) {
			list := m.filteredList()
			index := c.Y - g.Rail.Min.Y + f.railOffset
			if index < 0 || index >= len(list) || list[index].row < 0 {
				return nil, true
			}
			f.file = list[index].row
			f.scroll = 0
			f.readingFocus = paneList
		} else if image.Pt(c.X, c.Y).In(g.Detail) {
			f.readingFocus = paneDiff
		}
	}
	return nil, true
}

// Original validated unit-to-file references identify at most one guide. Rename
// and repeated ownership are ambiguous and stay in the explicit outside group.
func (m *Model) filteredGuideLabel(file inventory.FileChange) string {
	s := m.Session
	f := &m.commitFilter
	if s.Guides == nil || s.Guides.Status != guide.Generated {
		return "Guide absent; selected commit changes"
	}
	if f.guideSource != s {
		f.guideSource = s
		f.guideOwners = map[string]map[int]bool{}
		paths := map[string]string{}
		for ui, u := range s.Inventory.Units {
			if ui >= len(s.UnitFiles) {
				continue
			}
			fi := s.UnitFiles[ui]
			if fi < 0 || fi >= len(s.Inventory.Files) {
				continue
			}
			original := s.Inventory.Files[fi]
			if string(original.OldPath) != string(original.NewPath) && len(original.OldPath) > 0 && len(original.NewPath) > 0 {
				continue
			}
			path := string(original.NewPath)
			if path == "" {
				path = string(original.OldPath)
			}
			paths[u.ID] = path
		}
		for gi, item := range s.Guides.Items {
			for _, section := range item.Sections {
				for _, id := range section.UnitIDs {
					if path, ok := paths[id]; ok {
						if f.guideOwners[path] == nil {
							f.guideOwners[path] = map[int]bool{}
						}
						f.guideOwners[path][gi] = true
					}
				}
			}
		}
	}
	path := string(file.NewPath)
	if path == "" {
		path = string(file.OldPath)
	}
	if len(file.OldPath) > 0 && len(file.NewPath) > 0 && string(file.OldPath) != string(file.NewPath) {
		return "Selected commit changes outside guide"
	}
	owners := f.guideOwners[path]
	if len(owners) != 1 {
		return "Selected commit changes outside guide"
	}
	for gi := range owners {
		return "Full PR guide: " + Escape(s.Guides.Items[gi].Title)
	}
	return "Selected commit changes outside guide"
}
func (m *Model) filteredList() []listLine {
	files := m.commitFilter.inventory.Files
	if len(files) == 0 {
		message := m.filteredRows()
		if len(message) > 0 {
			return []listLine{{row: -1, text: message[0].Text}}
		}
	}
	var rows []listLine
	if m.selectedReviewView() != viewGuide {
		for i, f := range files {
			rows = append(rows, listLine{row: i, text: selectionMarker(i == m.commitFilter.file) + pathLabel(f)})
		}
		return rows
	}
	var order []string
	grouped := map[string][]int{}
	for i, f := range files {
		label := m.filteredGuideLabel(f)
		if _, ok := grouped[label]; !ok {
			order = append(order, label)
		}
		grouped[label] = append(grouped[label], i)
	}
	for _, label := range order {
		rows = append(rows, listLine{row: -1, text: label})
		for _, i := range grouped[label] {
			rows = append(rows, listLine{row: i, text: selectionMarker(i == m.commitFilter.file) + "  " + pathLabel(files[i])})
		}
	}
	return rows
}

func (m *Model) filteredGeometry() workspaceGeometry {
	screen := image.Rect(0, 0, m.Width, m.Height)
	end := 3 + m.bodyHeight()
	if m.Width < 100 {
		r := image.Rect(1, 3, m.Width-1, end).Intersect(screen)
		if m.commitFilter.readingFocus == paneDiff {
			return workspaceGeometry{Detail: r}
		}
		return workspaceGeometry{Rail: r}
	}
	left := m.listWidth()
	return workspaceGeometry{Rail: image.Rect(1, 3, left+1, end).Intersect(screen), Divider: image.Rect(left+1, 3, left+2, end).Intersect(screen), Detail: image.Rect(left+2, 3, m.Width-1, end).Intersect(screen)}
}

type commitFilterSourceCache struct {
	generation uint64
	file       int
	split      bool
	view       reviewView
	width      int
	rows       []diffLine
}

func (m *Model) moveFilteredFile(delta int) {
	f := &m.commitFilter
	list := m.filteredList()
	var order []int
	current := 0
	for _, r := range list {
		if r.row < 0 {
			continue
		}
		if r.row == f.file {
			current = len(order)
		}
		order = append(order, r.row)
	}
	if len(order) > 0 {
		f.file = order[max(0, min(current+delta, len(order)-1))]
	}
	f.scroll = 0
}
func (m *Model) filteredGuideExplanation(file inventory.FileChange) []diffLine {
	label := m.filteredGuideLabel(file)
	rows := body(classMetadata, label+" · original full PR interpretation")
	path := string(file.NewPath)
	if path == "" {
		path = string(file.OldPath)
	}
	owners := m.commitFilter.guideOwners[path]
	if len(owners) != 1 || !strings.HasPrefix(label, "Full PR guide:") {
		return rows
	}
	for gi := range owners {
		item := m.Session.Guides.Items[gi]
		if item.Description != "" {
			for _, text := range wrap(Escape(item.Description), m.detailWidth()) {
				rows = append(rows, body(classMetadata, text)...)
			}
		}
		for _, section := range item.Sections {
			matched := false
			for _, id := range section.UnitIDs {
				for ui, u := range m.Session.Inventory.Units {
					if id != u.ID || ui >= len(m.Session.UnitFiles) {
						continue
					}
					fi := m.Session.UnitFiles[ui]
					if fi < 0 || fi >= len(m.Session.Inventory.Files) {
						continue
					}
					original := m.Session.Inventory.Files[fi]
					if string(original.NewPath) == path || string(original.OldPath) == path {
						matched = true
					}
				}
			}
			if matched {
				rows = append(rows, body(classTitle, Escape(section.Title))...)
				if section.Description != "" {
					for _, text := range wrap(Escape(section.Description), m.detailWidth()) {
						rows = append(rows, body(classMetadata, text)...)
					}
				}
			}
		}
	}
	return rows
}

func (m *Model) commitFilterControlContains(x int) bool {
	start := 2
	empty := !m.commitFilter.subset && len(m.Session.Inventory.Units) == 0
	switch {
	case empty:
		start = 0
	case m.Width >= 100:
		start = m.listWidth() + 3
	default:
		label, _ := m.fileFilterHeader()
		if m.selectedReviewView() == viewGuide {
			label = "Guide"
		}
		if m.Inventory {
			label = "Full inventory (i)"
		}
		start += visibleWidth(label + " · ")
	}
	end := min(start+visibleWidth("Commits [C] · "+m.commitFilterLabel()), m.Width-1)
	return x >= start && x < end
}
func (m *Model) resetCommitFilter() {
	f := &m.commitFilter
	if f.cancel != nil {
		f.cancel()
	}
	generation := f.generation + 1
	*f = commitFilterState{generation: generation}
}

func commitFilterPath(file inventory.FileChange) string {
	if len(file.NewPath) > 0 {
		return string(file.NewPath)
	}
	return string(file.OldPath)
}
