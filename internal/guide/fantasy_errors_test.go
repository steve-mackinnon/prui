package guide

import (
	"context"
	"fmt"
	"net"
	"testing"

	"charm.land/fantasy"
)

func TestFantasySafeProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		want    string
	}{
		{"quota", &fantasy.ProviderError{StatusCode: 429, ResponseBody: []byte(`{"error":{"code":"insufficient_quota","message":"secret source"}}`)}, "API quota exhausted; check billing and credits"},
		{"expired credits", &fantasy.ProviderError{StatusCode: 400, ResponseBody: []byte(`{"error":{"code":"credit_balance_too_low","message":"secret"}}`)}, "API credits unavailable; check billing and credit balance"},
		{"anthropic credits", &fantasy.ProviderError{StatusCode: 400, ResponseBody: []byte("HTTP/1.1 400 Bad Request\r\nContent-Type: application/json\r\n\r\n" + `{"error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API. secret"}}`)}, "API credits unavailable; check billing and credit balance"},
		{"authentication", &fantasy.ProviderError{StatusCode: 401, Message: "secret"}, "API authentication failed; check or renew credentials"},
		{"permission", &fantasy.ProviderError{StatusCode: 403}, "API access denied; check model and account permissions"},
		{"rate limit", &fantasy.ProviderError{StatusCode: 429}, "API rate limit reached; try again later"},
		{"server", &fantasy.ProviderError{StatusCode: 503}, "guide provider temporarily unavailable; try again later"},
		{"model", &fantasy.ProviderError{StatusCode: 404}, "API model or endpoint not found; check guide settings"},
		{"context", &fantasy.ProviderError{ContextTooLargeErr: true}, "guide input exceeds the model context limit; choose a larger model"},
		{"timeout", &net.DNSError{IsTimeout: true, Err: "secret"}, "guide provider request timed out; try again later"},
		{"network", &net.DNSError{Err: "secret"}, "cannot connect to guide provider; check network and endpoint"},
		{"unknown code", &fantasy.ProviderError{StatusCode: 400, ResponseBody: []byte(`{"error":{"code":"secret","message":"secret"}}`)}, "API rejected the guide request; check model and provider settings"},
		{"unknown", fmt.Errorf("secret insufficient_quota"), "guide provider request failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newFantasy(fixtureLanguageModel{err: fmt.Errorf("wrapped: %w", tc.failure)}, "fixture", "m")
			b, err := a.Analyze(context.Background(), Input{})
			if err != nil || b.Status != Unavailable || b.Reason != tc.want || len(b.Items) != 0 {
				t.Fatalf("bundle=%+v err=%v, want %q", b, err, tc.want)
			}
		})
	}
}
