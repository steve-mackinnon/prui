package tui

import (
	"context"
	"strings"
	"testing"
)

func TestSecurityAuditDescriptionControls(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		forbidden     rune
	}{
		{"entity_bell", "safe&#7;hostile", '\a'},
		{"C1CSI", "safe\u009b2Jhostile", '\u009b'},
		{"C1OSC", "safe\u009d52;c;aGVsbG8=\u009c", '\u009d'},
		{"bidi", "safe\u202ehostile\u202c", '\u202e'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			t.Cleanup(m.Close)
			s := screenSession()
			s.PullRequestDescription = &tc.payload
			m.openReviewTab(s)
			m.Width, m.Height = 80, 24
			m.selectReviewView(viewDescription)
			out := m.View().Content
			if strings.ContainsRune(out, tc.forbidden) {
				t.Errorf("PR description control U+%04X survives Model.View", tc.forbidden)
			}
		})
	}
}

func TestSecurityAuditDescriptionEntityOSCIsInertInView(t *testing.T) {
	payload := "safe&#x1b;]52;c;hostile&#7;"
	m := New(context.Background(), nil)
	t.Cleanup(m.Close)
	s := screenSession()
	s.PullRequestDescription = &payload
	m.openReviewTab(s)
	m.Width, m.Height = 80, 24
	m.selectReviewView(viewDescription)
	out := m.View().Content
	if strings.Contains(out, "\x1b]52") || strings.ContainsRune(out, '\a') {
		t.Fatalf("entity-encoded OSC controls survived Model.View: %.200q", out)
	}
	for _, want := range []string{`\x1b`, `\x07`} {
		if !strings.Contains(out, want) {
			t.Fatalf("entity-encoded control not visible as %q: %.200q", want, out)
		}
	}
}
