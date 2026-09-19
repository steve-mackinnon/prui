package tui

import (
	"context"
	"errors"
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
type GuideLoader func(context.Context, *review.Session, func(string)) (*review.Session, error)
type ActionResult struct {
	Session *review.Session
	Err     error
	Reset   bool
}

type PullRequestListResult struct {
	PullRequests []source.PullRequest
	Repository   string
	Target       int
	Err          error
}

// PullRequestOpenResult carries the browser tab and immutable identity that
// initiated an asynchronous open. A result must never be applied to whichever
// review happens to be visible when the worker finishes.
type PullRequestOpenResult struct {
	Target   int
	Identity source.Identity
	Session  *review.Session
	Err      error
}

type switcherResult struct {
	identity source.Identity
	title    string
	open     bool
}

func (m *Model) SetLifecycle(store *session.Store, reader review.MetadataReader, fresh FreshLoader) {
	m.store, m.reader, m.fresh = store, reader, fresh
	if store != nil {
		m.listSessions, m.listRepositories = store.List, store.ListRepositories
	}
}

func (m *Model) SetPullRequestLifecycle(list PullRequestLoader, open PullRequestOpener) {
	m.listPullRequests, m.openPullRequest = list, open
}

func (m *Model) SetGuideLifecycle(generate GuideLoader) { m.generateGuide = generate }

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
		if m.reader == nil {
			m.ActionError = errors.New("offline mode: metadata refresh is unavailable")
			return nil, true
		}
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
		return m.loadSessions(), true
	case "b":
		return m.loadRepositories(), true
	case "g":
		if m.generateGuide == nil {
			return nil, true
		}
		m.push(pageGuideConsent)
		return nil, true
	}
	return nil, false
}

