package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/session"
)

type Loader func(context.Context, func(string)) (*review.Session, error)
type Loaded struct {
	Session *review.Session
	Err     error
}
type Notice string

type pane int

const (
	paneList pane = iota
	paneDiff
)

const diffStep = 5

type page int

const (
	pageReview page = iota
	pageEvidence
	pageAnalysis
	pageHelp
	pageURL
	pagePicker
	pageEdit
	pageReorder
)

type Model struct {
	Session                   *review.Session
	Err                       error
	Selected                  int
	Row                       int  // selected guide hierarchy row
	Files                     bool // G: navigate the deterministic file plan instead of guides
	collapsed                 expansion
	Scroll                    map[int]int
	GuideScroll               map[int]int
	Width, Height, Horizontal int
	Inventory                 bool
	Focus                     pane
	Stack                     []page
	Loading                   bool
	Busy                      bool
	EditIndex                 int
	PickerIndex               int
	Entries                   []session.Entry
	ActionError               error
	store                     *session.Store
	reader                    review.MetadataReader
	fresh                     FreshLoader
	worker                    <-chan struct{}
	notice                    string
	ctx                       context.Context
	cancel                    context.CancelFunc
	load                      Loader
	notify                    func(string)
}

func New(parent context.Context, load Loader) *Model {
	ctx, cancel := context.WithCancel(parent)
	return &Model{ctx: ctx, cancel: cancel, load: load, Scroll: map[int]int{}, GuideScroll: map[int]int{}, collapsed: newExpansion(), Stack: []page{pageReview}, Width: 100, Height: 24, Loading: true, notice: "Loading GitHub metadata and pinned committed objects..."}
}
func (m *Model) SetNotifier(f func(string)) { m.notify = f }
func (m *Model) Init() tea.Cmd {
	return m.start(func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, e := m.load(m.ctx, n)
		return Loaded{s, e}
	})
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case Loaded:
		m.Session = v.Session
		m.Err = v.Err
		m.Loading = false
		m.Busy = false
		m.begin()
	case ActionResult:
		m.Busy = false
		m.ActionError = v.Err
		if v.Err == nil && v.Session != nil {
			m.Session = v.Session
			if v.Reset {
				m.Selected, m.Horizontal, m.Row = 0, 0, 0
				m.collapsed = newExpansion()
				m.Scroll = map[int]int{}
				m.GuideScroll = map[int]int{}
				m.Stack = []page{pageReview}
				m.Focus = paneList
				m.begin()
			}
		}
	case Notice:
		m.notice = string(v)
	case tea.WindowSizeMsg:
		m.Width = max(1, v.Width)
		m.Height = max(1, v.Height)
	case tea.KeyPressMsg:
		if v.String() != "q" && v.String() != "ctrl+c" {
			if m.Busy {
				return m, nil
			}
			if p := m.top(); p != pageReview {
				return m, m.pageKey(p, v.String())
			}
			if cmd, handled := m.lifecycleKey(v.String()); handled {
				return m, cmd
			}
		}
		switch v.String() {
		case "q", "ctrl+c":
			m.cancel()
			return m, tea.Quit
		case "?":
			m.push(pageHelp)
		case "g":
			m.push(pageURL)
		case "i":
			m.Inventory = !m.Inventory
			m.Focus = paneList
			if !m.Inventory {
				m.syncRow(m.rows())
			}
		case "G":
			m.Files = !m.Files
			m.syncRow(m.rows())
		case "e":
			m.push(pageEvidence)
			m.Inventory, m.Focus = false, paneList
		case "a":
			m.push(pageAnalysis)
			m.Inventory, m.Focus = false, paneList
		case "v":
			m.beginMove()
		case "o":
			m.beginReorder()
		case "esc":
			m.back()
		case "ctrl+h":
			m.Focus = paneList
		case "ctrl+l", "enter":
			m.focusDetail(m.navigable())
		case "tab":
			// enter keeps main's meaning (focus the diff), so expansion gets
			// its own key rather than overloading one the reviewer already uses.
			if rows := m.navigable(); rows != nil {
				m.toggle(rows)
			}
		case "n":
			m.move(1)
		case "p":
			m.move(-1)
		case "]":
			m.file(1)
		case "[":
			m.file(-1)
		case "down":
			if m.Focus == paneDiff {
				m.scroll(1)
			} else {
				m.move(1)
			}
		case "up":
			if m.Focus == paneDiff {
				m.scroll(-1)
			} else {
				m.move(-1)
			}
		case "j":
			if m.Focus == paneDiff {
				m.scroll(1)
			} else {
				m.move(1)
			}
		case "k":
			if m.Focus == paneDiff {
				m.scroll(-1)
			} else {
				m.move(-1)
			}
		case "J":
			m.scroll(diffStep)
		case "K":
			m.scroll(-diffStep)
		case "pgdown":
			m.scroll(m.pageStep())
		case "pgup":
			m.scroll(-m.pageStep())
		case "right", "l":
			m.Horizontal += 8
		case "left", "h":
			m.Horizontal = max(0, m.Horizontal-8)
		case "home":
			m.setOffset(0)
			m.Horizontal = 0
		}
	}
	return m, nil
}

