package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
	"prui/internal/source"
	"strings"
	"time"
)

type ReadinessReader func(context.Context, source.Identity) (source.Readiness, error)
type ReadinessResult struct {
	Target     int
	Session    *review.Session
	Generation uint64
	Snapshot   source.Readiness
	Err        error
}
type readinessState struct {
	snapshot               source.Readiness
	generation             uint64
	loaded, loading, stale bool
	scroll                 int
	notice                 string
	cancel                 context.CancelFunc
}

func (m *Model) SetReadinessReader(read ReadinessReader) { m.readReadiness = read }
func (m *Model) refreshReadiness() tea.Cmd {
	if m.Session == nil {
		return nil
	}
	if m.readReadiness == nil {
		m.readiness.notice = "Readiness unavailable (offline or unsupported client)"
		return nil
	}
	d := &m.readiness
	if d.cancel != nil {
		d.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	d.cancel = cancel
	m.readinessGeneration++
	d.generation = m.readinessGeneration
	d.loading = true
	d.stale = d.loaded
	d.notice = "Refreshing readiness; previous evidence is stale"
	target, s, generation, read := m.activeTab, m.Session, d.generation, m.readReadiness
	id := s.Inventory.Comparison.Metadata.Identity
	return func() tea.Msg { r, err := read(ctx, id); return ReadinessResult{target, s, generation, r, err} }
}
func (m *Model) applyReadinessResult(v ReadinessResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.readiness.generation != v.Generation {
		return
	}
	d := &state.readiness
	d.loading = false
	if v.Err != nil {
		d.stale = true
		d.notice = "Readiness unavailable; retained evidence is stale: " + Escape(v.Err.Error())
		return
	}
	if v.Snapshot.Identity != v.Session.Inventory.Comparison.Metadata.Identity {
		d.stale = true
		d.notice = "Readiness identity mismatch"
		return
	}
	d.snapshot = v.Snapshot
	d.loaded = true
	d.stale = false
	d.notice = ""
}
func (m *Model) readinessKey(key string) tea.Cmd {
	switch key {
	case "esc":
		m.pop()
	case "r", "ctrl+r", "c":
		return m.refreshReadiness()
	case "j", "down":
		m.readiness.scroll++
	case "k", "up":
		m.readiness.scroll = max(0, m.readiness.scroll-1)
	case "d", "pgdown":
		m.readiness.scroll += m.pageStep()
	case "u", "pgup":
		m.readiness.scroll = max(0, m.readiness.scroll-m.pageStep())
	case "home":
		m.readiness.scroll = 0
	case "end":
		m.readiness.scroll = 1 << 20
	}
	return nil
}
func (m *Model) readinessLines() []string {
	d := &m.readiness
	lines := []string{"PR readiness · live remote evidence (read-only)", d.notice}
	if !d.loaded {
		if d.loading {
			lines = append(lines, "Loading · readiness unknown")
		} else {
			lines = append(lines, "Readiness unknown · refresh to retrieve checks and requirements")
		}
		return lines
	}
	r := d.snapshot
	frozen := m.Session.Inventory.Comparison.Metadata.HeadSHA
	lines = append(lines, "Observed head: "+r.HeadSHA, "Observed base: "+r.BaseSHA+" · "+Escape(r.BaseRef), "Captured: "+r.ObservedAt.UTC().Format(time.RFC3339), "Pinned code head: "+frozen)
	if r.HeadSHA != frozen {
		lines = append(lines, "Live head differs from pinned code; readiness does not describe the pinned comparison")
	}
	if !r.HeadVerified {
		lines = append(lines, "Head freshness unknown/stale; results are for the observed head only")
	}
	label := "Readiness unknown or blocked"
	if !d.stale && !d.loading && r.Ready(r.HeadSHA) {
		label = "No observed merge blockers at the verified live head"
	}
	if d.stale || d.loading {
		label = "Readiness stale · refresh required"
	}
	lines = append(lines, label, "Viewer repository permission: "+nonemptyTUI(r.ViewerPermission, "unknown")+" · merge authorization is not evaluated", "PR: "+r.Identity.URL(), "Rich checks: "+r.Identity.URL()+"/checks", "Merge conflicts/state: "+nonemptyTUI(r.Mergeable, "unknown")+" / "+nonemptyTUI(r.MergeState, "unknown"))
	count := "unknown"
	if r.RequirementsKnown {
		count = fmt.Sprint(r.RequiredReviews)
	} else if r.RequiredReviews > 0 {
		count = fmt.Sprintf("at least %d (full requirements unknown)", r.RequiredReviews)
	}
	lines = append(lines, "Required approving reviews: "+count+" · GitHub review decision: "+nonemptyTUI(r.ReviewDecision, "unknown"))
	for _, p := range r.Problems {
		lines = append(lines, "Unknown/incomplete: "+Escape(p))
	}
	for _, b := range r.Blockers {
		lines = append(lines, "Blocker/policy: "+Escape(b))
	}
	coverage := "complete"
	if !r.ChecksComplete {
		coverage = "incomplete"
	}
	lines = append(lines, "", "Individual checks · "+coverage+" · head "+r.HeadSHA)
	if len(r.Checks) == 0 {
		lines = append(lines, "No individual checks observed; this does not imply passing")
	}
	for _, c := range r.Checks {
		lines = append(lines, fmt.Sprintf("%s · %s · %s · %s app:%d · revision %s", Escape(c.Name), nonemptyTUI(c.Required, "unknown"), nonemptyTUI(c.State, "unknown"), Escape(c.Kind), c.AppID, c.SHA))
		if c.Conclusion != "" {
			lines = append(lines, "  Conclusion: "+Escape(c.Conclusion))
		}
		if c.URL != "" {
			lines = append(lines, "  Details: "+Escape(c.URL))
		}
	}
	coverage = "complete"
	if !r.ReviewsComplete {
		coverage = "incomplete"
	}
	lines = append(lines, "", "Current reviewer decisions · "+coverage+" (GitHub remains authoritative)")
	if len(r.Reviews) == 0 {
		lines = append(lines, "No submitted reviewer decisions observed")
	}
	for _, v := range r.Reviews {
		label := "observed head"
		if v.SHA != r.HeadSHA {
			label = "older head; approval validity depends on policy"
		}
		lines = append(lines, fmt.Sprintf("@%s · %s · revision %s · %s", Escape(v.Author), Escape(v.Decision), v.SHA, label))
		if v.URL != "" {
			lines = append(lines, "  Review: "+Escape(v.URL))
		}
	}
	return lines
}
func nonemptyTUI(s, f string) string {
	if s == "" {
		return f
	}
	return Escape(s)
}
func (m *Model) readinessView() string {
	text := ansi.Wrap(strings.Join(m.readinessLines(), "\n"), max(1, m.Width), "")
	lines := strings.Split(text, "\n")
	height := max(1, m.Height-2)
	m.readiness.scroll = min(m.readiness.scroll, max(0, len(lines)-height))
	start := m.readiness.scroll
	end := min(len(lines), start+height)
	footer := "r/ctrl+r: refresh | j/k, PgUp/PgDn, Home/End: scroll | esc: back | q: quit"
	if m.Width < 80 {
		footer = "r: refresh | ↑/↓: scroll | esc: back"
	}
	return strings.Join(lines[start:end], "\n") + "\n" + ansi.Truncate(footer, max(1, m.Width), "")
}
