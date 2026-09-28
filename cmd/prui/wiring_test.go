package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/guide"
	"prui/internal/guideconfig"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
	"prui/internal/theme"
	"prui/internal/tui"
)

func modelKey(m *tui.Model, code rune) {
	msg := tea.KeyPressMsg{Code: code}
	if code >= ' ' && code <= '~' {
		msg.Text = string(code)
	}
	_, cmd := m.Update(msg)
	for m.Busy && cmd != nil {
		_, cmd = m.Update(cmd())
	}
}

func completeModelAction(t *testing.T, m *tui.Model, cmd tea.Cmd) {
	t.Helper()
	for m.Busy && cmd != nil {
		_, cmd = m.Update(cmd())
	}
	if m.Busy {
		t.Fatal("action ended without clearing busy state")
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
			completeModelAction(t, m, m.Init())
			if command == "prs" || command == "current" {
				modelKey(m, tea.KeyEnter)
				modelKey(m, tea.KeyEnter)
			}
			if m.Session == nil || m.Err != nil || m.ActionError != nil {
				t.Fatal("entry failed", m.Err, m.ActionError)
			}
			expectedCalls := 0
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
		"comment":        func() error { _, err := a.submitReviewComment(ctx, tui.CommentSubmission{}); return err },
		"guide consent":  func() error { _, err := a.requestGuide(ctx, nil, guideconfig.Selection{}, nil); return err },
		"guide analysis": func() error { _, err := a.generateGuide(ctx, nil, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := operation(); err == nil || !strings.Contains(err.Error(), "offline") {
				t.Fatal("offline operation not rejected", err)
			}
		})
	}
}

func TestGuideSelectionUsesAbsoluteXDGWithoutHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", root)
	path := filepath.Join(root, "prui")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"guide":{"provider":"google","model":"gemini-test"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	a := &application{}
	a.resolveGuideSelection()
	if a.guideSelectionError != nil || a.guideSelection.Provider != "google" || a.guideSelection.Model != "gemini-test" {
		t.Fatalf("selection=%+v error=%v", a.guideSelection, a.guideSelectionError)
	}
}

func TestInteractiveGuideChoicesRestoreRememberedProviderAndModel(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "fixture-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "fixture-anthropic-key")
	a := &application{}
	a.resolveGuideSelection()
	path := guideconfig.RememberedPath(a.guideConfigPath)
	if err := guideconfig.PersistRemembered(path, guideconfig.RememberedSelection{Provider: "anthropic", Model: "claude-remembered"}); err != nil {
		t.Fatal(err)
	}
	choices, selected, err := a.interactiveGuideChoices()
	if err != nil || len(choices) != 2 || selected.Provider != "anthropic" || selected.Model != "claude-remembered" || selected.Destination != "https://api.anthropic.com" {
		t.Fatalf("remembered selection = %+v, choices=%+v, err=%v", selected, choices, err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	_, selected, err = a.interactiveGuideChoices()
	if err != nil || selected.Provider != "openai" || selected.Model != guide.DefaultModel {
		t.Fatalf("unavailable remembered provider selected: %+v, err=%v", selected, err)
	}
}

func TestConfiguredGuideAnalyzerConstruction(t *testing.T) {
	for _, tc := range []struct {
		provider, keyEnv, base string
	}{
		{"openai", "OPENAI_API_KEY", "https://api.openai.com/v1"},
		{"anthropic", "ANTHROPIC_API_KEY", "https://api.anthropic.com"},
		{"google", "GEMINI_API_KEY", "https://generativelanguage.googleapis.com"},
		{"openai-compatible", "", "http://127.0.0.1:1111/v1"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			if tc.keyEnv != "" {
				t.Setenv(tc.keyEnv, "fake-key")
			}
			a := application{guideSelectionResolved: true, guideSelection: guideconfig.Selection{Provider: tc.provider, Model: "fixture-model", BaseURL: tc.base, APIKeyEnv: tc.keyEnv}}
			analyzer, err := a.createGuideAnalyzer()
			if err != nil || analyzer == nil {
				t.Fatalf("%s analyzer unavailable: %v", tc.provider, err)
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
	completeModelAction(t, m, m.Init())
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

func TestModelThemeSelectionSaverPersistsGlobalConfig(t *testing.T) {
	app, original := wiringFixture(t)
	configPath := filepath.Join(t.TempDir(), "theme.json")
	palette, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := app.model(context.Background(), options{
		Command: "resume", SessionID: original.ID, Theme: palette, ThemeConfigPath: configPath,
	})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	modelKey(m, 't')
	m.ThemePicker.Index = 1 // light
	modelKey(m, tea.KeyEnter)
	config, exists, err := theme.LoadConfig(configPath)
	if err != nil || !exists || config.Theme != theme.Light {
		t.Fatalf("saved config = %#v, exists %t, err %v", config, exists, err)
	}
}

type guideAnalyzerFunc func(context.Context, guide.Input) (guide.Bundle, error)

func (f guideAnalyzerFunc) Analyze(ctx context.Context, input guide.Input) (guide.Bundle, error) {
	return f(ctx, input)
}

func wiringFixture(t *testing.T) (*application, *review.Session) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "fixture-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "fixture-anthropic-key")
	repo := testutil.NewRepo(t)
	repo.Write("a", "old\n")
	base := repo.Commit()
	repo.Write("a", "new\n")
	head := repo.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(temporaryDefaultStore(t, t.TempDir()))
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
			if err := review.Mark(context.Background(), app.store, original, original.Slices[0].FileID, true); err != nil {
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
