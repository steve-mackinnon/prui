package main

import (
	"context"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
)

type lifecycleGH struct {
	fixtureGH
	state         source.Lifecycle
	reads, writes int
	writeErr      error
	refreshErr    bool
	changeHead    bool
}

func (g *lifecycleGH) ReadLifecycle(context.Context, source.Identity) (source.Lifecycle, error) {
	g.reads++
	if g.refreshErr && g.reads > 1 {
		return source.Lifecycle{}, errors.New("synthetic refresh failure")
	}
	s := g.state
	if g.changeHead {
		s.HeadSHA = strings.Repeat("c", 40)
	}
	return s, nil
}
func (g *lifecycleGH) WriteLifecycle(_ context.Context, a source.LifecycleAction) error {
	g.writes++
	if a.Expected.HeadSHA != g.state.HeadSHA {
		panic("retarget")
	}
	switch a.Kind {
	case "merge":
		g.state.State = "MERGED"
	case "enable-auto":
		g.state.AutoMerge = true
	case "disable-auto":
		g.state.AutoMerge = false
	case "enqueue":
		g.state.Queued = true
	case "dequeue":
		g.state.Queued = false
	case "draft":
		g.state.Draft = true
	case "reopen":
		g.state.State = "OPEN"
	case "close":
		g.state.State = "CLOSED"
	case "ready":
		g.state.Draft = false
	}
	return g.writeErr
}
func appLifecycleState() source.Lifecycle {
	return source.Lifecycle{Identity: source.Identity{Repository: "o/r", Number: 1}, NodeID: "PR_1", HeadSHA: strings.Repeat("a", 40), BaseSHA: strings.Repeat("b", 40), State: "OPEN", Verified: true, CanClose: true, CanUpdate: true}
}
func TestSubmitLifecycleRevalidatesAndRefreshes(t *testing.T) {
	a, _ := wiringFixture(t)
	s := appLifecycleState()
	g := &lifecycleGH{state: s}
	a.gh = g
	action := source.LifecycleAction{Kind: "close", Expected: s}
	a.offline = true
	if _, e := a.submitLifecycle(context.Background(), action); e == nil || g.reads != 0 || g.writes != 0 {
		t.Fatal("offline client access")
	}
	a.offline = false
	g.changeHead = true
	if _, e := a.submitLifecycle(context.Background(), action); e == nil || g.writes != 0 {
		t.Fatal("changed head accepted")
	}
	g.changeHead = false
	g.reads = 0
	out, e := a.submitLifecycle(context.Background(), action)
	if e != nil || !out.Attempted || !out.Refreshed || out.Uncertain || out.Snapshot.State != "CLOSED" || g.reads != 2 || g.writes != 1 {
		t.Fatal(out, e, g)
	}
}
func TestSubmitLifecycleUncertaintyNeverRetries(t *testing.T) {
	for _, refreshFailure := range []bool{false, true} {
		t.Run("refresh", func(t *testing.T) {
			a, _ := wiringFixture(t)
			s := appLifecycleState()
			g := &lifecycleGH{state: s, writeErr: errors.New("synthetic timeout"), refreshErr: refreshFailure}
			a.gh = g
			out, e := a.submitLifecycle(context.Background(), source.LifecycleAction{Kind: "close", Expected: s})
			if g.writes != 1 || g.reads != 2 || !out.Attempted {
				t.Fatal("repeated/skipped write", g)
			}
			if refreshFailure {
				if e == nil || !out.Uncertain || out.Refreshed {
					t.Fatal(out, e)
				}
			} else if e != nil || out.Uncertain || !out.Refreshed {
				t.Fatal("observed target not reconciled", out, e)
			}
		})
	}
}

