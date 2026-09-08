package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

func (m *Model) beginMove() {
	if m.Session == nil || m.store == nil || len(m.Session.Inventory.Units) == 0 {
		return
	}
	m.EditIndex = 0
	m.push(pageEdit)
}

func (m *Model) beginReorder() {
	if m.Session == nil || m.store == nil || len(m.Session.Inventory.Files) == 0 {
		return
	}
	if p := m.Session.CurrentPlan(); p != nil && len(p.Slices) > 0 {
		m.EditIndex = m.Session.UnitFiles[m.Selected]
		m.push(pageReorder)
	}
}

func (m *Model) editKey(k string) tea.Cmd {
	if m.top() == pageReorder {
		p := m.Session.CurrentPlan()
		if p == nil {
			m.pop()
			return nil
		}
		switch k {
		case "up", "k":
			m.EditIndex = max(0, m.EditIndex-1)
		case "down", "j":
			m.EditIndex = min(len(p.Slices)-1, m.EditIndex+1)
		case "enter":
			order := make([]string, len(p.Slices))
			for i := range p.Slices {
				order[i] = p.Slices[i].SliceID
			}
			selected := order[m.Session.UnitFiles[m.Selected]]
			order = append([]string{selected}, removeID(order, selected)...)
			// Move the selected slice to the previewed position.
			order = moveID(order, selected, m.EditIndex)
			return m.applyEdit(func(s *review.Session) error { return review.ReorderSlices(m.store, s, order) })
		case "esc":
			m.pop()
		}
		return nil
	}
	if m.top() == pageEdit {
		count := len(m.Session.Slices) + 1 // final entry is unassigned
		switch k {
		case "up", "k":
			m.EditIndex = max(0, m.EditIndex-1)
		case "down", "j":
			m.EditIndex = min(count-1, m.EditIndex+1)
		case "enter":
			destination := ""
			if m.EditIndex < len(m.Session.Slices) {
				destination = m.Session.Slices[m.EditIndex].FileID
			}
			id := m.Session.Inventory.Units[m.Selected].ID
			return m.applyEdit(func(s *review.Session) error { return review.MoveUnit(m.store, s, id, destination) })
		case "esc":
			m.pop()
		}
	}
	return nil
}

func (m *Model) applyEdit(work func(*review.Session) error) tea.Cmd {
	m.notice = "Preview confirmed; saving new plan and clearing completion..."
	m.pop()
	m.Busy = true
	s := *m.Session
	return m.start(func() tea.Msg { err := work(&s); return ActionResult{Session: &s, Err: err, Reset: err == nil} })
}

func removeID(ids []string, id string) []string {
	out := ids[:0]
	for _, v := range ids {
		if v != id {
			out = append(out, v)
		}
	}
	return out
}
func moveID(ids []string, id string, at int) []string {
	ids = removeID(ids, id)
	at = max(0, min(len(ids), at))
	return append(ids[:at], append([]string{id}, ids[at:]...)...)
}

type FreshLoader func(context.Context, *review.Session, func(string)) (*review.Session, error)
type ActionResult struct {
	Session *review.Session
	Err     error
	Reset   bool
}

type PullRequestListResult struct {
	PullRequests []source.PullRequest
	Err          error
}

func (m *Model) SetLifecycle(store *session.Store, reader review.MetadataReader, fresh FreshLoader) {
	m.store, m.reader, m.fresh = store, reader, fresh
}

func (m *Model) SetPullRequestLifecycle(list PullRequestLoader, open PullRequestOpener) {
	m.listPullRequests, m.openPullRequest = list, open
}

func (m *Model) start(work func() tea.Msg) tea.Cmd {
	m.Busy = true
	result := make(chan tea.Msg, 1)
	done := make(chan struct{})
	m.worker = done
	go func() { defer close(done); result <- work() }()
	return func() tea.Msg { return <-result }
}

func (m *Model) Close() {
	m.cancel()
	if m.worker != nil {
		<-m.worker
	}
}

func (m *Model) lifecycleKey(k string) (tea.Cmd, bool) {
	if m.store == nil || m.Session == nil {
		return nil, false
	}
	s := *m.Session
	switch k {
	case "m":
		if len(s.Slices) == 0 {
			return nil, true
		}
		// Guide rows only ever point at a frozen unit, so marking resolves
		// through UnitFiles exactly as the file plan does: the whole slice is
		// marked, including the units this file contributes to other guides.
		id := s.Slices[s.UnitFiles[m.Selected]].FileID
		m.notice = "Saving local reading progress..."
		return m.start(func() tea.Msg {
			err := review.Mark(m.store, &s, id, !slices.Contains(s.ReviewedSliceIDs, id))
			return ActionResult{Session: &s, Err: err}
		}), true
	case "r":
		m.notice = "Checking GitHub metadata only; source stays local..."
		return m.start(func() tea.Msg {
			err := review.Refresh(m.ctx, m.store, &s, m.reader)
			return ActionResult{Session: &s, Err: err}
		}), true
	case "N":
		if m.fresh == nil {
			return nil, true
		}
		m.notice = "Opening new comparison; old snapshot and progress retained..."
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			fresh, err := m.fresh(m.ctx, &s, n)
			return ActionResult{Session: fresh, Err: err, Reset: true}
		}), true
	case "s":
		m.Entries, m.ActionError = m.store.List()
		m.PickerIndex = 0
		m.push(pagePicker)
		return nil, true
	case "b":
		m.Repositories, m.ActionError = m.store.ListRepositories()
		m.PickerIndex = 0
		m.push(pageRepositoryPicker)
		return nil, true
	}
	return nil, false
}

