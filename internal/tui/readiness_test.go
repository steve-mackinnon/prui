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

func TestReadinessRefreshRacesAndPinnedCodeIndependence(t *testing.T) {
	m := commitModel(t)
	s := m.Session
	id := s.Inventory.Comparison.Metadata.Identity
	calls := 0
	m.SetReadinessReader(func(_ context.Context, got source.Identity) (source.Readiness, error) {
		calls++
		if got != id {
			t.Fatal("identity")
		}
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("f", 40), BaseSHA: strings.Repeat("b", 40), ObservedAt: time.Now(), HeadVerified: true}, nil
	})
	first := m.refreshReadiness()
	second := m.refreshReadiness()
	m.applyReadinessResult(first().(ReadinessResult))
	if m.readiness.loaded {
		t.Fatal("old refresh accepted")
	}
	m.applyReadinessResult(second().(ReadinessResult))
	if !m.readiness.loaded || m.Session != s || calls != 2 {
		t.Fatal("refresh changed pinned code or lost result")
	}
	text := strings.Join(m.readinessLines(), "\n")
	for _, want := range []string{"Live head differs", s.Inventory.Comparison.Metadata.HeadSHA, strings.Repeat("f", 40), "Required approving reviews: unknown"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing revision/state", want, text)
		}
	}
	old := ReadinessResult{Target: m.activeTab, Session: s, Generation: m.readiness.generation, Snapshot: source.Readiness{Identity: source.Identity{"wrong/repo", 1}}}
	m.applyReadinessResult(old)
	if !m.readiness.stale {
		t.Fatal("wrong identity accepted")
	}
}
func TestReadinessLoadingFailureAndUnknownNotPassing(t *testing.T) {
	m := commitModel(t)
	m.SetReadinessReader(func(context.Context, source.Identity) (source.Readiness, error) {
		return source.Readiness{}, errors.New("synthetic denied")
	})
	c := m.refreshReadiness()
	if !strings.Contains(strings.Join(m.readinessLines(), "\n"), "Loading · readiness unknown") {
		t.Fatal("loading looked passing")
	}
	m.applyReadinessResult(c().(ReadinessResult))
	if !m.readiness.stale || m.readiness.loading {
		t.Fatal("failure lost")
	}
	m.readiness.loaded = true
	m.readiness.snapshot = source.Readiness{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("a", 40), Checks: []source.ReadinessCheck{{Name: "ci\x1b[31m", Required: "unknown", State: "unknown", SHA: strings.Repeat("a", 40), URL: "https://example.com/failure"}}, Reviews: []source.ReadinessReview{{Author: "alice", Decision: "DISMISSED", SHA: strings.Repeat("b", 40)}}}
	text := strings.Join(m.readinessLines(), "\n")
	for _, want := range []string{"Readiness stale", "unknown", "https://example.com/failure", "older head", "DISMISSED"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Contains(text, "No observed merge blockers") {
		t.Fatal("untrusted or passing output", text)
	}
}
func TestReadinessControlsResizeAndOffline(t *testing.T) {
	m := commitModel(t)
	m.Width = 40
	m.Height = 12
	key(m, 'C')
	if m.top() != pageReadiness || !strings.Contains(m.readinessView(), "offline") {
		t.Fatal("readiness unavailable not visible")
	}
	m.readiness.loaded = true
	m.readiness.snapshot = source.Readiness{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("f", 40)}
	m.readinessKey("end")
	view := m.readinessView()
	if m.readiness.scroll == 0 || strings.Count(view, "\n") > m.Height {
		t.Fatal("viewport not bounded", view)
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.readiness.scroll < 0 {
		t.Fatal("negative scroll")
	}
	m.readinessKey("home")
	if m.readiness.scroll != 0 {
		t.Fatal("home")
	}
	m.readinessKey("esc")
	if m.top() != pageReview {
		t.Fatal("escape")
	}
}
