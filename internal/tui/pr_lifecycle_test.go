package tui

import (
	"context"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
)

func tuiLifecycleState(m *Model) source.Lifecycle {
	return source.Lifecycle{Identity: m.Session.Inventory.Comparison.Metadata.Identity, NodeID: "PR_1", HeadSHA: strings.Repeat("f", 40), BaseSHA: strings.Repeat("b", 40), State: "OPEN", Verified: true, CanClose: true, CanUpdate: true}
}
func TestPRLifecycleConfirmationRepeatedKeysAndUncertainty(t *testing.T) {
	m := commitModel(t)
	s := tuiLifecycleState(m)
	session := m.Session
	writes, reads := 0, 0
	m.SetPRLifecycle(func(context.Context, source.Identity) (source.Lifecycle, error) { reads++; return s, nil }, func(_ context.Context, a source.LifecycleAction) (source.LifecycleOutcome, error) {
		writes++
		return source.LifecycleOutcome{Attempted: true, Uncertain: true}, errors.New("synthetic uncertain")
	})
	m.push(pageReadiness)
	cmd := m.readinessKey("l")
	m.applyLifecycleResult(cmd().(LifecycleResult))
	m.lifecycle.selected = 11 // close
	if m.prLifecycleKey("enter") != nil || m.lifecycle.confirmation == nil || writes != 0 {
		t.Fatal("selection wrote")
	}
	text := m.lifecycleView()
	for _, want := range []string{s.HeadSHA, s.Identity.URL(), "Live head differs", "no atomic head", "confirm one write"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	m.prLifecycleKey("esc")
	if m.lifecycle.confirmation != nil || writes != 0 {
		t.Fatal("escape wrote")
	}
	m.prLifecycleKey("enter")
	cmd = m.prLifecycleKey("enter")
	if cmd == nil {
		t.Fatal("missing submit")
	}
	if m.prLifecycleKey("enter") != nil || m.prLifecycleKey("r") != nil {
		t.Fatal("duplicate in-flight command")
	}
	m.applyLifecycleResult(cmd().(LifecycleResult))
	if writes != 1 || m.lifecycle.uncertain == nil || !m.lifecycle.stale {
		t.Fatal("uncertain attempt lost")
	}
	if m.prLifecycleKey("enter") != nil {
		t.Fatal("uncertain write repeated")
	}
	cmd = m.prLifecycleKey("r")
	m.applyLifecycleResult(cmd().(LifecycleResult))
	if writes != 1 || m.lifecycle.uncertain == nil {
		t.Fatal("refresh retried or unlocked unobserved state")
	}
	s.State = "CLOSED"
	cmd = m.prLifecycleKey("r")
	m.applyLifecycleResult(cmd().(LifecycleResult))
	if writes != 1 || m.lifecycle.uncertain != nil || reads != 3 || m.Session != session {
		t.Fatal("reconciliation changed pins/repeated write")
	}
}
func TestPRLifecycleRefreshGenerationDenialsAndOffline(t *testing.T) {
	m := commitModel(t)
	m.push(pageLifecycle)
	if m.refreshLifecycle() != nil || !strings.Contains(m.lifecycleView(), "offline") {
		t.Fatal("offline capability")
	}
	s := tuiLifecycleState(m)
	m.SetPRLifecycle(func(context.Context, source.Identity) (source.Lifecycle, error) { return s, nil }, nil)
	first, second := m.refreshLifecycle(), m.refreshLifecycle()
	m.applyLifecycleResult(first().(LifecycleResult))
	if m.lifecycle.loaded {
		t.Fatal("stale read accepted")
	}
	m.applyLifecycleResult(second().(LifecycleResult))
	m.lifecycle.selected = 11
	if m.prLifecycleKey("enter") != nil || m.lifecycle.confirmation != nil {
		t.Fatal("read-only client confirmation")
	}
	m.submitLifecycle = func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		t.Fatal("denied write")
		return source.LifecycleOutcome{}, nil
	}
	m.lifecycle.snapshot.CanClose = false
	m.prLifecycleKey("enter")
	if m.lifecycle.confirmation != nil {
		t.Fatal("denied action confirmed")
	}
	m.Width = 18
	m.Height = 4
	view := m.lifecycleView()
	if strings.Count(view, "\n") >= m.Height {
		t.Fatal("tiny viewport overflow", view)
	}
	m.prLifecycleKey("esc")
	if m.top() != pageReview {
		t.Fatal("navigation")
	}
}
func TestPRLifecycleResultOriginDoesNotRetarget(t *testing.T) {
	m := commitModel(t)
	s := tuiLifecycleState(m)
	m.lifecycle.generation = 7
	v := LifecycleResult{Target: m.activeTab, Session: m.Session, Generation: 6, Snapshot: s}
	m.applyLifecycleResult(v)
	if m.lifecycle.loaded {
		t.Fatal("stale result")
	}
	v.Generation = 7
	v.Snapshot.Identity.Repository = "wrong/repo"
	m.applyLifecycleResult(v)
	if m.lifecycle.loaded || !m.lifecycle.stale {
		t.Fatal("wrong identity")
	}
	v.Session = nil
	v.Snapshot = s
	m.applyLifecycleResult(v)
	if m.lifecycle.loaded {
		t.Fatal("different session result")
	}
}

