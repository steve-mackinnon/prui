package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
)

type Loader func(context.Context, func(string)) (*review.Session, error)
type Loaded struct {
	Session *review.Session
	Err     error
}
type Notice string
type Model struct {
	Session                       *review.Session
	Err                           error
	Selected                      int
	Scroll                        map[int]int
	Width, Height, Horizontal     int
	Inventory, Details, Help, URL bool
	Loading                       bool
	notice                        string
	ctx                           context.Context
	cancel                        context.CancelFunc
	load                          Loader
	notify                        func(string)
}

func New(parent context.Context, load Loader) *Model {
	ctx, cancel := context.WithCancel(parent)
	return &Model{ctx: ctx, cancel: cancel, load: load, Scroll: map[int]int{}, Width: 100, Height: 24, Loading: true, notice: "Loading GitHub metadata and pinned committed objects..."}
}
func (m *Model) SetNotifier(f func(string)) { m.notify = f }
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, e := m.load(m.ctx, n)
		return Loaded{s, e}
	}
}
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case Loaded:
		m.Session = v.Session
		m.Err = v.Err
		m.Loading = false
	case Notice:
		m.notice = string(v)
	case tea.WindowSizeMsg:
		m.Width = max(1, v.Width)
		m.Height = max(1, v.Height)
	case tea.KeyPressMsg:
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
	case m.Help:
		text = "Keyboard\nn/p: next/previous unit | [/]: next/previous file\nup/down: move focused pane | tab/enter: switch focus/pane\nj/k: scroll diff | pgup/pgdown/space: page\nh/l or left/right: horizontal scroll | home: reset scroll\ni: full inventory | g: GitHub URL | ?: help | q: quit\nControls and invalid bytes escaped. No mouse capture.\nReading is not GitHub approval; no progress saved in Phase 1."
	case m.URL:
		text = m.Session.Inventory.Comparison.Metadata.Identity.URL() + "\nOpen this URL in your browser for GitHub review actions.\ng: return | q: quit"
	default:
		text = m.reviewView()
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
	if len(s.Inventory.Units) == 0 {
		return title + "\nEmpty comparison: no net tree changes.\nq: quit | g: GitHub URL"
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
	header := fmt.Sprintf("%s | focus: %s | unit %d/%d [%s]", label, focus, m.Selected+1, len(s.Inventory.Units), kind)
	bodyHeight := max(1, m.Height-4)
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
			list = append(list, marker+pathLabel(f))
		}
	}
	start := max(0, selectedRow-bodyHeight+1)
	list = list[start:min(len(list), start+bodyHeight)]
	detail := strings.Split(unitText(s, m.Selected), "\n")
	offset := min(m.Scroll[m.Selected], max(0, len(detail)-1))
	detail = detail[offset:min(len(detail), offset+bodyHeight)]
	for i, line := range detail {
		runes := []rune(line)
		detail[i] = string(runes[min(m.Horizontal, len(runes)):])
	}
	body := []string{}
	for row := 0; row < bodyHeight; row++ {
		left, right := "", ""
		if row < len(list) {
			left = list[row]
		}
		if row < len(detail) {
			right = detail[row]
		}
		if m.Width < 100 {
			if m.Details {
				body = append(body, clip(right, m.Width))
			} else {
				body = append(body, clip(left, m.Width))
			}
		} else {
			leftWidth := min(36, m.Width/3)
			left = clip(left, leftWidth)
			body = append(body, left+strings.Repeat(" ", max(0, leftWidth-visibleWidth(left)))+" | "+clip(right, m.Width-leftWidth-3))
		}
	}
	return title + "\n" + header + "\n" + strings.Join(body, "\n") + "\nn/p unit  [/] file  tab pane  j/k scroll  i inventory  g URL  ? help  q quit"
}
