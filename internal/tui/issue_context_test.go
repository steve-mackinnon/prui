package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
	"time"
)

func TestIssueContextExplicitRefreshAndRetainedCache(t *testing.T) {
	m := commitModel(t)
	reads := []bool{}
	m.SetIssueContextReader(func(_ context.Context, id source.Identity, refresh bool) (source.IssueContext, error) {
		reads = append(reads, refresh)
		return source.IssueContext{Identity: id, HeadSHA: strings.Repeat("f", 40), CapturedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), Complete: true, Labels: []string{"bug\x1b[31m"}, LinearReason: "integration not configured"}, nil
	})
	cmd := m.openIssueContext()
	m.Update(cmd())
	if len(reads) != 1 || reads[0] {
		t.Fatal("open performed remote read")
	}
	text := m.issueContextView()
	for _, want := range []string{"Saved context", "2026-10-04", "differs", "integration not configured", "bug"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[31m") {
		t.Fatal("unescaped provider text")
	}
	cmd = m.issueContextKey("r")
	m.Update(cmd())
	if len(reads) != 2 || !reads[1] {
		t.Fatal("explicit refresh lost")
	}
	m.issues.generation++
	m.applyIssueContextResult(IssueContextResult{Target: m.activeTab, Session: m.Session, Generation: m.issues.generation, Err: errors.New("offline")})
	if !m.issues.loaded || !strings.Contains(m.issueContextView(), "stale") {
		t.Fatal("failed refresh erased saved context")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.top() != pageReview {
		t.Fatal("escape failed")
	}
}
func TestIssueContextLateResultAndViewport(t *testing.T) {
	m := commitModel(t)
	m.Width = 30
	m.Height = 8
	m.issues.generation = 2
	v := IssueContextResult{Target: m.activeTab, Session: m.Session, Generation: 1, Snapshot: source.IssueContext{Labels: []string{"wrong"}}}
	m.applyIssueContextResult(v)
	if m.issues.loaded {
		t.Fatal("late generation accepted")
	}
	m.issues.loaded = true
	m.issues.snapshot = source.IssueContext{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now(), Complete: false, Reason: "partial", Linear: []source.ContextIssue{{Title: strings.Repeat("wide ", 100), Description: strings.Repeat("line\n", 100), URL: "https://linear.app/work/issue/APP-1"}}}
	m.openIssueContext()
	m.issueContextKey("end")
	text := m.issueContextView()
	if len(strings.Split(text, "\n")) > m.Height {
		t.Fatal("viewport overflow", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(line)) > m.Width {
			t.Fatal("width overflow", line)
		}
	}
}

func TestIssueContextRefreshFailureBeforeCacheResult(t *testing.T) {
	m := commitModel(t)
	snapshot := source.IssueContext{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now(), Complete: true, ReviewDecision: "APPROVED"}
	m.SetIssueContextReader(func(_ context.Context, _ source.Identity, refresh bool) (source.IssueContext, error) {
		if refresh {
			return source.IssueContext{}, errors.New("offline")
		}
		return snapshot, nil
	})
	first := m.openIssueContext()
	refresh := m.issueContextKey("r")
	m.Update(refresh())
	m.Update(first())
	if !m.issues.loaded || m.issues.fresh || !strings.Contains(m.issueContextView(), "stale") || m.issues.snapshot.ReviewDecision != "APPROVED" {
		t.Fatal("failed refresh hid saved context")
	}
}
