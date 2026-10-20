package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// pageMouseControl is trusted UI chrome. Never derive actions from source text.
type pageMouseControl struct{ label, key string }

func pageMouseFooter(controls []pageMouseControl, separator string) string {
	labels := make([]string, len(controls))
	for i, c := range controls {
		labels[i] = c.label
	}
	return strings.Join(labels, separator)
}
func pageMouseFooterKey(controls []pageMouseControl, separator string, x, width int) string {
	offset := 0
	for _, c := range controls {
		end := offset + ansi.StringWidth(c.label)
		if x >= offset && x < end && end <= width {
			return c.key
		}
		offset = end + ansi.StringWidth(separator)
	}
	return ""
}

var inboxMouseControls = []pageMouseControl{{"enter: open", "enter"}, {"m: mark read", "m"}, {"f: filters", "f"}, {"r: refresh", "r"}, {"esc: back", "esc"}}
var inboxFilterMouseControls = []pageMouseControl{{"tab: field", "tab"}, {"left/right: cycle", "right"}, {"type repo/author", ""}, {"enter: use cache", "enter"}, {"r in inbox: refresh", ""}, {"esc: discard", "esc"}}
var incrementalMouseControls = []pageMouseControl{{"n/p: file", "n"}, {"↑↓/PgUp/PgDn: scroll", "down"}, {"←→: pan", "right"}, {"d: drafts/diff", "d"}, {"o: old", "o"}, {"esc: back", "esc"}}
var lifecycleMouseControls = []pageMouseControl{{"↑/↓: select", "down"}, {"enter: review/confirm", "enter"}, {"r: refresh", "r"}, {"esc: back", "esc"}}

