package guide

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"charm.land/fantasy/providers/anthropic"
)

// DefaultAnthropicModel is used when an Anthropic model is not selected.
const DefaultAnthropicModel = "claude-sonnet-4-20250514"

// NewFantasyAnthropic builds the native Anthropic guide analyzer. The SDK's
// object mode uses one forced tool call, and its provider disables retries.
func NewFantasyAnthropic(o FantasyOptions) (*Fantasy, error) {
	key := strings.TrimSpace(o.APIKey)
	if key == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required to send source for analysis")
	}
	modelID := strings.TrimSpace(o.Model)
	if modelID == "" {
		modelID = DefaultAnthropicModel
	}
	baseURL := strings.TrimSpace(o.BaseURL)
	if baseURL == "" {
		baseURL = anthropic.DefaultURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "https" && (u.Scheme != "http" || !guideLoopback(u.Hostname()))) {
		return nil, errors.New("invalid Anthropic endpoint")
	}
	provider, err := anthropic.New(
		anthropic.WithAPIKey(key),
		anthropic.WithBaseURL(strings.TrimRight(baseURL, "/")),
		anthropic.WithHTTPClient(newSingleRequestHTTPClient(o.Client)),
	)
	if err != nil {
		return nil, errors.New("anthropic guide provider is unavailable")
	}
	model, err := provider.LanguageModel(context.Background(), modelID)
	if err != nil {
		return nil, errors.New("anthropic guide model is unavailable")
	}
	return newFantasy(model, anthropic.Name, modelID), nil
}
