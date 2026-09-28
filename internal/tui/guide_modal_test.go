package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/guideconfig"
	"pr-review/internal/review"
)

func TestGuideModalSelectsExactProviderAndModel(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 80, 24
	m.Session = &review.Session{}
	options := []guideconfig.Selection{
		{Provider: "openai", Model: "gpt-test", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
		{Provider: "anthropic", Model: "claude-test", Destination: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"},
	}
	var requested, saved guideconfig.Selection
	m.SetGuideOptions(options, options[0], func(s guideconfig.Selection) error { saved = s; return nil })
	m.SetGuideLifecycle(func(_ context.Context, _ *review.Session, s guideconfig.Selection, _ func(string)) (*review.Session, error) {
		requested = s
		return nil, nil
	})
	m.push(pageGuideConsent)
	if view := m.View().Content; !strings.Contains(view, "Provider: openai") || !strings.Contains(view, "Model: gpt-test") || !strings.Contains(view, "Recipient: https://api.openai.com") {
		t.Fatalf("missing initial selection: %s", view)
	}
	m.guideConsentKey("tab")
	m.guideConsentKey("right")
	m.guideConsentKey("tab")
	m.guideConsentKey("X")
	m.guideConsentKey("tab")
	m.guideConsentKey("tab")
	m.guideConsentKey("right")
	m.guideConsentKey("right")
	m.guideConsentKey("tab")
	if view := m.View().Content; !strings.Contains(view, "Provider: anthropic") || !strings.Contains(view, "Model: claude-testX") || !strings.Contains(view, "Recipient: https://api.anthropic.com") {
		t.Fatalf("selection did not update: %s", view)
	}
	m.guideConsentKey("ctrl+a")
	m.guideConsentKey("N")
	if !strings.Contains(m.View().Content, "Model: N") {
		t.Fatal("clear-and-replace model failed")
	}
	m.guideConsentKey("ctrl+a")
	for _, key := range "claude-testX" {
		m.guideConsentKey(string(key))
	}
	m.guideConsentKey("tab")
	cmd := m.guideConsentKey("enter")
	if cmd == nil || saved.Provider != "anthropic" || saved.Model != "claude-testX" {
		t.Fatalf("confirmation failed: saved=%+v cmd=%v", saved, cmd)
	}
	cmd()
	if requested != saved {
		t.Fatalf("request diverged from consent: requested=%+v saved=%+v", requested, saved)
	}
}

func TestGuideModalCancelDiscardsUnconfirmedEdits(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 80, 24
	m.Session = &review.Session{}
	choices := []guideconfig.Selection{
		{Provider: "openai", Model: "gpt-one", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"},
		{Provider: "anthropic", Model: "claude-one", Destination: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY"},
	}
	m.SetGuideOptions(choices, choices[0], nil)
	m.SetGuideLifecycle(func(context.Context, *review.Session, guideconfig.Selection, func(string)) (*review.Session, error) {
		return nil, nil
	})
	m.lifecycleKey("g")
	m.guideConsentKey("tab")
	m.guideConsentKey("right")
	m.guideConsentKey("tab")
	m.guideConsentKey("X")
	m.guideConsentKey("esc")
	m.lifecycleKey("g")
	if got := m.currentGuideSelection(); got.Provider != "openai" || got.Model != "gpt-one" {
		t.Fatalf("cancelled draft reused on reopen: %+v", got)
	}
}

func TestGuideModalModelAcceptsQuitAndThemeLetters(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 80, 24
	m.Session = &review.Session{}
	s := guideconfig.Selection{Provider: "openai", Model: "gpt", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{s}, s, nil)
	m.push(pageGuideConsent)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	for _, key := range "qt" {
		m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
	}
	if m.top() != pageGuideConsent || m.currentGuideSelection().Model != "gptqt" {
		t.Fatalf("model letters triggered global shortcuts: page=%v choice=%+v", m.top(), m.currentGuideSelection())
	}
}

func TestGuideModalCannotConfirmWhenConsentDoesNotFit(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 30, 9
	m.Session = &review.Session{}
	s := guideconfig.Selection{Provider: "openai", Model: "gpt", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{s}, s, nil)
	m.SetGuideLifecycle(func(context.Context, *review.Session, guideconfig.Selection, func(string)) (*review.Session, error) {
		return nil, nil
	})
	m.push(pageGuideConsent)
	if cmd := m.guideConsentKey("enter"); cmd != nil || m.top() != pageGuideConsent || !strings.Contains(m.View().Content, "Resize") {
		t.Fatalf("small terminal permitted undisclosed upload: %q", m.View().Content)
	}
	m.Width, m.Height = 80, 24
	if cmd := m.guideConsentKey("enter"); cmd == nil {
		t.Fatal("confirmation remained disabled after resizing")
	}
}

func TestGuideModalDisabledAndSaveFailure(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 80, 24
	m.Session = &review.Session{}
	m.SetGuideOptions(nil, guideconfig.Selection{}, nil)
	m.push(pageGuideConsent)
	if cmd := m.guideConsentKey("enter"); cmd != nil || m.top() != pageGuideConsent || !strings.Contains(m.View().Content, "Provider:") || !strings.Contains(m.View().Content, "Model:") {
		t.Fatal("empty provider set was confirmable or invisible")
	}
	m.pop()
	s := guideconfig.Selection{Provider: "openai", Model: "gpt-test", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{s}, s, func(guideconfig.Selection) error { return errors.New("write failed") })
	m.SetGuideLifecycle(func(context.Context, *review.Session, guideconfig.Selection, func(string)) (*review.Session, error) {
		return nil, nil
	})
	m.push(pageGuideConsent)
	if cmd := m.guideConsentKey("enter"); cmd == nil || m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "remember") {
		t.Fatal("save failure was not reported while request proceeded")
	}
}

func TestGuideModalOverlayAndEscapedSelection(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Width, m.Height = 74, 22
	m.Session = &review.Session{}
	s := guideconfig.Selection{Provider: "openai", Model: "gpt-test\x1b[31m", Destination: "https://api.openai.com", APIKeyEnv: "OPENAI_API_KEY"}
	m.SetGuideOptions([]guideconfig.Selection{s}, s, nil)
	m.push(pageGuideConsent)
	view := m.View().Content
	if !strings.Contains(view, "Generate guide?") || !strings.Contains(view, "Provider: openai") || strings.Contains(view, "\x1b[31m") {
		t.Fatalf("overlay or escaping failed: %q", view)
	}
	if !strings.Contains(view, "gpt-test\\x1b[31m") {
		t.Fatalf("model control was not escaped: %q", view)
	}
	m.guideConsentKey("esc")
	if m.top() != pageReview {
		t.Fatal("escape did not restore review")
	}
	for _, size := range [][2]int{{30, 9}, {18, 6}} {
		small := renderGuideConsentModal(size[0], size[1], "background", "Generate guide?\nProvider: openai\nModel: gpt-test")
		if !strings.Contains(small, "Provider:") || !strings.Contains(small, "Model:") {
			t.Fatalf("selection vanished at %v: %q", size, small)
		}
	}
}