// mousePage consumes all pointer input on the owning auxiliary page, including
// blank space, so it cannot select or activate the obscured review underneath.
func (m *Model) mousePage(msg tea.MouseMsg) (tea.Cmd, bool) {
	switch m.top() {
	case pageInbox, pageInboxFilters, pageActions, pageHelp, pageEvidence, pageURL, pageIncremental, pageLifecycle, pageReadiness, pageIssueContext:
	default:
		return nil, false
	}
	e := msg.Mouse()
	if e.Mod != 0 || e.X < 0 || e.Y < 0 || e.X >= m.Width || e.Y >= m.Height {
		return nil, true
	}
	if wheel, ok := msg.(tea.MouseWheelMsg); ok {
		key := ""
		switch wheel.Button {
		case tea.MouseWheelUp:
			key = "up"
		case tea.MouseWheelDown:
			key = "down"
		case tea.MouseWheelLeft:
			key = "left"
		case tea.MouseWheelRight:
			key = "right"
		}
		if key == "" {
			return nil, true
		}
		for i := 0; i < mouseWheelStep; i++ {
			switch m.top() {
			case pageInbox:
				if key == "up" || key == "down" {
					m.inboxKey(key)
				}
			case pageActions:
				if key == "up" || key == "down" {
					m.actionsKey(tea.KeyPressMsg{Code: map[string]rune{"up": tea.KeyUp, "down": tea.KeyDown}[key]})
				}
			case pageReadiness:
				m.readinessKey(key)
			case pageIssueContext:
				m.issueContextKey(key)
			case pageHelp:
				m.helpKey(key)
			case pageIncremental:
				m.incrementalKey(key)
			case pageLifecycle:
				if m.lifecycle.confirmation == nil {
					m.prLifecycleKey(key)
				}
			}
		}
		return nil, true
	}
	click, ok := msg.(tea.MouseClickMsg)
	if !ok || click.Button != tea.MouseLeft {
		return nil, true
	}
	x, y := click.X, click.Y
	switch m.top() {
	case pageInbox:
		s := &m.inbox
		details := 0
		if len(s.data.Items) > 0 {
			details = 2
		}
		w := layoutPicker(m.Height, 4+len(s.data.Problems), len(s.data.Items), details, s.picker.Index)
		if y == m.Height-1 && w.footerRows > 0 {
			if m.Busy || s.loading {
				prefix := "Loading inbox (explicit action) · "
				if m.Busy {
					prefix = Escape(m.notice) + " · "
				}
				if s.err != nil || m.ActionError != nil {
					return nil, true
				}
				offset := ansi.StringWidth(prefix)
				if pageMouseFooterKey([]pageMouseControl{{"esc: cancel", "esc"}}, "", x-offset, m.Width-offset) == "esc" {
					return m.inboxKey("esc"), true
				}
				return nil, true
			}
			if s.err == nil && m.ActionError == nil {
				return m.inboxKey(pageMouseFooterKey(inboxMouseControls, " · ", x, m.Width)), true
			}
			return nil, true
		}
		if index, hit := w.itemAt(y); hit {
			s.picker.Index = index
			id := s.data.Items[index].PullRequest.Identity
			if m.mouseActivate(fmt.Sprintf("inbox:%s:%d", id.Repository, id.Number)) {
				return m.inboxKey("enter"), true
			}
		}
	case pageInboxFilters:
		w := layoutPicker(m.Height, 1, 8, 0, m.inbox.field)
		if y == m.Height-1 && w.footerRows > 0 {
			return m.inboxFilterKey(pageMouseFooterKey(inboxFilterMouseControls, " · ", x, m.Width)), true
		}
		if index, hit := w.itemAt(y); hit {
			m.inbox.field = index
			if index != 1 && index != 2 {
				return m.inboxFilterKey("right"), true
			}
		}
	case pageActions:
		width, height := m.actionsDimensions()
		_, count := m.actionsWindow(len(m.reviewActions()), height)
		left, top := 0, 0
		if m.Width >= 20 && m.Height >= 8 {
			left = max(0, (m.Width-width-4)/2) + 2
			top = max(0, (m.Height-(min(height, count+3)+2))/2) + 1
		}
		if y == top+count+2 && x >= left && x < min(m.Width, left+width) {
			switch pageMouseFooterKey(actionsMouseControls, " · ", x-left, width) {
			case "esc":
				m.pop()
			case "enter":
				return m.activateAction(), true
			}
			return nil, true
		}
		return m.actionsMouseClick(x, y), true
	case pageHelp:
		footer := m.helpMouseControls()
		footerY := min(m.Height-1, 1+min(m.helpBodyHeight(), len(m.helpLines())))
		if y == footerY {
			m.helpKey(pageMouseFooterKey(footer, " · ", x, m.Width))
		}
	case pageReadiness:
		footerY := strings.Count(m.readinessView(), "\n")
		if y == footerY {
			key := pageMouseFooterKey(m.readinessMouseControls(), " | ", x, m.Width)
			if key == "q" {
				return m.mouseKey(key), true
			}
			return m.readinessKey(key), true
		}
	case pageIssueContext:
		footerY := strings.Count(m.issueContextView(), "\n")
		if y == footerY {
			return m.issueContextKey(pageMouseFooterKey(issueMouseControls, " · ", x, m.Width)), true
		}
	case pageEvidence:
		footerY := strings.Count(m.evidenceView(), "\n")
		if y == footerY && x < ansi.StringWidth("esc: back") && m.Width >= ansi.StringWidth("esc: back") {
			m.pop()
		}
	case pageURL:
		if y == 2 {
			key := pageMouseFooterKey([]pageMouseControl{{"esc: back", "esc"}, {"q: quit", "q"}}, " | ", x, m.Width)
			if key != "" {
				return m.mouseKey(key), true
			}
		}
		target := m.Session.Inventory.Comparison.Metadata.Identity.URL()
		if y == 0 && x < min(m.Width, ansi.StringWidth(target)) && validBrowserURL(target) {
			return func() tea.Msg {
				if err := openBrowserURL(target); err != nil {
					return Notice("Could not open URL in browser.")
				}
				return nil
			}, true
		}
	case pageIncremental:
		if y == m.Height-1 {
			if m.ActionError != nil {
				return nil, true
			}
			key := pageMouseFooterKey(incrementalMouseControls, " · ", x, m.Width)
			// Paired labels expose separate previous/next and left/right targets.
			if x < 3 {
				key = "n"
				if x == 2 {
					key = "p"
				}
			}
			panStart := ansi.StringWidth(incrementalMouseControls[0].label + " · " + incrementalMouseControls[1].label + " · ")
			if x == panStart {
				key = "left"
			}
			return m.incrementalKey(key), true
		}
	case pageLifecycle:
		if m.lifecycle.confirmation != nil {
			return m.mouseLifecycleConfirmation(x, y), true
		}
		m.lifecycleView() // shares the selected-action viewport calculation
		if y == m.Height-1 {
			return m.prLifecycleKey(pageMouseFooterKey(lifecycleMouseControls, " | ", x, m.Width)), true
		}
		if m.lifecycle.loaded {
			first := 6 + len(m.lifecycle.snapshot.Problems)
			if m.lifecycle.stale || m.lifecycle.loading {
				first++
			}
			index := y + m.lifecycle.scroll - first
			if index >= 0 && index < len(lifecycleChoices(m.lifecycle.snapshot)) {
				m.lifecycle.selected = index
				if m.mouseActivate(fmt.Sprintf("lifecycle:%d", index)) {
					return m.prLifecycleKey("enter"), true
				}
			}
		}
	}
	return nil, true
}