func (m *Model) guideConsentKey(k string) tea.Cmd {
	switch k {
	case "esc":
		m.pop()
	case "enter":
		if m.generateGuide == nil || m.Session == nil {
			return nil
		}
		m.pop()
		m.notice = "Sending bounded pinned source and evidence to OpenAI..."
		ctx := m.beginAction()
		s := *m.Session
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			derived, err := m.generateGuide(ctx, &s, n)
			return ActionResult{Session: derived, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) pickerKey(k string) tea.Cmd {
	m.SessionPicker.clamp(len(m.Entries))
	switch k {
	case "r":
		return m.loadSessions()
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.SessionPicker.Index = min(max(0, len(m.Entries)-1), m.SessionPicker.Index+1)
	case "p", "up", "k":
		m.SessionPicker.Index = max(0, m.SessionPicker.Index-1)
	case "enter":
		if len(m.Entries) == 0 {
			return nil
		}
		id := m.Entries[m.SessionPicker.Index].ID
		m.notice = "Resuming frozen session; checking metadata freshness..."
		return m.start(func() tea.Msg {
			s, err := review.Resume(m.ctx, m.store, id, m.reader)
			return ActionResult{Session: s, Err: err, Reset: true}
		})
	}
	return nil
}

func (m *Model) repositoryPickerKey(k string) tea.Cmd {
	m.RepositoryPicker.clamp(len(m.Repositories))
	switch k {
	case "r":
		return m.loadRepositories()
	case "esc":
		m.pop()
	case "n", "down", "j":
		m.RepositoryPicker.Index = min(max(0, len(m.Repositories)-1), m.RepositoryPicker.Index+1)
	case "p", "up", "k":
		m.RepositoryPicker.Index = max(0, m.RepositoryPicker.Index-1)
	case "enter":
		if len(m.Repositories) == 0 || m.listPullRequests == nil {
			return nil
		}
		repository := m.Repositories[m.RepositoryPicker.Index].Repository
		return m.loadPullRequests(repository)
	}
	return nil
}

func (m *Model) loadPullRequests(repository string) tea.Cmd {
	m.notice = "Listing open pull requests from GitHub..."
	ctx := m.beginAction()
	list := m.listPullRequests
	target := m.activeTab
	return m.start(func() tea.Msg {
		if list == nil {
			return PullRequestListResult{Target: target, Repository: repository, Err: errors.New("pull request listing unavailable")}
		}
		prs, err := list(ctx, repository)
		return PullRequestListResult{PullRequests: prs, Repository: repository, Target: target, Err: err}
	})
}

// openSwitcher keeps the active review visible until the overlay is rendered.
// Listing is deliberately scoped to the active repository; open reviews are
// immediately useful even while that cancellable request is in flight.
func (m *Model) openSwitcher() tea.Cmd {
	if m.Session == nil {
		return nil
	}
	m.SwitcherQuery = ""
	m.PullRequestPicker = pickerState{}
	m.push(pagePullRequestPicker)
	repository := m.Session.Inventory.Comparison.Metadata.Identity.Repository
	if repository == "" || m.listPullRequests == nil {
		return nil
	}
	return m.loadPullRequests(repository)
}

func (m *Model) switcherResults() []switcherResult {
	query := strings.ToLower(strings.TrimSpace(m.SwitcherQuery))
	matches := func(s string) bool { return query == "" || strings.Contains(strings.ToLower(s), query) }
	results := make([]switcherResult, 0, len(m.tabs)+len(m.PullRequests))
	opened := make(map[source.Identity]bool, len(m.tabs))
	for _, tab := range m.tabs {
		opened[tab.identity] = true
		label := fmt.Sprintf("%s#%d", tab.identity.Repository, tab.identity.Number)
		if matches(label) {
			results = append(results, switcherResult{identity: tab.identity, title: label, open: true})
		}
	}
	for _, pr := range m.PullRequests {
		if opened[pr.Identity] {
			continue
		}
		label := fmt.Sprintf("%s#%d %s", pr.Identity.Repository, pr.Identity.Number, pr.Title)
		if matches(label) {
			results = append(results, switcherResult{identity: pr.Identity, title: pr.Title})
		}
	}
	return results
}

func (m *Model) pullRequestRepository() string {
	if m.currentRepository != "" {
		return m.currentRepository
	}
	if len(m.Repositories) == 0 {
		return ""
	}
	m.RepositoryPicker.clamp(len(m.Repositories))
	return m.Repositories[m.RepositoryPicker.Index].Repository
}

func (m *Model) pullRequestPickerKey(k string) tea.Cmd {
	if m.Session != nil {
		return m.switcherKey(k)
	}
	m.PullRequestPicker.clamp(len(m.PullRequests))
	switch k {
	case "esc":
		if m.currentRepository != "" && len(m.Stack) == 1 {
			m.cancel()
			return tea.Quit
		}
		m.pop()
	case "r":
		if repository := m.pullRequestRepository(); repository != "" {
			return m.loadPullRequests(repository)
		}
	case "n", "down", "j":
		m.PullRequestPicker.Index = min(max(0, len(m.PullRequests)-1), m.PullRequestPicker.Index+1)
	case "p", "up", "k":
		m.PullRequestPicker.Index = max(0, m.PullRequestPicker.Index-1)
	case "enter":
		if len(m.PullRequests) == 0 || m.openPullRequest == nil {
			return nil
		}
		pr := m.PullRequests[m.PullRequestPicker.Index]
		for i, tab := range m.tabs {
			if tab.identity == pr.Identity {
				m.activateTab(i)
				return nil
			}
		}
		if len(m.tabs) >= maxTabs {
			m.ActionError = fmt.Errorf("maximum of %d tabs open; switch to an existing review", maxTabs)
			return nil
		}
		checkout := m.currentCheckout
		if checkout == "" {
			if len(m.Repositories) == 0 {
				return nil
			}
			checkout = m.Repositories[0].Checkout
			for _, repository := range m.Repositories {
				if repository.Repository == pr.Identity.Repository {
					checkout = repository.Checkout
					break
				}
			}
		}
		m.notice = "Opening selected pull request; source stays local..."
		ctx := m.beginAction()
		target := m.activeTab
		return m.start(func() tea.Msg {
			n := m.notify
			if n == nil {
				n = func(string) {}
			}
			s, err := m.openPullRequest(ctx, checkout, pr.Identity, n)
			return PullRequestOpenResult{Target: target, Identity: pr.Identity, Session: s, Err: err}
		})
	}
	return nil
}

func (m *Model) switcherKey(k string) tea.Cmd {
	results := m.switcherResults()
	m.PullRequestPicker.clamp(len(results))
	switch k {
	case "esc":
		m.pop()
		return nil
	case "backspace":
		if len(m.SwitcherQuery) > 0 {
			m.SwitcherQuery = m.SwitcherQuery[:len(m.SwitcherQuery)-1]
			m.PullRequestPicker.Index = 0
		}
		return nil
	case "up", "k":
		m.PullRequestPicker.Index = max(0, m.PullRequestPicker.Index-1)
		return nil
	case "down", "j":
		m.PullRequestPicker.Index = min(max(0, len(results)-1), m.PullRequestPicker.Index+1)
		return nil
	case "enter":
		if len(results) == 0 {
			return nil
		}
		selected := results[m.PullRequestPicker.Index]
		if selected.open {
			if m.Busy && m.cancelAction != nil {
				m.cancelAction()
			}
			m.pop() // do not persist the overlay in either review's saved state
			for i, tab := range m.tabs {
				if tab.identity == selected.identity {
					m.activateTab(i)
					return nil
				}
			}
		}
		return m.openSelectedPullRequest(selected.identity)
	}
	if len(k) == 1 && k >= " " && k <= "~" {
		m.SwitcherQuery += k
		m.PullRequestPicker.Index = 0
	}
	return nil
}

func (m *Model) openSelectedPullRequest(identity source.Identity) tea.Cmd {
	if m.openPullRequest == nil {
		m.ActionError = errors.New("pull request opening unavailable")
		return nil
	}
	if len(m.tabs) >= maxTabs {
		m.ActionError = fmt.Errorf("maximum of %d reviews open; switch to an existing review", maxTabs)
		return nil
	}
	checkout := m.currentCheckout
	if checkout == "" {
		for _, repository := range m.Repositories {
			if repository.Repository == identity.Repository {
				checkout = repository.Checkout
				break
			}
		}
	}
	if checkout == "" {
		m.ActionError = errors.New("no pinned checkout available for selected pull request")
		return nil
	}
	m.notice = "Opening selected pull request; source stays local..."
	ctx := m.beginAction()
	target := m.activeTab
	return m.start(func() tea.Msg {
		n := m.notify
		if n == nil {
			n = func(string) {}
		}
		s, err := m.openPullRequest(ctx, checkout, identity, n)
		return PullRequestOpenResult{Target: target, Identity: identity, Session: s, Err: err}
	})
}

func (m *Model) pickerView() string {
	rows := make([]string, len(m.Entries))
	for i, entry := range m.Entries {
		text := entry.ID
		if entry.Err != nil {
			text += " UNREADABLE: " + entry.Err.Error()
		} else {
			meta := entry.Record.Inventory.Comparison.Metadata
			text += fmt.Sprintf(" %s #%d %d/%d read", meta.Identity.Repository, meta.Identity.Number, len(entry.Record.ReviewedSliceIDs), len(entry.Record.Slices))
		}
		rows[i] = Escape(text)
	}
	storage := ""
	if m.store != nil {
		storage = "Storage: " + Escape(m.store.Path())
	}
	return m.pickerScreen(&m.SessionPicker, []string{appHeader("Sessions", ""), "Stored checks are historical; resume checks freshness.", storage}, rows, "No saved sessions.", "enter: resume · r: reload · esc: back")
}

func (m *Model) repositoryPickerView() string {
	rows := make([]string, len(m.Repositories))
	for i, repository := range m.Repositories {
		rows[i] = Escape(repository.Repository)
	}
	return m.pickerScreen(&m.RepositoryPicker, []string{appHeader("Repositories", ""), "Select a repository to list its open pull requests."}, rows, "No remembered repositories. Open a PR with --repo first.", "enter: open · r: reload · esc: back")
}

func (m *Model) pullRequestPickerView() string {
	if m.Session != nil {
		return m.switcherView()
	}
	rows := make([]string, len(m.PullRequests))
	for i, pr := range m.PullRequests {
		rows[i] = fmt.Sprintf("#%d %s", pr.Identity.Number, Escape(pr.Title))
	}
	return m.pickerScreen(&m.PullRequestPicker, []string{appHeader("Open pull requests", "")}, rows, "No open pull requests.", "enter: open · esc: back")
}

func (m *Model) switcherView() string {
	results := m.switcherResults()
	m.PullRequestPicker.clamp(len(results))
	rows := make([]string, len(results))
	for i, result := range results {
		if result.open {
			rows[i] = "open " + Escape(result.title)
		} else {
			rows[i] = fmt.Sprintf("#%d %s", result.identity.Number, Escape(result.title))
		}
	}
	header := []string{appHeader("Switch pull requests", "type to filter"), "filter: " + Escape(m.SwitcherQuery)}
	footer := "type: filter · enter: switch/open · esc: cancel"
	return m.pickerScreen(&m.PullRequestPicker, header, rows, "No matching open reviews or pull requests.", footer)
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