func (m *Model) top() page {
	if len(m.Stack) == 0 {
		return pageReview
	}
	return m.Stack[len(m.Stack)-1]
}
func (m *Model) push(p page) {
	if m.top() != p {
		m.Stack = append(m.Stack, p)
	}
}
func (m *Model) pop() {
	if len(m.Stack) > 1 {
		m.Stack = m.Stack[:len(m.Stack)-1]
	}
}
func (m *Model) back() {
	if len(m.Stack) > 1 {
		m.pop()
	} else if m.Focus == paneDiff {
		m.Focus = paneList
	}
}
func (m *Model) pageKey(p page, k string) tea.Cmd {
	switch p {
	case pagePicker:
		return m.pickerKey(k)
	case pageEdit, pageReorder:
		return m.editKey(k)
	case pageHelp, pageURL, pageEvidence, pageAnalysis:
		if k == "esc" {
			m.pop()
		}
	}
	return nil
}

// navigable returns the guide rows when they are the active hierarchy. The
// raw inventory view stays unit-by-unit, so `i` remains the complete source
// view rather than a second guide list.
func (m *Model) navigable() []row {
	if m.Inventory {
		return nil
	}
	rows := m.rows()
	if len(rows) == 0 {
		return nil
	}
	return rows
}
func (m *Model) move(delta int) {
	if rows := m.navigable(); rows != nil {
		m.moveRow(rows, delta)
		return
	}
	if m.Session != nil && len(m.Session.Inventory.Units) > 0 {
		m.Selected = max(0, min(len(m.Session.Inventory.Units)-1, m.Selected+delta))
		m.Horizontal = 0
	}
}
func (m *Model) file(delta int) {
	if rows := m.navigable(); rows != nil {
		m.moveGuide(rows, delta)
		return
	}
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	f := m.Session.UnitFiles[m.Selected]
	f = max(0, min(len(m.Session.Slices)-1, f+delta))
	m.Selected = m.Session.Slices[f].Units[0]
	m.Horizontal = 0
}
func (m *Model) scroll(delta int) {
	if m.Session == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	// The clamp counts the same display lines the diff pane renders, so a diff
	// that already fits cannot be scrolled past its end.
	m.setOffset(m.clampOffset(m.offset() + delta))
}

func (m *Model) clampOffset(offset int) int {
	last := max(0, len(m.detail())-m.bodyHeight())
	return max(0, min(last, offset))
}

// focusDetail preserves a guide row's saved reading position, while section
// and file rows jump to their corresponding guide-detail file occurrence.
func (m *Model) focusDetail(rows []row) {
	if len(rows) > 0 {
		r := rows[max(0, min(len(rows)-1, m.Row))]
		if offset, ok := anchorFor(detailFor(m.Session, r.guide), r); ok {
			m.setOffset(m.clampOffset(offset))
		}
	}
	m.Focus = paneDiff
}

