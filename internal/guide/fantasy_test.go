package guide

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/fantasy"
)

type fixtureLanguageModel struct {
	fantasy.LanguageModel
	response *fantasy.ObjectResponse
	err      error
}

func (m fixtureLanguageModel) GenerateObject(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error) {
	return m.response, m.err
}

func TestFantasyRejectsUnusableObjectsWithoutLeakingProviderText(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *fantasy.ObjectResponse
		err      error
	}{
		{"provider error", nil, errors.New("fake-key and source excerpt")},
		{"refusal", nil, errors.New("refused fake-key")},
		{"incomplete", &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonLength}, nil},
		{"malformed schema", &fantasy.ObjectResponse{Object: map[string]any{"unexpected": "fake-key"}, FinishReason: fantasy.FinishReasonStop}, nil},
		{"no guides", &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonStop}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newFantasy(fixtureLanguageModel{response: tc.response, err: tc.err}, "fixture", "m", "")
			b, err := a.Analyze(context.Background(), Input{})
			if err != nil || b.Status != Unavailable || strings.Contains(b.Reason, "fake-key") || strings.Contains(b.Reason, "source excerpt") {
				t.Fatalf("unsafe object accepted: bundle=%#v err=%v", b, err)
			}
		})
	}
}

func TestFantasyRelaysSanitizedProviderError(t *testing.T) {
	err := fmt.Errorf("tool-based generation failed: %w", &fantasy.ProviderError{
		Message:      "POST https://example.invalid/v1/messages?source=excerpt",
		URL:          "https://example.invalid/v1/messages",
		StatusCode:   400,
		RequestBody:  []byte("source excerpt"),
		ResponseBody: []byte("HTTP/1.1 400 Bad Request\r\nX-Source: excerpt\r\n\r\n" + `{"type":"error","error":{"type":"invalid_request_error","message":"bad\nfake-key ` + strings.Repeat("x", 300) + `"}}`),
	})
	a := newFantasy(fixtureLanguageModel{err: err}, "fixture", "m", "fake-key")
	b, aerr := a.Analyze(context.Background(), Input{})
	if aerr != nil || b.Status != Unavailable {
		t.Fatalf("bundle=%#v err=%v", b, aerr)
	}
	if !strings.HasPrefix(b.Reason, "guide provider request failed: HTTP 400: bad [redacted] x") {
		t.Fatalf("provider message not relayed: %q", b.Reason)
	}
	for _, leak := range []string{"fake-key", "source excerpt", "example.invalid", "\n"} {
		if strings.Contains(b.Reason, leak) {
			t.Fatalf("reason leaked %q: %q", leak, b.Reason)
		}
	}
	if n := len([]rune(b.Reason)); n > len("guide provider request failed: HTTP 400: ")+maxErrorDetail+1 {
		t.Fatalf("reason not truncated: %d runes", n)
	}
}

func TestFantasyRelaysStatusWithoutProviderMessage(t *testing.T) {
	err := &fantasy.ProviderError{StatusCode: 503, URL: "https://example.invalid", ResponseBody: []byte("HTTP/1.1 503\r\n\r\n<html>source excerpt</html>")}
	b, _ := newFantasy(fixtureLanguageModel{err: err}, "fixture", "m", "").Analyze(context.Background(), Input{})
	if b.Reason != "guide provider request failed: HTTP 503" {
		t.Fatalf("reason = %q", b.Reason)
	}
}

func TestFantasyRelaysGoogleProviderMessage(t *testing.T) {
	err := &fantasy.ProviderError{StatusCode: 404, Message: "models/x is not found", ResponseBody: []byte("models/x is not found")}
	b, _ := newFantasy(fixtureLanguageModel{err: err}, "fixture", "m", "").Analyze(context.Background(), Input{})
	if b.Reason != "guide provider request failed: HTTP 404: models/x is not found" {
		t.Fatalf("reason = %q", b.Reason)
	}
}

func TestFantasyReportsOutputTokenLimit(t *testing.T) {
	resp := &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonLength}
	b, _ := newFantasy(fixtureLanguageModel{response: resp}, "fixture", "m", "").Analyze(context.Background(), Input{})
	if b.Status != Unavailable || b.Reason != "guide provider output hit the output token limit" {
		t.Fatalf("bundle = %#v", b)
	}
}

func TestFantasyReportsFinishReason(t *testing.T) {
	resp := &fantasy.ObjectResponse{Object: map[string]any{"guides": []any{}}, FinishReason: fantasy.FinishReasonContentFilter}
	b, _ := newFantasy(fixtureLanguageModel{response: resp}, "fixture", "m", "").Analyze(context.Background(), Input{})
	if b.Reason != "guide provider returned incomplete output (finish reason: content-filter)" {
		t.Fatalf("reason = %q", b.Reason)
	}
}
