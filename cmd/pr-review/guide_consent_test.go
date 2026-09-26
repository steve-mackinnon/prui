package main

import (
	"context"
	"strings"
	"testing"
)

func TestGuideConsentNamesConfiguredRecipient(t *testing.T) {
	for _, tc := range []struct{ endpoint, want string }{
		{"", "https://api.openai.com"},
		{"https://provider.example/api?private=hidden", "https://provider.example"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("OPENAI_BASE_URL", tc.endpoint)
			app, saved := wiringFixture(t)
			m := app.model(context.Background(), options{Command: "resume", SessionID: saved.ID})
			defer m.Close()
			completeModelAction(t, m, m.Init())
			modelKey(m, 'g')
			view := m.View().Content
			if !strings.Contains(view, tc.want) || strings.Contains(view, "private=hidden") {
				t.Fatalf("wrong recipient disclosure: %s", view)
			}
			if !strings.Contains(view, "can miss secrets") || !strings.Contains(view, "API key") {
				t.Fatal("consent omits upload limitations")
			}
		})
	}
}
