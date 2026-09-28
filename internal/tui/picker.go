package tui

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"prui/internal/session"
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
	// An explicit foreground action owns its frozen comparison until completion.
	// Older background freshness must not replace that tab or reset its Busy state.
	if m.activeTab >= 0 && m.activeTab < len(m.tabs) {
		m.tabs[m.activeTab].freshnessGeneration++
	}
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
	window := layoutPicker(m.Height, len(header), len(rows), len(detail), p.Index)
	header = header[:window.headerRows]
	detail = detail[:window.detailRows]
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
		for i := window.start; i < window.end; i++ {
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
	if window.footerRows > 0 {
		lines = append(lines, footer)
	}
	return strings.Join(lines, "\n")
}
