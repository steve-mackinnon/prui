package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
	"pr-review/internal/tui"
)

func modelKey(m *tui.Model, code rune) {
	msg := tea.KeyPressMsg{Code: code}
	if code >= ' ' && code <= '~' {
		msg.Text = string(code)
	}
	_, cmd := m.Update(msg)
	if cmd != nil {
		m.Update(cmd())
	}
}

func TestEntryPointsShareReviewOperations(t *testing.T) {
	for _, command := range []string{"prs", "current", "open", "resume"} {
		t.Run(command, func(t *testing.T) {
			app, original := wiringFixture(t)
			calls := 0
			app.newAnalyzer = func() (guide.Analyzer, error) {
				calls++
				return unavailableGuideAnalyzer{}, nil
			}
			o := options{Command: command, SessionID: original.ID, Identity: original.Inventory.Comparison.Metadata.Identity, Repository: original.Inventory.Comparison.Metadata.Identity.Repository, Checkout: string(original.Checkout)}
			m := app.model(context.Background(), o)
			defer m.Close()
			m.Update(m.Init()())
			if command == "prs" || command == "current" {
				modelKey(m, tea.KeyEnter)
				modelKey(m, tea.KeyEnter)
			}
			if m.Session == nil || m.Err != nil || m.ActionError != nil {
				t.Fatal("entry failed", m.Err, m.ActionError)
			}
			expectedCalls := 0
			if command == "prs" || command == "current" {
				expectedCalls = 1
			}
			if calls != expectedCalls {
				t.Fatalf("analyzer calls after %s entry = %d, want %d", command, calls, expectedCalls)
			}
			modelKey(m, 'r')
			if m.ActionError != nil || m.Session.RevisionStatus != session.Current {
				t.Fatal("refresh not wired", m.ActionError)
			}
			id := m.Session.ID
			modelKey(m, 'N')
			if m.ActionError != nil || m.Session.ID == id {
				t.Fatal("new comparison not wired", m.ActionError)
			}
			id = m.Session.ID
			modelKey(m, 'g')
			if calls != expectedCalls {
				t.Fatal("consent screen created analyzer")
			}
			modelKey(m, tea.KeyEnter)
			if m.ActionError != nil || calls != expectedCalls+1 || m.Session.DerivedFrom != id {
				t.Fatal("guide not wired", m.ActionError)
			}
		})
	}
}

type forbiddenGitHub struct{}

func (forbiddenGitHub) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	panic("offline metadata call")
}
func (forbiddenGitHub) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	panic("offline PR list")
}
func (forbiddenGitHub) Token(context.Context) (string, error) { panic("offline credentials") }

func TestOfflineOperationsRejectBeforeDependencies(t *testing.T) {
	a := &application{offline: true, gh: forbiddenGitHub{}, newAnalyzer: func() (guide.Analyzer, error) { panic("offline analyzer factory") }}
	ctx := context.Background()
	for name, operation := range map[string]func() error{
		"metadata":       func() error { _, err := a.Metadata(ctx, source.Identity{}); return err },
		"list":           func() error { _, err := a.listPullRequests(ctx, "owner/repo"); return err },
		"open":           func() error { _, err := a.open(ctx, "", source.Identity{}, nil); return err },
		"fresh":          func() error { _, err := a.fresh(ctx, nil, "", nil); return err },
		"guide consent":  func() error { _, err := a.requestGuide(ctx, nil, nil); return err },
		"guide analysis": func() error { _, err := a.generateGuide(ctx, nil, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); err == nil || !strings.Contains(err.Error(), "offline") {
				t.Fatal("offline operation not rejected", err)
			}
		})
	}
}

func TestOfflineModelKeepsSavedSessionsUsable(t *testing.T) {
	app, original := wiringFixture(t)
	app.offline, app.gh = true, forbiddenGitHub{}
	app.newAnalyzer = func() (guide.Analyzer, error) { panic("offline analyzer factory") }
	m := app.model(context.Background(), options{Command: "resume", SessionID: original.ID, Offline: true})
	defer m.Close()
	m.Update(m.Init()())
	if m.Err != nil || m.Session.RevisionStatus != session.Unchecked {
		t.Fatal("offline resume failed", m.Err)
	}
	for _, code := range []rune{'r', 'N', 'g'} {
		modelKey(m, code)
		if code == 'g' {
			modelKey(m, tea.KeyEnter)
		}
		if m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "offline") || m.Session.ID != original.ID {
			t.Fatal("offline action did not retain session", m.ActionError)
		}
	}
	modelKey(m, 's')
	modelKey(m, tea.KeyEnter)
	if m.ActionError != nil || m.Session.ID != original.ID || m.Session.RevisionStatus != session.Unchecked {
		t.Fatal("saved-session picker unusable offline", m.ActionError)
	}
	modelKey(m, 'b')
	modelKey(m, tea.KeyEnter)
	if m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "offline") {
		t.Fatal("offline browser contacted GitHub", m.ActionError)
	}
}

type guideAnalyzerFunc func(context.Context, guide.Input) (guide.Bundle, error)

func (f guideAnalyzerFunc) Analyze(ctx context.Context, input guide.Input) (guide.Bundle, error) {
	return f(ctx, input)
}

func wiringFixture(t *testing.T) (*application, *review.Session) {
	t.Helper()
	repo := testutil.NewRepo(t)
	repo.Write("a", "old\n")
	base := repo.Commit()
	repo.Write("a", "new\n")
	head := repo.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	app := &application{store: store, gh: &fixtureGH{value: meta, prs: []source.PullRequest{{Identity: meta.Identity, Title: "Fixture change"}}}, runner: source.NewRunner(), limits: source.Defaults()}
	saved, err := app.open(context.Background(), repo.Dir, meta.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	return app, saved
}

func TestGuideCancellationDoesNotCreateDerivedSession(t *testing.T) {
	for _, before := range []bool{true, false} {
		name := "during analysis"
		if before {
			name = "before analysis"
		}
		t.Run(name, func(t *testing.T) {
			app, original := wiringFixture(t)
			if err := review.Mark(app.store, original, original.Slices[0].FileID, true); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if before {
				cancel()
			}
			called := false
			analyzer := guideAnalyzerFunc(func(context.Context, guide.Input) (guide.Bundle, error) {
				called = true
				cancel()
				return guide.Bundle{}, context.Canceled
			})
			derived, err := app.generateGuide(ctx, original, analyzer)
			if !errors.Is(err, context.Canceled) || derived != nil {
				t.Errorf("cancelled guide returned session %v, error %v", derived != nil, err)
			}
			if before && called {
				t.Error("cancelled request invoked analyzer")
			}
			entries, err := app.store.List()
			if err != nil || len(entries) != 1 {
				t.Fatalf("cancellation changed saved sessions: %d, %v", len(entries), err)
			}
			unchanged, err := app.store.Load(original.ID)
			if err != nil || len(unchanged.ReviewedSliceIDs) != 1 || unchanged.DerivedFrom != "" {
				t.Fatal("cancellation changed original", err)
			}
		})
	}
}
