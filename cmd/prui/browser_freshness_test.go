package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"prui/internal/commits"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/tui"
)

type browserMetadata struct {
	performanceGitHub
	calls            atomic.Int64
	started, release chan struct{}
	once             sync.Once
	failure          error
}

func (g *browserMetadata) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	if g.calls.Add(1) > 2 {
		g.once.Do(func() { close(g.started) })
		select {
		case <-g.release:
		case <-ctx.Done():
			return source.Metadata{}, ctx.Err()
		}
		if g.failure != nil {
			return source.Metadata{}, g.failure
		}
	}
	return g.performanceGitHub.Metadata(ctx, id)
}
func coldBrowserFixture(t *testing.T) (*application, *review.Session, *browserMetadata) {
	t.Helper()
	app, seed := wiringFixture(t)
	if err := app.store.Delete(seed.ID); err != nil {
		t.Fatal(err)
	}
	gh := &browserMetadata{performanceGitHub: performanceGitHub{fixtureGH: app.gh.(*fixtureGH)}, started: make(chan struct{}), release: make(chan struct{})}
	app.gh = gh
	return app, seed, gh
}
func TestBrowserColdOpenCapturesCompleteSourceBeforeFreshness(t *testing.T) {
	app, seed, gh := coldBrowserFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := app.openForBrowser(ctx, string(seed.Checkout), seed.Inventory.Comparison.Metadata.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Inventory.Complete || got.Commits == nil || !got.Commits.Complete || got.Commits.Status != commits.Captured || got.Commits.Composition.Status != commits.Captured {
		t.Fatal("first frame omitted source/history")
	}
	if got.RevisionStatus != session.Unchecked || gh.calls.Load() != 2 {
		t.Fatal("post-save freshness blocked open or capture check skipped", got.RevisionStatus, gh.calls.Load())
	}
	stored, err := app.store.Load(got.ID)
	if err != nil || stored.RevisionStatus != session.Unchecked {
		t.Fatal(stored, err)
	}
	select {
	case <-gh.started:
		t.Fatal("background check started without UI owner")
	default:
	}
}
func browserFreshnessMessage(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			if msg := browserFreshnessMessage(child); msg != nil {
				return msg
			}
		}
		return nil
	}
	if _, ok := msg.(tui.PullRequestRefreshResult); ok {
		return msg
	}
	return nil
}
func TestBrowserColdOpenBackgroundFreshnessPreservesProgress(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "failed"}[failed], func(t *testing.T) {
			app, seed, gh := coldBrowserFixture(t)
			if failed {
				gh.failure = errors.New("unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := app.openForBrowser(ctx, string(seed.Checkout), seed.Inventory.Comparison.Metadata.Identity, nil)
			if err != nil {
				t.Fatal(err)
			}
			m := app.model(ctx, options{Command: "current", Repository: seed.Inventory.Comparison.Metadata.Identity.Repository, Checkout: string(seed.Checkout)})
			defer m.Close()
			completeModelAction(t, m, m.Init())
			_, cmd := m.Update(tui.PullRequestOpenResult{Target: -1, Identity: seed.Inventory.Comparison.Metadata.Identity, Session: got})
			if m.Session == nil || !strings.Contains(strings.ToLower(m.View().Content), "freshness unknown") {
				t.Fatalf("pending freshness was hidden: session=%v view=%s", m.Session != nil, m.View().Content)
			}
			done := make(chan tea.Msg, 1)
			go func() { done <- browserFreshnessMessage(cmd) }()
			select {
			case <-gh.started:
			case <-ctx.Done():
				t.Fatal("browser did not start freshness check")
			}
			if err := review.Mark(ctx, app.store, m.Session, m.Session.Slices[0].FileID, true); err != nil {
				t.Fatal(err)
			}
			close(gh.release)
			msg := <-done
			stored, err := app.store.Load(got.ID)
			if err != nil || stored.RevisionStatus != session.Unchecked {
				t.Fatal("worker persisted UI state", err)
			}
			m.Update(msg)
			want := session.Current
			if failed {
				want = session.CheckFailed
			}
			stored, err = app.store.Load(got.ID)
			if err != nil || m.Session.RevisionStatus != want || stored.RevisionStatus != want || len(stored.ReviewedSliceIDs) != 1 || stored.SnapshotReference != got.SnapshotReference {
				t.Fatal("freshness lost progress or frozen source", err)
			}
		})
	}
}