func (m *Model) activeGuide() (int, bool) {
	if m.Inventory {
		return 0, false
	}
	rows := m.rows()
	if len(rows) == 0 {
		return 0, false
	}
	return rows[max(0, min(len(rows)-1, m.Row))].guide, true
}

func (m *Model) detail() []styledLine {
	if guide, ok := m.activeGuide(); ok {
		return detailFor(m.Session, guide).lines
	}
	if m.Session == nil || m.Selected < 0 || m.Selected >= len(m.Session.Inventory.Units) {
		return nil
	}
	return unitLines(m.Session, m.Selected)
}

func (m *Model) offset() int {
	if guide, ok := m.activeGuide(); ok {
		return m.GuideScroll[guide]
	}
	return m.Scroll[m.Selected]
}

func (m *Model) setOffset(offset int) {
	if guide, ok := m.activeGuide(); ok {
		m.GuideScroll[guide] = offset
		return
	}
	m.Scroll[m.Selected] = offset
}

func (m *Model) bodyHeight() int {
	n := m.Height - 4
	if m.Session != nil && m.Session.ID != "" {
		n = m.Height - 5
	}
	return max(1, n)
}

func (m *Model) pageStep() int { return max(1, m.bodyHeight()-1) }
func (m *Model) View() tea.View {
	text := ""
	switch {
	case m.Loading:
		text = "Loading\n" + Escape(m.notice) + "\nq / ctrl+c: cancel"
	case m.Err != nil:
		text = "Unable to open review\n" + Escape(m.Err.Error()) + "\nNo complete comparison available. q: quit"
	case m.Session == nil:
		text = "No review loaded. q: quit"
	default:
		switch m.top() {
		case pagePicker:
			text = m.pickerView()
		case pageEdit, pageReorder:
			text = m.editView()
		case pageHelp:
			text = "Keyboard\n" + renderBindings(groupHelp) + "\nControls and invalid bytes escaped. No mouse capture.\nReading progress is local, not GitHub approval.\nGuides interpret the diff; the raw inventory remains the complete source view.\nMarking any portion of a file marks its whole slice, under every guide.\nEvidence is pinned, bounded, and omissions are reported. Analysis is optional and consent-bound."
			if m.store != nil {
				text += "\nStorage: " + Escape(m.store.Path()) + "\nSession: " + m.Session.ID
			}
		case pageURL:
			text = m.Session.Inventory.Comparison.Metadata.Identity.URL() + "\nOpen this URL in your browser for GitHub review actions.\nesc: back | q: quit"
		case pageAnalysis:
			text = m.analysisView()
		case pageEvidence:
			text = m.evidenceView()
		default:
			text = m.reviewView()
		}
	}
	lines := strings.Split(text, "\n")
	if len(lines) > m.Height {
		lines = lines[:m.Height]
	}
	for i := range lines {
		lines[i] = clip(lines[i], m.Width)
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
func (m *Model) reviewView() string {
	s := m.Session
	title := styleLine(statusClass(s), status(s))
	if s.ID != "" {
		title += "\n" + styleLine(progressClass(s), progress(s))
	}
	if len(s.Inventory.Units) == 0 {
		return title + "\nEmpty comparison: no net tree changes.\n" + m.styledFooter()
	}
	kind := s.Inventory.Units[m.Selected].Kind
	focus := "files"
	if m.Focus == paneDiff {
		focus = "diff"
	}
	label := "File slices"
	if m.Inventory {
		label = "Full inventory"
	}
	rows := m.navigable()
	if rows != nil {
		label = "Guides"
	}
	plan := s.CurrentPlan()
	unassigned, unavailable := 0, 0
	if plan != nil {
		unassigned = len(plan.UnassignedUnitIDs)
	}
	for _, u := range s.Inventory.Units {
		if u.Kind == "unavailable" {
			unavailable++
		}
	}
	headerClass := classTitle
	if unavailable > 0 {
		headerClass = classWarning
	}
	text := fmt.Sprintf("%s | focus: %s | unit %d/%d [%s] | unassigned %d | unavailable %d", label, focus, m.Selected+1, len(s.Inventory.Units), kind, unassigned, unavailable)
	if rows != nil {
		// The hierarchy interprets the change; progress does not follow it, and
		// saying so here keeps a section from looking independently completable.
		text += " | m marks the whole file slice"
		guide, _ := m.activeGuide()
		text += fmt.Sprintf(" | guide %d/%d", guide+1, len(s.Guides.Items))
	}
	header := styleLine(headerClass, text)
	bodyHeight := m.bodyHeight()
	leftWidth := m.Width
	if m.Width >= 100 {
		leftWidth = min(36, m.Width/3)
	}
	list := []listLine{}
	selectedRow := 0
	if m.Inventory {
		selectedRow = m.Selected
		for i, u := range s.Inventory.Units {
			marker := "  "
			if i == m.Selected {
				marker = "· "
				if m.Focus == paneList {
					marker = "> "
				}
			}
			list = append(list, listLine{row: i, text: marker + pathLabel(s.Inventory.Files[s.UnitFiles[i]]) + " [" + string(u.Kind) + "]"})
		}
	} else if rows != nil {
		selectedRow = max(0, min(len(rows)-1, m.Row))
		list = guideList(s, rows, selectedRow, leftWidth, m.Focus == paneList)
	} else {
		selectedRow = s.UnitFiles[m.Selected]
		for i, f := range s.Inventory.Files {
			marker := "  "
			if i == selectedRow {
				marker = "· "
				if m.Focus == paneList {
					marker = "> "
				}
			}
			list = append(list, listLine{row: i, text: marker + readMarker(s, f.ID) + pathLabel(f)})
		}
	}
	start := max(0, firstDisplayLine(list, selectedRow)-bodyHeight+1)
	list = list[start:min(len(list), start+bodyHeight)]
	detail := m.detail()
	offset := min(m.offset(), max(0, len(detail)-1))
	detail = detail[offset:min(len(detail), offset+bodyHeight)]
	// Horizontal scrolling stays on unstyled text; styles are applied after clipping.
	for i, line := range detail {
		runes := []rune(line.Text)
		detail[i].Text = string(runes[min(m.Horizontal, len(runes)):])
	}
	body := []string{}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		class, leftClass := classPlain, classPlain
		if row < len(list) {
			left = list[row].text
			if list[row].row == selectedRow {
				// The focused pane's selection gets the stronger cue; the text
				// markers "> " and "· " already distinguish the two states.
				leftClass = classSelection
				if m.Focus == paneList {
					leftClass = classSelectionFocused
				}
			}
		}
		if row < len(detail) {
			right, class = detail[row].Text, detail[row].Class
		}
		if m.Width < 100 {
			if m.Focus == paneDiff {
				body = append(body, styleLine(class, clip(right, m.Width)))
			} else {
				body = append(body, styleLine(leftClass, clip(left, m.Width)))
			}
		} else {
			leftWidth := min(36, m.Width/3)
			left = clip(left, leftWidth)
			// Padding is measured on the clipped plain row, then the row is styled.
			padding := strings.Repeat(" ", max(0, leftWidth-visibleWidth(left)))
			body = append(body, styleLine(leftClass, left)+padding+" | "+styleLine(class, clip(right, m.Width-leftWidth-3)))
		}
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.styledFooter()
}

// styledFooter paints the footer as chrome, or as a warning when the last
// action failed. Its wording is identical to footer().
func (m *Model) styledFooter() string {
	if m.ActionError != nil {
		return styleLine(classWarning, m.footer())
	}
	return styleLine(classTitle, m.footer())
}
