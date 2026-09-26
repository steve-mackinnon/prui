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

func (m *Model) cancelCurrentAction() {
	if m.cancelAction == nil {
		return
	}
	cancel := m.cancelAction
	m.cancelAction = nil
	cancel()
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
	return m.pickerScreenDetail(p, header, rows, nil, empty, footer)
}

func (m *Model) pickerScreenDetail(p *pickerState, header, rows, detail []string, empty, footer string) string {
	if m.Busy {
		footer = Escape(m.notice) + " · esc: cancel"
	} else if m.ActionError != nil {
		footer = "! Action failed: " + Escape(m.ActionError.Error())
	}
	footer = clip(footer+" · ?: Health & help", m.Width)
	// Preserve one picker content row even in a tiny terminal.
	height := max(1, m.Height)
	header = header[:min(len(header), max(0, height-1))]
	footerRows := 0
	if height > len(header)+1 {
		footerRows = 1
	}
	capacity := max(1, height-len(header)-footerRows)
	// Reserve detail only when at least three choices still fit. Tiny terminals
	// prioritize the cursor and controls over supplementary metadata.
	if len(rows) == 0 || capacity < len(detail)+3 {
		detail = nil
	}
	capacity -= len(detail)
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
			line := clip(selectionMarker(i == selected)+rows[i], m.Width)
			if i == selected {
				line = m.styleLine(selectedClass(true), line)
			}
			lines = append(lines, line)
		}
	}
	for _, line := range detail {
		lines = append(lines, m.styleLine(classMetadata, clip(line, m.Width)))
	}
	if footerRows > 0 {
		lines = append(lines, footer)
	}
	return strings.Join(lines, "\n")
}