func (m *Model) pickerKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.PickerIndex = min(max(0, len(m.Entries)-1), m.PickerIndex+1)
	case "p", "up", "k":
		m.PickerIndex = max(0, m.PickerIndex-1)
	case "enter":
		if len(m.Entries) == 0 {
			return nil
		}
		id := m.Entries[m.PickerIndex].ID
		m.notice = "Resuming frozen session; checking metadata freshness..."
		return m.start(func() tea.Msg {
			s, err := review.Resume(m.ctx, m.store, id, m.reader)
			return ActionResult{Session: s, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) repositoryPickerKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.PickerIndex = min(max(0, len(m.Repositories)-1), m.PickerIndex+1)
	case "p", "up", "k":
		m.PickerIndex = max(0, m.PickerIndex-1)
	case "enter":
		if len(m.Repositories) == 0 || m.listPullRequests == nil {
			return nil
		}
		repository := m.Repositories[m.PickerIndex].Repository
		m.notice = "Listing open pull requests from GitHub..."
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelAction = cancel
		return m.start(func() tea.Msg {
			prs, err := m.listPullRequests(ctx, repository)
			return PullRequestListResult{PullRequests: prs, Err: err}
		})
	}
	return nil
}

func (m *Model) pullRequestPickerKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.PickerIndex = min(max(0, len(m.PullRequests)-1), m.PickerIndex+1)
	case "p", "up", "k":
		m.PickerIndex = max(0, m.PickerIndex-1)
	case "enter":
		if len(m.PullRequests) == 0 || len(m.Repositories) == 0 || m.openPullRequest == nil {
			return nil
		}
		pr := m.PullRequests[m.PickerIndex]
		checkout := m.Repositories[0].Checkout
		for _, repository := range m.Repositories {
			if repository.Repository == pr.Identity.Repository {
				checkout = repository.Checkout
				break
			}
		}
		m.notice = "Opening selected pull request; source stays local..."
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancelAction = cancel
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			s, err := m.openPullRequest(ctx, checkout, pr.Identity, n)
			return ActionResult{Session: s, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) pickerView() string {
	lines := []string{"Sessions (stored checks are historical; resume checks freshness)", "Storage: " + Escape(m.store.Path())}
	start := max(0, m.PickerIndex-max(1, m.Height-4)+1)
	for i := start; i < min(len(m.Entries), start+max(1, m.Height-4)); i++ {
		entry := m.Entries[i]
		marker := "  "
		if i == m.PickerIndex {
			marker = "> "
		}
		text := entry.ID
		if entry.Err != nil {
			text += " UNREADABLE: " + Escape(entry.Err.Error())
		} else {
			meta := entry.Record.Inventory.Comparison.Metadata
			text += fmt.Sprintf(" %s #%d %d/%d read", meta.Identity.Repository, meta.Identity.Number, len(entry.Record.ReviewedSliceIDs), len(entry.Record.Slices))
		}
		lines = append(lines, marker+Escape(text))
	}
	if len(m.Entries) == 0 {
		lines = append(lines, "No saved sessions.")
	}
	lines = append(lines, "up/down: select | enter: resume | esc: back | q: quit")
	if m.ActionError != nil {
		lines = append(lines, Escape(m.ActionError.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) repositoryPickerView() string {
	lines := []string{"Remembered repositories", "Select a repository to list its open pull requests."}
	for i, repository := range m.Repositories {
		marker := "  "
		if i == m.PickerIndex {
			marker = "> "
		}
		lines = append(lines, marker+Escape(repository.Repository))
	}
	if len(m.Repositories) == 0 {
		lines = append(lines, "No remembered repositories. Open a PR with --repo first.")
	}
	lines = append(lines, "up/down: select | enter: list PRs | esc: back | q: quit")
	if m.ActionError != nil {
		lines = append(lines, Escape(m.ActionError.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) pullRequestPickerView() string {
	lines := []string{"Open pull requests"}
	for i, pr := range m.PullRequests {
		marker := "  "
		if i == m.PickerIndex {
			marker = "> "
		}
		lines = append(lines, fmt.Sprintf("%s#%d %s", marker, pr.Identity.Number, Escape(pr.Title)))
	}
	if len(m.PullRequests) == 0 {
		lines = append(lines, "No open pull requests.")
	}
	lines = append(lines, "up/down: select | enter: open PR | esc: back | q: quit")
	if m.ActionError != nil {
		lines = append(lines, Escape(m.ActionError.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) footer() string {
	if m.Busy {
		return Escape(m.notice) + " | q: cancel"
	}
	if m.ActionError != nil {
		return "Action failed; snapshot retained: " + Escape(m.ActionError.Error())
	}
	return strings.ReplaceAll(renderBindings(groupFooter), "\n", "  ")
}

func progress(s *review.Session) string {
	state := s.RevisionStatus
	if state == "" {
		state = session.Unchecked
	}
	text := fmt.Sprintf("freshness: %s | %d/%d read (local)", state, len(s.ReviewedSliceIDs), len(s.Slices))
	if state == session.Stale {
		text += " | keep reading old or N: new/reset"
	}
	if state == session.Unchecked || state == session.CheckFailed {
		text += " | freshness unknown"
	}
	return text
}

// progressClass warns on freshness states that are not confirmed current. The
// label text itself is unchanged, so the state survives without color.
func progressClass(s *review.Session) lineClass {
	if s.RevisionStatus == session.Stale || s.RevisionStatus == session.CheckFailed {
		return classWarning
	}
	return classTitle
}

func readMarker(s *review.Session, id string) string {
	if s.ID == "" {
		return ""
	}
	if slices.Contains(s.ReviewedSliceIDs, id) {
		return "[x] "
	}
	return "[ ] "
}
