package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/inbox"
	"prui/internal/source"
)

type InboxLoader func(context.Context, source.InboxOptions, bool) (source.Inbox, error)
type InboxMarker func(context.Context, string, source.InboxItem) error
type inboxResult struct {
	Generation uint64
	Options    source.InboxOptions
	Data       source.Inbox
	Err        error
}
type inboxReadResult struct {
	Generation uint64
	Identity   source.Identity
	Err        error
}
type inboxOpenResult struct {
	Generation uint64
	Result     PullRequestOpenResult
}
type inboxState struct {
	load                      InboxLoader
	mark                      InboxMarker
	open                      PullRequestOpener
	options                   source.InboxOptions
	data                      source.Inbox
	picker                    pickerState
	generation                uint64
	cancel                    context.CancelFunc
	loading, initial, refresh bool
	field                     int
	draftOptions              source.InboxOptions
	err                       error
}

func (m *Model) SetInbox(load InboxLoader, mark InboxMarker, open PullRequestOpener, o source.InboxOptions, initial, refresh bool) {
	if o.View == "" {
		o = source.InboxOptions{View: "requested", State: "open", Draft: "all", Review: "all", Requests: "all", Activity: "all"}
	}
	m.inbox = inboxState{load: load, mark: mark, open: open, options: o, initial: initial, refresh: refresh}
	if initial {
		m.Stack = []page{pageInbox}
	}
}
func (m *Model) loadInbox(refresh bool) tea.Cmd {
	s := &m.inbox
	if s.load == nil {
		return nil
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	s.cancel = cancel
	s.generation++
	s.loading = true
	s.err = nil
	gen, o, load := s.generation, s.options, s.load
	return func() tea.Msg {
		r, err := load(ctx, o, refresh)
		return inboxResult{Generation: gen, Options: o, Data: r, Err: err}
	}
}
func (m *Model) inboxKey(k string) tea.Cmd {
	s := &m.inbox
	s.picker.clamp(len(s.data.Items))
	switch k {
	case "esc":
		if m.Busy {
			m.cancelCurrentAction()
		}
		if s.cancel != nil {
			s.cancel()
		}
		s.generation++
		s.loading = false
		if len(m.Stack) == 1 {
			m.cancel()
			return tea.Quit
		}
		m.pop()
	case "r":
		return m.loadInbox(true)
	case "f":
		s.draftOptions = s.options
		s.field = 0
		m.push(pageInboxFilters)
	case "down", "j":
		s.picker.Index = min(max(0, len(s.data.Items)-1), s.picker.Index+1)
	case "up", "k":
		s.picker.Index = max(0, s.picker.Index-1)
	case "m":
		if len(s.data.Items) == 0 || s.mark == nil {
			return nil
		}
		item := s.data.Items[s.picker.Index]
		gen, mark, viewer, ctx := s.generation, s.mark, s.data.Viewer, m.ctx
		return func() tea.Msg {
			return inboxReadResult{Generation: gen, Identity: item.PullRequest.Identity, Err: mark(ctx, viewer, item)}
		}
	case "enter":
		if len(s.data.Items) == 0 || s.open == nil {
			return nil
		}
		id := s.data.Items[s.picker.Index].PullRequest.Identity
		for i, tab := range m.tabs {
			if tab.identity == id {
				if m.Busy {
					m.cancelCurrentAction()
				}
				m.pop()
				m.activateTab(i)
				return nil
			}
		}
		if m.Busy {
			return nil
		}
		if len(m.tabs) >= maxTabs {
			s.err = fmt.Errorf("maximum of %d reviews open", maxTabs)
			return nil
		}
		generation := s.generation
		ctx := m.beginAction()
		open, target, notify := s.open, m.activeTab, m.notify
		m.notice = "Opening selected PR's frozen session or own checkout..."
		if notify == nil {
			notify = func(string) {}
		}
		return m.start(func() tea.Msg {
			r, err := open(ctx, "", id, notify)
			return inboxOpenResult{Generation: generation, Result: PullRequestOpenResult{Target: target, Identity: id, Session: r, Err: err, Frozen: true}}
		})
	}
	return nil
}
func (m *Model) inboxView() string {
	s := &m.inbox
	rows := make([]string, len(s.data.Items))
	var detail []string
	for j, i := range s.data.Items {
		request := ""
		if i.PersonalRequest {
			request += " · personal"
		}
		if i.TeamRequest {
			request += " · team"
		}
		rows[j] = fmt.Sprintf("%s#%d [%s]%s %s", Escape(i.PullRequest.Identity.Repository), i.PullRequest.Identity.Number, i.Activity, request, Escape(i.PullRequest.Title))
		if j == s.picker.Index {
			detail = []string{fmt.Sprintf("@%s · %s · draft=%t · review=%s · head=%s", Escape(i.PullRequest.Author), i.State, i.Draft, i.ReviewDecision, i.HeadSHA), "Team requests: " + Escape(strings.Join(i.Teams, ", ")) + fmt.Sprintf(" · request details complete=%t", i.RequestsComplete)}
		}
	}
	header := []string{appHeader("Review inbox", s.options.View), Escape(inbox.Label(s.data)), fmt.Sprintf("repo=%s author=%s state=%s draft=%s review=%s requests=%s activity=%s", Escape(s.options.Repository), Escape(s.options.Author), s.options.State, s.options.Draft, s.options.Review, s.options.Requests, s.options.Activity), "Local activity · unknown until marked read · alerts off · no polling"}
	for _, p := range s.data.Problems {
		header = append(header, "! "+Escape(p))
	}
	footer := "enter: open · m: mark read · f: filters · r: refresh · esc: back"
	if s.loading {
		footer = "Loading inbox (explicit action) · esc: cancel"
	}
	if s.err != nil {
		footer = "! " + Escape(s.err.Error())
	}
	return m.pickerScreenDetail(&s.picker, header, rows, detail, "No cached matches; r explicitly refreshes online.", footer)
}
func (m *Model) inboxFilterKey(k string) tea.Cmd {
	s := &m.inbox
	o := &s.draftOptions
	values := []*string{&o.View, &o.Repository, &o.Author, &o.Review, &o.State, &o.Draft, &o.Requests, &o.Activity}
	choices := [][]string{{"requested", "authored", "participated"}, nil, nil, {"all", "none", "required", "approved", "changes_requested"}, {"open", "closed", "all"}, {"all", "yes", "no"}, {"all", "personal", "team"}, {"all", "changed", "unknown", "read"}}
	switch k {
	case "esc":
		m.pop()
	case "tab", "down":
		s.field = (s.field + 1) % len(values)
	case "shift+tab", "up":
		s.field = (s.field + len(values) - 1) % len(values)
	case "enter":
		if err := o.Validate(); err != nil {
			s.err = err
			return nil
		}
		s.options = *o
		s.data = source.Inbox{}
		s.picker = pickerState{}
		m.pop()
		return m.loadInbox(false)
	case "left", "right", " ":
		c := choices[s.field]
		if len(c) > 0 {
			index := 0
			for j, v := range c {
				if v == *values[s.field] {
					index = j
				}
			}
			*values[s.field] = c[(index+1)%len(c)]
		}
	case "backspace":
		if s.field == 1 || s.field == 2 {
			v := values[s.field]
			if len(*v) > 0 {
				*v = (*v)[:len(*v)-1]
			}
		}
	default:
		if (s.field == 1 || s.field == 2) && len(k) == 1 && k >= " " && k <= "~" && len(*values[s.field]) < 128 {
			*values[s.field] += k
		}
	}
	return nil
}
func (m *Model) inboxFiltersView() string {
	s := &m.inbox
	o := s.draftOptions
	values := []string{"View: " + o.View, "Repository: " + o.Repository, "Author: " + o.Author, "Review: " + o.Review, "State: " + o.State, "Draft: " + o.Draft, "Requests: " + o.Requests, "Activity: " + o.Activity}
	p := pickerState{Index: s.field}
	return m.pickerScreen(&p, []string{"Inbox filters · alerts off; no polling"}, values, "", "tab: field · left/right: cycle · type repo/author · enter: use cache · r in inbox: refresh · esc: discard")
}

func (m *Model) applyInboxMessage(msg tea.Msg) tea.Cmd {
	s := &m.inbox
	switch v := msg.(type) {
	case inboxResult:
		if v.Generation != s.generation {
			return nil
		}
		s.loading = false
		s.err = v.Err
		if v.Err == nil {
			s.data = v.Data
			s.picker.clamp(len(v.Data.Items))
		}
	case inboxReadResult:
		if v.Generation != s.generation {
			return nil
		}
		s.err = v.Err
		if v.Err == nil {
			for j := range s.data.Items {
				if s.data.Items[j].PullRequest.Identity == v.Identity {
					s.data.Items[j].Activity = "read"
				}
			}
			s.data = inbox.Filter(s.data, s.options)
			s.picker.clamp(len(s.data.Items))
		}
	case inboxOpenResult:
		if v.Generation != s.generation {
			_ = m.finishAction(v.Result.Err)
			m.ActionError = nil
			return nil
		}
		_, cmd := m.Update(v.Result)
		return cmd
	}
	return nil
}