func TestPRLifecycleCrossTabWriteResults(t *testing.T) {
	m := commitModel(t)
	origin := m.reviewTabState
	s := tuiLifecycleState(m)
	origin.lifecycle.generation = 8
	m.tabs = []workspaceTab{{review: origin}, {review: newReviewTabState(m.Session)}}
	m.activeTab = 1
	m.restoreReviewTab(m.tabs[1].review)
	action := source.LifecycleAction{Kind: "close", Expected: s}
	s.State = "CLOSED"
	m.applyLifecycleResult(LifecycleResult{Target: 0, Session: origin.Session, Generation: 8, Action: &action, Outcome: source.LifecycleOutcome{Attempted: true, Refreshed: true, Snapshot: s}})
	if !origin.lifecycle.loaded || origin.lifecycle.snapshot.State != "CLOSED" || m.lifecycle.loaded {
		t.Fatal("result retargeted to active tab")
	}
}

func TestPRLifecycleQueueAutoMergeConfirmationUsesPolicyMethod(t *testing.T) {
	m := commitModel(t)
	s := tuiLifecycleState(m)
	s.QueueRequired = true
	s.QueueMethod = "SQUASH"
	s.PolicyKnown = true
	s.Permission = "WRITE"
	s.AutoMergeAllowed = true
	s.CanAutoMerge = true
	m.lifecycle = lifecycleState{snapshot: s, loaded: true, selected: 3}
	m.submitLifecycle = func(context.Context, source.LifecycleAction) (source.LifecycleOutcome, error) {
		return source.LifecycleOutcome{}, nil
	}
	m.prLifecycleKey("enter")
	if m.lifecycle.confirmation == nil || m.lifecycle.confirmation.Method != "" {
		t.Fatal("queue method override")
	}
	if !strings.Contains(m.lifecycleView(), "queue policy method: SQUASH") || !strings.Contains(m.lifecycleView(), "no method override") {
		t.Fatal("queue method not reviewable", m.lifecycleView())
	}
}

func TestPRLifecycleQueuePolicyRefreshClampsSelection(t *testing.T) {
	m := commitModel(t)
	s := tuiLifecycleState(m)
	m.lifecycle.generation = 1
	m.lifecycle.selected = 12
	s.QueueRequired = true
	m.applyLifecycleResult(LifecycleResult{Target: m.activeTab, Session: m.Session, Generation: 1, Snapshot: s})
	if m.lifecycle.selected >= len(lifecycleChoices(s)) {
		t.Fatal("selection outside updated choices")
	}
	m.Width = 12
	m.Height = 2
	_ = m.lifecycleView()
	m.prLifecycleKey("enter")
}
