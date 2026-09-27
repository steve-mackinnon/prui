package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/guide"
)

func TestGuideConsentNamesConfiguredRecipient(t *testing.T) {
	for _, tc := range []struct{ endpoint, want string }{
		{"", "https://api.openai.com"},
		{"https://provider.example/api", "https://provider.example"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("OPENAI_BASE_URL", tc.endpoint)
			app, saved := wiringFixture(t)
			m := app.model(context.Background(), options{Command: "resume", SessionID: saved.ID})
			defer m.Close()
			completeModelAction(t, m, m.Init())
			modelKey(m, 'g')
			view := m.View().Content
			if !strings.Contains(view, tc.want) || strings.Contains(view, "provider.example/api") {
				t.Fatalf("wrong recipient disclosure: %s", view)
			}
			if !strings.Contains(view, "can miss secrets") || !strings.Contains(view, "API key") {
				t.Fatal("consent omits upload limitations")
			}
		})
	}
}

func TestGuideConsentNamesConfiguredModelAndRecipient(t *testing.T) {
	app, saved := wiringFixture(t)
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pr-review")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"guide":{"provider":"anthropic","model":"claude-test"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	m := app.model(context.Background(), options{Command: "resume", SessionID: saved.ID})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	modelKey(m, 'g')
	view := m.View().Content
	for _, want := range []string{"Provider: anthropic", "Model: claude-test", "Recipient: https://api.anthropic.com", "can miss secrets"} {
		if !strings.Contains(view, want) {
			t.Fatalf("consent omits %q: %s", want, view)
		}
	}
	if strings.Contains(view, "store:false") || strings.Contains(view, "OPENAI_API_KEY") {
		t.Fatalf("consent misstated provider request: %s", view)
	}
}

func TestInvalidGuideConfigDoesNotCreateAnalyzer(t *testing.T) {
	app, saved := wiringFixture(t)
	path := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "pr-review")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.json"), []byte(`{"guide":{"provider":"openai","model":"gpt-test","base_url":"https://bad.example/?key=secret"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	app.newAnalyzer = func() (guide.Analyzer, error) { panic("invalid configuration created analyzer") }
	m := app.model(context.Background(), options{Command: "resume", SessionID: saved.ID})
	defer m.Close()
	completeModelAction(t, m, m.Init())
	modelKey(m, 'g')
	if strings.Contains(m.View().Content, "key=secret") || !strings.Contains(m.View().Content, "invalid configured endpoint") {
		t.Fatalf("unsafe configuration notice: %s", m.View().Content)
	}
	modelKey(m, tea.KeyEnter)
	if m.ActionError == nil || m.Session.ID != saved.ID {
		t.Fatalf("invalid config altered session or failed silently: error=%v session=%#v", m.ActionError, m.Session)
	}
}
