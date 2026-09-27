package tui

import (
	"strings"
	"testing"
)

func TestGuideConsentSelectionDisclosure(t *testing.T) {
	for _, tc := range []struct {
		name, provider, model, endpoint string
		storeFalse, hasCredential       bool
	}{
		{"openai", "openai", "gpt-test", "https://api.openai.com/v1", true, true},
		{"anthropic", "anthropic", "claude-test", "https://api.anthropic.com/v1", false, true},
		{"local", "openai-compatible", "local-test", "http://127.0.0.1:1234/v1", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &Model{Width: 80}
			m.SetGuideSelection(tc.provider, tc.model, tc.endpoint, tc.storeFalse, tc.hasCredential)
			view := m.guideConsentView()
			for _, want := range []string{"Provider: " + tc.provider, "Model: " + tc.model, "Recipient: " + m.guideRecipient(), "can miss secrets"} {
				if !strings.Contains(view, want) {
					t.Fatalf("consent missing %q: %s", want, view)
				}
			}
			if strings.Contains(view, "/v1") || strings.Contains(view, "store:false") != tc.storeFalse || strings.Contains(view, "API key") != tc.hasCredential {
				t.Fatalf("consent leaked path or misstated request: %s", view)
			}
		})
	}
}