func TestSubmitLifecycleEveryActionCanonicalRefresh(t *testing.T) {
	for _, kind := range []string{"merge", "enable-auto", "disable-auto", "enqueue", "dequeue", "draft", "ready", "close", "reopen"} {
		t.Run(kind, func(t *testing.T) {
			a, _ := wiringFixture(t)
			s := appLifecycleState()
			s.Permission = "WRITE"
			s.PolicyKnown = true
			s.Methods = []string{"SQUASH"}
			s.CanAutoMerge = true
			s.CanDisableAutoMerge = true
			s.CanReopen = true
			s.AutoMergeAllowed = true
			s.Readiness = source.Readiness{Identity: s.Identity, HeadSHA: s.HeadSHA, BaseSHA: s.BaseSHA, State: "OPEN", HeadVerified: true, RequirementsKnown: true, ChecksComplete: true, ReviewsComplete: true, Mergeable: "MERGEABLE", MergeState: "CLEAN"}
			switch kind {
			case "disable-auto":
				s.AutoMerge = true
			case "enqueue":
				s.QueueRequired = true
			case "dequeue":
				s.QueueRequired = true
				s.Queued = true
			case "ready":
				s.Draft = true
			case "reopen":
				s.State = "CLOSED"
			}
			g := &lifecycleGH{state: s}
			a.gh = g
			action := source.LifecycleAction{Kind: kind, Method: "SQUASH", Expected: s}
			out, e := a.submitLifecycle(context.Background(), action)
			if e != nil || g.writes != 1 || g.reads != 2 || !out.Refreshed || out.Uncertain || !out.Snapshot.ActionObserved(action) {
				t.Fatal(out, e, g)
			}
		})
	}
}
func TestSubmitLifecyclePermissionAndStateRevalidated(t *testing.T) {
	for _, change := range []func(*source.Lifecycle){func(s *source.Lifecycle) { s.CanClose = false }, func(s *source.Lifecycle) { s.Draft = true }, func(s *source.Lifecycle) { s.State = "CLOSED" }, func(s *source.Lifecycle) { s.Verified = false }} {
		a, _ := wiringFixture(t)
		s := appLifecycleState()
		g := &lifecycleGH{state: s}
		a.gh = g
		change(&g.state)
		out, e := a.submitLifecycle(context.Background(), source.LifecycleAction{Kind: "close", Expected: s})
		if e == nil || out.Attempted || g.writes != 0 || !out.Refreshed {
			t.Fatal("preflight contract", out, e, g)
		}
	}
}

func TestSubmitLifecycleMethodPolicyChangedAfterConfirmation(t *testing.T) {
	a, _ := wiringFixture(t)
	s := appLifecycleState()
	s.Methods = []string{"SQUASH"}
	s.Permission = "WRITE"
	s.PolicyKnown = true
	s.CanAutoMerge = true
	s.AutoMergeAllowed = true
	g := &lifecycleGH{state: s}
	g.state.Methods = []string{"REBASE"}
	a.gh = g
	out, e := a.submitLifecycle(context.Background(), source.LifecycleAction{Kind: "enable-auto", Method: "SQUASH", Expected: s})
	if e == nil || out.Attempted || g.writes != 0 {
		t.Fatal("changed policy method wrote", out, e, g)
	}
}

func TestSubmitLifecycleQueuePolicyChangedAfterConfirmation(t *testing.T) {
	for _, introduce := range []bool{false, true} {
		a, _ := wiringFixture(t)
		s := appLifecycleState()
		s.Permission = "WRITE"
		s.PolicyKnown = true
		s.CanAutoMerge = true
		s.AutoMergeAllowed = true
		s.Methods = []string{"SQUASH"}
		s.QueueRequired = !introduce
		if s.QueueRequired {
			s.QueueMethod = "SQUASH"
		}
		g := &lifecycleGH{state: s}
		g.state.QueueRequired = true
		g.state.QueueMethod = "MERGE"
		a.gh = g
		out, e := a.submitLifecycle(context.Background(), source.LifecycleAction{Kind: "enable-auto", Method: "SQUASH", Expected: s})
		if e == nil || out.Attempted || g.writes != 0 {
			t.Fatal("unconfirmed queue policy wrote", out, e, g)
		}
	}
}
