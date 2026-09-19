package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/session"
)

// Each picker owns its cursor. Its visible window is derived from that cursor
// and the current terminal size, so a resize cannot hide the selection.
type pickerState struct{ Index int }

func (p *pickerState) clamp(count int) {
	p.Index = max(0, min(p.Index, count-1))
}

type SessionListResult struct {
	Entries []session.Entry
	Err     error
}

type RepositoryListResult struct {
	Repositories []session.Repository
	Err          error
}

func (m *Model) beginAction() context.Context {
	m.actionCtx, m.cancelAction = context.WithCancel(m.ctx)
	m.ActionError = nil
	return m.actionCtx
}

func (m *Model) finishAction(err error) error {
	if m.actionCtx != nil && err == nil {
		err = m.actionCtx.Err()
	}
	if m.cancelAction != nil {
		m.cancelAction()
	}
	m.actionCtx, m.cancelAction = nil, nil
	m.Busy, m.ActionError = false, err
	return err
}

func (m *Model) loadSessions() tea.Cmd {
	m.push(pagePicker)
	m.notice = "Loading saved sessions..."
	ctx := m.beginAction()
	list := m.listSessions
	return m.start(func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return SessionListResult{Err: err}
		}
		if list == nil {
			return SessionListResult{Err: errors.New("session storage unavailable")}
		}
		entries, err := list()
		return SessionListResult{Entries: entries, Err: err}
	})
}

func (m *Model) loadRepositories() tea.Cmd {
	m.push(pageRepositoryPicker)
	m.notice = "Loading remembered repositories..."
	ctx := m.beginAction()
	list := m.listRepositories
	return m.start(func() tea.Msg {
		if err := ctx.Err(); err != nil {
			return RepositoryListResult{Err: err}
		}
		if list == nil {
			return RepositoryListResult{Err: errors.New("repository storage unavailable")}
		}
		repositories, err := list()
		return RepositoryListResult{Repositories: repositories, Err: err}
	})
}

func (m *Model) pickerScreen(p *pickerState, header, rows []string, empty, footer string) string {
	if m.Busy {
		footer = Escape(m.notice) + " · esc: cancel"
	} else if m.ActionError != nil {
		footer = "! Action failed: " + Escape(m.ActionError.Error())
	}
	footer = clip(footer+" · ?: Health & help", m.Width)
	// The workspace tab strip owns the first terminal row. Preserve at least
	// one picker content row in the remaining space, even in a tiny terminal.
	height := max(1, m.Height-1)
	header = header[:min(len(header), max(0, height-1))]
	footerRows := 0
	if height > len(header)+1 {
		footerRows = 1
	}
	capacity := max(1, height-len(header)-footerRows)
	lines := append([]string(nil), header...)
	if len(rows) == 0 {
		if m.Busy {
			empty = Escape(m.notice)
		} else if m.ActionError != nil {
			empty = "Action failed: " + Escape(m.ActionError.Error())
		}
		lines = append(lines, empty)
	} else {
		selected := max(0, min(p.Index, len(rows)-1))
		start := max(0, selected-capacity+1)
		for i := start; i < min(len(rows), start+capacity); i++ {
			line := selectionMarker(i == selected) + rows[i]
			if i == selected {
				line = styleLine(selectedClass(true), line)
			}
			lines = append(lines, line)
		}
	}
	if footerRows > 0 {
		lines = append(lines, footer)
	}
	return strings.Join(lines, "\n")
}