func (m *Model) mouseLifecycleConfirmation(x, y int) tea.Cmd {
	body, fits := m.lifecycleConfirmationBody()
	if !fits {
		return nil
	}
	width := max(1, m.Width)
	left, top := 0, 0
	if m.Width >= 20 && m.Height >= 8 {
		width = min(96, m.Width-8)
		left = max(0, (m.Width-width-4)/2) + 2
		top = max(0, (m.Height-(strings.Count(body, "\n")+3))/2) + 1
	}
	lines := m.lifecycleConfirmationLines()
	prefix := strings.Join(lines[:len(lines)-2], "\n")
	wrapped := ansi.Hardwrap(ansi.Wrap(prefix, width, ""), width, false)
	footerY := top + strings.Count(wrapped, "\n") + 1
	if y == footerY && x >= left {
		controls := []pageMouseControl{{"Enter: confirm one write", "enter"}, {"esc: cancel", "esc"}}
		return m.prLifecycleKey(pageMouseFooterKey(controls, " | ", x-left, width))
	}
	return nil
}

var issueMouseControls = []pageMouseControl{{"r: refresh context", "r"}, {"j/k: scroll", "down"}, {"esc: back", "esc"}}

func (m *Model) readinessMouseControls() []pageMouseControl {
	if m.Width < 80 {
		return []pageMouseControl{{"l: lifecycle", "l"}, {"r: refresh", "r"}, {"↑/↓: scroll", "down"}, {"esc: back", "esc"}}
	}
	return []pageMouseControl{{"l: lifecycle", "l"}, {"r/ctrl+r: refresh", "r"}, {"j/k, PgUp/PgDn, Home/End: scroll", "down"}, {"esc: back", "esc"}, {"q: quit", "q"}}
}

var actionsMouseControls = []pageMouseControl{{"Hotkey or ↑/↓", "down"}, {"Enter open", "enter"}, {"Esc close", "esc"}}

func (m *Model) helpMouseControls() []pageMouseControl {
	controls := []pageMouseControl{{"↑/↓: scroll", "down"}, {"s: all shortcuts", "s"}, {"esc: back", "esc"}}
	if m.helpExpanded {
		controls[1].label = "s: getting started"
	}
	if m.Width < 40 {
		controls[0].label = "↑↓ scroll"
		controls[1].label = "s: keys"
	}
	return controls
}
