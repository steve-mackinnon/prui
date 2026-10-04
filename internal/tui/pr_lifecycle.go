package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
)

type LifecycleRead func(context.Context, source.Identity) (source.Lifecycle, error)
type LifecycleSubmit func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error)
type lifecycleState struct {
	snapshot                     source.Lifecycle
	generation                   uint64
	loaded, loading, stale, busy bool
	selected, scroll             int
	confirmation, uncertain      *source.LifecycleAction
	notice                       string
	cancel                       context.CancelFunc
}
type LifecycleResult struct {
	Target     int
	Session    *review.Session
	Generation uint64
	Snapshot   source.Lifecycle
	Outcome    source.LifecycleOutcome
	Action     *source.LifecycleAction
	Err        error
}

func (m *Model) SetPRLifecycle(read LifecycleRead, submit LifecycleSubmit) {
	m.readLifecycle = read
	m.submitLifecycle = submit
}
func (m *Model) refreshLifecycle() tea.Cmd {
	d := &m.lifecycle
	if m.Session == nil || d.busy {
		return nil
	}
	if m.readLifecycle == nil {
		d.notice = "Lifecycle unavailable (offline or unsupported client)"
		return nil
	}
	if d.cancel != nil {
		d.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	d.cancel = cancel
	m.lifecycleGeneration++
	d.generation = m.lifecycleGeneration
	d.loading = true
	d.stale = d.loaded
	d.confirmation = nil
	d.notice = "Refreshing canonical PR lifecycle"
	target, s, gen, read := m.activeTab, m.Session, d.generation, m.readLifecycle
	id := s.Inventory.Comparison.Metadata.Identity
	return func() tea.Msg {
		value, err := read(ctx, id)
		return LifecycleResult{Target: target, Session: s, Generation: gen, Snapshot: value, Err: err}
	}
}
func (m *Model) applyLifecycleResult(v LifecycleResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.lifecycle.generation != v.Generation {
		return
	}
	d := &state.lifecycle
	d.loading = false
	d.busy = false
	if v.Action != nil {
		d.confirmation = nil
		if v.Outcome.Attempted {
			d.uncertain = v.Action
			state.readiness.stale = true
		}
		if v.Outcome.Refreshed {
			v.Snapshot = v.Outcome.Snapshot
		} else {
			d.stale = true
		}
	}
	if v.Err != nil {
		d.notice = Escape(v.Err.Error())
	} else {
		d.notice = "Canonical state refreshed"
	}
	available := v.Err == nil || (v.Action != nil && v.Outcome.Refreshed)
	if available && v.Snapshot.Identity == v.Session.Inventory.Comparison.Metadata.Identity {
		d.snapshot = v.Snapshot
		d.loaded = true
		d.stale = !v.Snapshot.Verified
		if d.uncertain != nil && d.snapshot.ActionObserved(*d.uncertain) {
			d.uncertain = nil
			d.notice = "Target state observed in canonical refresh; no write repeated"
		}
	} else {
		d.stale = true
	}
	if d.uncertain != nil {
		d.notice += " · delivery unresolved; refresh only, writes locked"
	}
}
func lifecycleChoices(s source.Lifecycle) []source.LifecycleAction {
	out := []source.LifecycleAction{}
	// Keep all methods visible with their denial reasons, including disabled ones.
	for _, method := range []string{"MERGE", "SQUASH", "REBASE"} {
		out = append(out, source.LifecycleAction{Kind: "merge", Method: method, Expected: s})
	}
	for _, method := range []string{"MERGE", "SQUASH", "REBASE"} {
		out = append(out, source.LifecycleAction{Kind: "enable-auto", Method: method, Expected: s})
	}
	for _, kind := range []string{"disable-auto", "enqueue", "dequeue", "draft", "ready", "close", "reopen"} {
		out = append(out, source.LifecycleAction{Kind: kind, Expected: s})
	}
	return out
}
func (m *Model) prLifecycleKey(key string) tea.Cmd {
	d := &m.lifecycle
	if d.busy {
		return nil
	}
	if d.confirmation != nil {
		switch key {
		case "esc":
			d.confirmation = nil
		case "enter":
			if m.submitLifecycle == nil || d.uncertain != nil || d.stale || d.loading {
				return nil
			}
			action := *d.confirmation
			d.busy = true
			d.notice = "Revalidating head, readiness, permissions and policy before one write"
			target, s, gen, submit, ctx := m.activeTab, m.Session, d.generation, m.submitLifecycle, m.ctx
			return func() tea.Msg {
				out, err := submit(ctx, action)
				return LifecycleResult{Target: target, Session: s, Generation: gen, Action: &action, Outcome: out, Err: err}
			}
		}
		return nil
	}
	switch key {
	case "esc":
		m.pop()
	case "r", "ctrl+r":
		return m.refreshLifecycle()
	case "j", "down":
		d.selected = min(len(lifecycleChoices(d.snapshot))-1, d.selected+1)
	case "k", "up":
		d.selected = max(0, d.selected-1)
	case "enter":
		if !d.loaded || d.loading || d.stale || d.uncertain != nil || m.submitLifecycle == nil {
			d.notice = "Fresh verified canonical state and lifecycle writer required; uncertain attempts allow refresh only"
			return nil
		}
		actions := lifecycleChoices(d.snapshot)
		a := actions[d.selected]
		if err := d.snapshot.ValidateAction(a); err != nil {
			d.notice = err.Error()
			return nil
		}
		d.confirmation = &a
	}
	return nil
}
func lifecycleLabel(a source.LifecycleAction) string {
	if a.Method != "" {
		return a.Kind + " · " + a.Method
	}
	return a.Kind
}
func (m *Model) lifecycleView() string {
	d := &m.lifecycle
	s := d.snapshot
	if d.confirmation != nil {
		a := d.confirmation
		body := []string{"Confirm PR lifecycle action", lifecycleLabel(*a), "PR: " + Escape(a.Expected.Identity.URL()), "Expected head: " + Escape(a.Expected.HeadSHA), "Expected base: " + Escape(a.Expected.BaseSHA), "Pinned code head: " + m.Session.Inventory.Comparison.Metadata.HeadSHA}
		if a.Expected.HeadSHA != m.Session.Inventory.Comparison.Metadata.HeadSHA {
			body = append(body, "Live head differs from the frozen reviewed code")
		}
		if a.Kind != "merge" && a.Kind != "enable-auto" && a.Kind != "enqueue" {
			body = append(body, "API has no atomic head condition; fresh preflight + server permission checks")
		}
		body = append(body, "GitHub enforces policy. No admin bypass or protection override.", "Enter: confirm one write | esc: cancel", d.notice)
		return renderActionModal(m.Width, m.Height, m.readinessView(), strings.Join(body, "\n"))
	}
	lines := []string{"PR lifecycle · live state; frozen code stays unchanged", d.notice}
	if d.loaded {
		lines = append(lines, "PR: "+Escape(s.Identity.URL()), "Expected live head: "+Escape(s.HeadSHA), fmt.Sprintf("State: %s · draft: %t · auto-merge: %t · queued: %t %s", Escape(s.State), s.Draft, s.AutoMerge, s.Queued, Escape(s.QueueState)), "Permission: "+Escape(s.Permission))
		for _, p := range s.Problems {
			lines = append(lines, Escape(p))
		}
		if d.stale || d.loading {
			lines = append(lines, "Evidence stale/unverified; actions disabled")
		}
		for i, a := range lifecycleChoices(s) {
			prefix := "  "
			if i == d.selected {
				prefix = "> "
			}
			line := prefix + lifecycleLabel(a)
			if err := s.ValidateAction(a); err != nil {
				line += " · unavailable: " + err.Error()
			}
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, "Lifecycle unknown; refresh required")
	}
	// Keep the selected action visible at narrow sizes without unbounded output.
	height := max(1, m.Height-1)
	selectedLine := len(lines) - len(lifecycleChoices(s)) + d.selected
	if d.loaded {
		d.scroll = max(0, selectedLine-height+1)
	} else {
		d.scroll = 0
	}
	end := min(len(lines), d.scroll+height)
	body := strings.Join(viewportLines(strings.Join(lines[d.scroll:end], "\n"), m.Width, height), "\n")
	return body + "\n" + strings.Join(viewportLines("↑/↓: select | enter: review/confirm | r: refresh | esc: back", m.Width, 1), "")
}
