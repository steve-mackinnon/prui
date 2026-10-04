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

type IssueContextReader func(context.Context, source.Identity, bool) (source.IssueContext, error)
type IssueContextResult struct {
	Target     int
	Session    *review.Session
	Generation uint64
	Snapshot   source.IssueContext
	Fresh      bool
	Err        error
}
type issueContextState struct {
	snapshot      source.IssueContext
	loaded, fresh bool
	generation    uint64
	scroll        int
	notice        string
	cancel        context.CancelFunc
}

func (m *Model) SetIssueContextReader(read IssueContextReader) { m.readIssueContext = read }
func (m *Model) openIssueContext() tea.Cmd {
	if m.Session == nil {
		return nil
	}
	m.push(pageIssueContext)
	if m.issues.loaded {
		return nil
	}
	return m.loadIssueContext(false)
}
func (m *Model) loadIssueContext(refresh bool) tea.Cmd {
	if m.readIssueContext == nil || m.Session == nil {
		m.issues.notice = "Issue context unavailable"
		return nil
	}
	if m.issues.cancel != nil {
		m.issues.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.issues.cancel = cancel
	m.issues.generation++
	target, s, generation, read := m.activeTab, m.Session, m.issues.generation, m.readIssueContext
	m.issues.notice = "Loading saved context..."
	if refresh {
		m.issues.notice = "Refreshing GitHub and authorized Linear context..."
	}
	return func() tea.Msg {
		snapshot, err := read(ctx, s.Inventory.Comparison.Metadata.Identity, refresh)
		fresh := refresh
		if err != nil && refresh && !source.ValidateIssueContext(snapshot) && ctx.Err() == nil {
			if cached, cacheErr := read(ctx, s.Inventory.Comparison.Metadata.Identity, false); cacheErr == nil {
				snapshot, fresh = cached, false
			}
		}
		return IssueContextResult{Target: target, Session: s, Generation: generation, Snapshot: snapshot, Fresh: fresh, Err: err}
	}
}
func (m *Model) applyIssueContextResult(v IssueContextResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.issues.generation != v.Generation {
		return
	}
	d := &state.issues
	if v.Err != nil {
		if source.ValidateIssueContext(v.Snapshot) && v.Snapshot.Identity == v.Session.Inventory.Comparison.Metadata.Identity {
			d.snapshot, d.loaded, d.fresh = v.Snapshot, true, v.Fresh
			d.notice = Escape(v.Err.Error())
			if !v.Fresh {
				d.notice = "Saved context stale; refresh failed: " + d.notice
			}
			return
		}
		d.fresh = false
		label := "Issue context unavailable: "
		if d.loaded {
			label = "Saved context stale; refresh failed: "
		}
		d.notice = label + Escape(v.Err.Error())
		return
	}
	// The command layer supplies a validated, independently cached snapshot.
	d.snapshot, d.loaded, d.fresh, d.notice = v.Snapshot, true, v.Fresh, ""
	d.scroll = 0
}
func (m *Model) issueContextKey(k string) tea.Cmd {
	switch k {
	case "esc":
		if m.issues.cancel != nil {
			m.issues.cancel()
		}
		m.issues.generation++
		m.pop()
	case "r":
		return m.loadIssueContext(true)
	case "down", "j":
		m.issues.scroll++
	case "up", "k":
		m.issues.scroll = max(0, m.issues.scroll-1)
	case "pgdown", "d":
		m.issues.scroll += max(1, m.Height-4)
	case "pgup", "u":
		m.issues.scroll = max(0, m.issues.scroll-max(1, m.Height-4))
	case "home":
		m.issues.scroll = 0
	case "end":
		m.issues.scroll = len(m.issueContextLines())
	}
	return nil
}
func (m *Model) issueContextLines() []string {
	d := &m.issues
	lines := []string{}
	if d.notice != "" {
		lines = append(lines, d.notice)
	}
	if !d.loaded {
		return m.wrapIssueContextLines(append(lines, "No saved issue context. r explicitly reads GitHub and configured Linear."))
	}
	c := d.snapshot
	label := "Saved context · freshness unknown (historical)"
	if d.fresh {
		label = "Context captured by explicit refresh · may change"
	}
	lines = append(lines, label, fmt.Sprintf("%s #%d", c.Identity.Repository, c.Identity.Number), "Captured: "+c.CapturedAt.Format(time.RFC3339), "Context head: "+c.HeadSHA)
	if c.HeadSHA != m.Session.Inventory.Comparison.Metadata.HeadSHA {
		lines = append(lines, "Context head differs from frozen code; code and progress unchanged")
	}
	if !c.Complete {
		lines = append(lines, "PARTIAL: "+c.Reason)
	}
	decision := c.ReviewDecision
	if decision == "" {
		decision = "unknown"
	}
	lines = append(lines, "GitHub review decision: "+decision, "Requested reviewers / teams:")
	if len(c.Requested) == 0 {
		lines = append(lines, "  None observed (bounded context)")
	}
	for _, r := range c.Requested {
		lines = append(lines, "  "+r.Kind+": "+r.Name)
	}
	lines = append(lines, "Latest opinionated reviews:")
	if len(c.Reviews) == 0 {
		lines = append(lines, "  None observed (bounded context)")
	}
	for _, r := range c.Reviews {
		lines = append(lines, "  "+r.Author+": "+r.Decision)
	}
	lines = append(lines, "Labels: "+strings.Join(c.Labels, ", "), "Linked GitHub closing issues:")
	if len(c.Issues) == 0 {
		lines = append(lines, "  None observed; arbitrary URLs are not issue metadata")
	}
	for _, v := range c.Issues {
		lines = append(lines, v.Identifier+" · "+v.Status+" · "+v.Title, v.URL)
	}
	lines = append(lines, "Linear issues:")
	if c.LinearReason != "" {
		lines = append(lines, c.LinearReason)
	}
	for _, v := range c.Linear {
		lines = append(lines, v.Identifier+" · "+v.Status+" · "+v.Title, v.Description, v.URL)
	}
	lines = append(lines, "Context stays local; no additional AI upload")
	return m.wrapIssueContextLines(lines)
}
func (m *Model) wrapIssueContextLines(lines []string) []string {
	wrapped := []string{}
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(Escape(line), max(1, m.Width), ""), "\n")...)
	}
	return wrapped
}
func (m *Model) issueContextView() string {
	lines := m.issueContextLines()
	height := max(1, m.Height-2)
	m.issues.scroll = min(max(0, m.issues.scroll), max(0, len(lines)-height))
	out := []string{clip("Issue context · independent of frozen code", m.Width)}
	out = append(out, lines[m.issues.scroll:min(len(lines), m.issues.scroll+height)]...)
	out = append(out, clip("r: refresh context · j/k: scroll · esc: back", m.Width))
	return strings.Join(out, "\n")
}