func TestCurrentBrowserWiresPendingFreshness(t *testing.T) {
	app, seed, gh := coldBrowserFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := app.model(ctx, options{Command: "current", Repository: seed.Inventory.Comparison.Metadata.Identity.Repository, Checkout: string(seed.Checkout)})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	modelKey(m, tea.KeyEnter)
	if m.Session == nil || m.Session.RevisionStatus != session.Unchecked || gh.calls.Load() != 2 {
		t.Fatal("current browser did not use pending freshness", m.ActionError, gh.calls.Load())
	}
}

func TestNonBrowserColdOpensKeepPostSaveFreshness(t *testing.T) {
	for _, kind := range []string{"direct", "list/inbox"} {
		t.Run(kind, func(t *testing.T) {
			app, seed, gh := coldBrowserFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			open := app.open
			if kind == "list/inbox" {
				open = app.openFromPullRequestList
			}
			type result struct {
				s   *review.Session
				err error
			}
			done := make(chan result, 1)
			go func() {
				s, err := open(ctx, string(seed.Checkout), seed.Inventory.Comparison.Metadata.Identity, nil)
				done <- result{s, err}
			}()
			select {
			case <-gh.started:
			case <-ctx.Done():
				t.Fatal("post-save check did not start")
			}
			select {
			case <-done:
				t.Fatal("non-browser open returned before freshness")
			default:
			}
			close(gh.release)
			got := <-done
			if got.err != nil || got.s == nil || got.s.RevisionStatus != session.Current {
				t.Fatal("non-browser freshness changed", got.err)
			}
		})
	}
}

func TestBrowserColdOpenCancelledCheckRemainsUnknown(t *testing.T) {
	app, seed, gh := coldBrowserFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := app.openForBrowser(ctx, string(seed.Checkout), seed.Inventory.Comparison.Metadata.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := app.model(ctx, options{Command: "current", Repository: seed.Inventory.Comparison.Metadata.Identity.Repository, Checkout: string(seed.Checkout)})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	_, cmd := m.Update(tui.PullRequestOpenResult{Target: -1, Identity: seed.Inventory.Comparison.Metadata.Identity, Session: got})
	done := make(chan tea.Msg, 1)
	go func() { done <- browserFreshnessMessage(cmd) }()
	select {
	case <-gh.started:
	case <-ctx.Done():
		t.Fatal("freshness did not start")
	}
	cancel()
	m.Update(<-done)
	stored, err := app.store.Load(got.ID)
	if err != nil || stored.RevisionStatus != session.Unchecked || stored.SnapshotReference != got.SnapshotReference {
		t.Fatal("cancelled check claimed freshness", err)
	}
}

func TestBrowserPendingFreshnessDoesNotBypassWritePreflight(t *testing.T) {
	for _, moved := range []bool{false, true} {
		t.Run(map[bool]string{false: "check-failed", true: "head-moved"}[moved], func(t *testing.T) {
			app, seed, gh := coldBrowserFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			got, err := app.openForBrowser(ctx, string(seed.Checkout), seed.Inventory.Comparison.Metadata.Identity, nil)
			if err != nil {
				t.Fatal(err)
			}
			close(gh.release)
			if moved {
				gh.value.HeadSHA = seed.Inventory.Comparison.Metadata.BaseSHA
			} else {
				gh.failure = errors.New("freshness unavailable")
			}
			frozen := got.Inventory.Comparison.Metadata
			err = app.submitPullRequestReview(ctx, tui.ReviewSubmission{Metadata: frozen, Review: source.PullRequestReview{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Event: "COMMENT", Body: "Fixture review"}})
			if err == nil || len(gh.reviews) != 0 || gh.calls.Load() != 3 {
				t.Fatal("pending review bypassed exact-head preflight", err)
			}
		})
	}
}
