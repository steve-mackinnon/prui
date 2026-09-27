package guide

import (
	"context"
	"errors"
	"strings"

	"charm.land/fantasy/providers/google"
)

// NewFantasyGoogle uses Gemini's native JSON-schema object mode for one guide
// generation request. Credentials and the HTTP client live only in memory.
func NewFantasyGoogle(o FantasyOptions) (*Fantasy, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, errors.New("GEMINI_API_KEY is required to send source for analysis")
	}
	if strings.TrimSpace(o.Model) == "" {
		return nil, errors.New("a Google model is required for guide generation")
	}
	opts := []google.Option{
		google.WithGeminiAPIKey(o.APIKey),
		google.WithHTTPClient(newSingleRequestHTTPClient(o.Client)),
	}
	// The underlying GenAI SDK reads GOOGLE_GEMINI_BASE_URL and process-wide
	// defaults when BaseURL is empty. Pin the normal endpoint explicitly so an
	// unrelated environment setting cannot silently redirect uploaded source.
	baseURL := strings.TrimSpace(o.BaseURL)
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	opts = append(opts, google.WithBaseURL(baseURL))
	provider, err := google.New(opts...)
	if err != nil {
		return nil, errors.New("guide provider could not be configured")
	}
	model, err := provider.LanguageModel(context.Background(), o.Model)
	if err != nil {
		return nil, errors.New("guide model could not be configured")
	}
	return newFantasy(model, "google", o.Model), nil
}
