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
type Model struct {
	Session                                           *review.Session
	Err                                               error
	Selected                                          int
	Scroll                                            map[int]int
	Width, Height, Horizontal                         int
	Inventory, Details, Evidence, Analysis, Help, URL bool
	Loading                                           bool
	Busy, Picker                                      bool
	Edit, Reorder                                     bool
	EditIndex                                         int
	PickerIndex                                       int
	Entries                                           []session.Entry
	ActionError                                       error
	store                                             *session.Store
	reader                                            review.MetadataReader
	fresh                                             FreshLoader
	worker                                            <-chan struct{}
	notice                                            string
	ctx                                               context.Context
	cancel                                            context.CancelFunc
	load                                              Loader
	notify                                            func(string)
}

func New(parent context.Context, load Loader) *Model {
	ctx, cancel := context.WithCancel(parent)
	return &Model{ctx: ctx, cancel: cancel, load: load, Scroll: map[int]int{}, Width: 100, Height: 24, Loading: true, notice: "Loading GitHub metadata and pinned committed objects..."}
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
	case ActionResult:
		m.Busy = false
		m.ActionError = v.Err
		if v.Err == nil && v.Session != nil {
			m.Session = v.Session
			if v.Reset {
				m.Selected, m.Horizontal = 0, 0
				m.Scroll = map[int]int{}
				m.Picker = false
				m.Help, m.URL = false, false
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
			if m.Edit || m.Reorder {
				if cmd := m.editKey(v.String()); cmd != nil {
					return m, cmd
				}
				return m, nil
			}
			if m.Picker {
				return m, m.pickerKey(v.String())
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
			m.Help = !m.Help
		case "g":
			m.URL = !m.URL
		case "i":
			m.Inventory = !m.Inventory
			m.Details = false
		case "e":
			m.Evidence = !m.Evidence
			m.Inventory, m.Details, m.Analysis = false, false, false
		case "a":
			m.Analysis = !m.Analysis
			m.Inventory, m.Details, m.Evidence = false, false, false
		case "v":
			m.beginMove()
		case "o":
			m.beginReorder()
		case "tab", "enter":
			m.Details = !m.Details
		case "n":
			m.move(1)
		case "p":
			m.move(-1)
		case "]":
			m.file(1)
		case "[":
			m.file(-1)
		case "down":
			if m.Details {
				m.scroll(1)
			} else {
				m.move(1)
			}
		case "up":
			if m.Details {
				m.scroll(-1)
			} else {
				m.move(-1)
			}
		case "j":
			m.scroll(1)
		case "k":
			m.scroll(-1)
		case "pgdown", "space":
			m.scroll(max(1, m.Height-6))
		case "pgup":
			m.scroll(-max(1, m.Height-6))
		case "right", "l":
			m.Horizontal += 8
		case "left", "h":
			m.Horizontal = max(0, m.Horizontal-8)
		case "home":
			m.Scroll[m.Selected] = 0
			m.Horizontal = 0
		case "esc":
			m.Edit, m.Reorder = false, false
		}
	}
	return m, nil
}
func (m *Model) move(delta int) {
	if m.Session != nil && len(m.Session.Inventory.Units) > 0 {
		m.Selected = max(0, min(len(m.Session.Inventory.Units)-1, m.Selected+delta))
		m.Horizontal = 0
	}
}
func (m *Model) file(delta int) {
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
	last := max(0, strings.Count(unitText(m.Session, m.Selected), "\n")-1)
	m.Scroll[m.Selected] = max(0, min(last, m.Scroll[m.Selected]+delta))
}
func (m *Model) View() tea.View {
	text := ""
	switch {
	case m.Loading:
		text = "Loading\n" + Escape(m.notice) + "\nq / ctrl+c: cancel"
	case m.Err != nil:
		text = "Unable to open review\n" + Escape(m.Err.Error()) + "\nNo complete comparison available. q: quit"
	case m.Session == nil:
		text = "No review loaded. q: quit"
	case m.Picker:
		text = m.pickerView()
	case m.Edit || m.Reorder:
		text = m.editView()
	case m.Help:
		text = "Keyboard\nn/p: next/previous unit | [/]: next/previous file\nup/down: move focused pane | tab/enter: switch focus/pane\nj/k: scroll diff | pgup/pgdown/space: page\nh/l or left/right: horizontal scroll | home: reset scroll\ni: full inventory | e: evidence scope | a: accepted plan | g: GitHub URL | ?: help | q: quit\nm: mark/unmark slice | v: move selected unit | o: reorder slices | r: refresh GitHub metadata\ns: saved sessions | N: new comparison, empty progress\nControls and invalid bytes escaped. No mouse capture.\nReading progress is local, not GitHub approval.\nEvidence is pinned, bounded, and omissions are reported. Analysis is optional and consent-bound."
		if m.store != nil {
			text += "\nStorage: " + Escape(m.store.Path()) + "\nSession: " + m.Session.ID
		}
	case m.URL:
		text = m.Session.Inventory.Comparison.Metadata.Identity.URL() + "\nOpen this URL in your browser for GitHub review actions.\ng: return | q: quit"
	default:
		if m.Analysis {
			text = m.analysisView()
		} else if m.Evidence {
			text = m.evidenceView()
		} else {
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
	title := status(s)
	if s.ID != "" {
		title += "\n" + progress(s)
	}
	if len(s.Inventory.Units) == 0 {
		return title + "\nEmpty comparison: no net tree changes.\n" + m.footer()
	}
	kind := s.Inventory.Units[m.Selected].Kind
	focus := "files"
	if m.Details {
		focus = "diff"
	}
	label := "File slices"
	if m.Inventory {
		label = "Full inventory"
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
	header := fmt.Sprintf("%s | focus: %s | unit %d/%d [%s] | unassigned %d | unavailable %d", label, focus, m.Selected+1, len(s.Inventory.Units), kind, unassigned, unavailable)
	bodyHeight := max(1, m.Height-4)
	if s.ID != "" {
		bodyHeight = max(1, m.Height-5)
	}
	list := []string{}
	selectedRow := 0
	if m.Inventory {
		selectedRow = m.Selected
		for i, u := range s.Inventory.Units {
			marker := "  "
			if i == m.Selected {
				marker = "> "
			}
			list = append(list, marker+pathLabel(s.Inventory.Files[s.UnitFiles[i]])+" ["+string(u.Kind)+"]")
		}
	} else {
		selectedRow = s.UnitFiles[m.Selected]
		for i, f := range s.Inventory.Files {
			marker := "  "
			if i == selectedRow {
				marker = "> "
			}
			list = append(list, marker+readMarker(s, f.ID)+pathLabel(f))
		}
	}
	start := max(0, selectedRow-bodyHeight+1)
	list = list[start:min(len(list), start+bodyHeight)]
	detail := unitLines(s, m.Selected)
	offset := min(m.Scroll[m.Selected], max(0, len(detail)-1))
	detail = detail[offset:min(len(detail), offset+bodyHeight)]
	// Horizontal scrolling stays on unstyled text; styles are applied after clipping.
	for i, line := range detail {
		runes := []rune(line.Text)
		detail[i].Text = string(runes[min(m.Horizontal, len(runes)):])
	}
	body := []string{}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		class := classPlain
		if row < len(list) {
			left = list[row]
		}
		if row < len(detail) {
			right, class = detail[row].Text, detail[row].Class
		}
		if m.Width < 100 {
			if m.Details {
				body = append(body, styleLine(class, clip(right, m.Width)))
			} else {
				body = append(body, clip(left, m.Width))
			}
		} else {
			leftWidth := min(36, m.Width/3)
			left = clip(left, leftWidth)
			body = append(body, left+strings.Repeat(" ", max(0, leftWidth-visibleWidth(left)))+" | "+styleLine(class, clip(right, m.Width-leftWidth-3)))
		}
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\n" + m.footer()
}
