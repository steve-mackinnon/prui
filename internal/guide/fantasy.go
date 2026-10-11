package guide

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unicode"

	"charm.land/fantasy"
	fantasyschema "charm.land/fantasy/schema"
)

// Fantasy analyzes one bounded guide input through a provider language model.
// Provider constructors supply only the protocol; this type owns the shared
// prompt, schema, validation boundary, and safe failure vocabulary.
type Fantasy struct {
	model    fantasy.LanguageModel
	provider string
	modelID  string
	// apiKey is kept only to redact it from relayed provider errors.
	apiKey string
}

// FantasyOptions are invocation-local. APIKey is held only in memory and is
// never copied into a guide bundle, config file, or cache fingerprint.
type FantasyOptions struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

func newFantasy(model fantasy.LanguageModel, provider, modelID, apiKey string) *Fantasy {
	return &Fantasy{model: model, provider: provider, modelID: modelID, apiKey: strings.TrimSpace(apiKey)}
}

func (f *Fantasy) Analyze(ctx context.Context, in Input) (Bundle, error) {
	if f == nil || f.model == nil {
		return Fallback("guide model is unavailable"), nil
	}
	maxTokens := int64(maxOutputTokens)
	response, err := f.model.GenerateObject(ctx, fantasy.ObjectCall{
		Prompt:          fantasy.Prompt{fantasy.NewUserMessage(prompt(in))},
		Schema:          fantasyschema.Generate(reflect.TypeOf(structured{})),
		SchemaName:      SchemaName,
		MaxOutputTokens: &maxTokens,
	})
	if err != nil {
		if ctx.Err() != nil {
			return Bundle{}, ctx.Err()
		}
		// SDK and provider errors may include a request URL, source excerpt, or
		// credential. None belongs in a saved session or terminal view.
		// Only the provider's own error message is relayed, sanitized.
		if detail := providerErrorDetail(err, f.apiKey); detail != "" {
			return Fallback("guide provider request failed: " + detail), nil
		}
		return Fallback("guide provider request failed"), nil
	}
	if response != nil && response.FinishReason == fantasy.FinishReasonLength {
		return Fallback("guide provider output hit the output token limit"), nil
	}
	if response == nil {
		return Fallback("guide provider returned incomplete output"), nil
	}
	if response.FinishReason != fantasy.FinishReasonStop && response.FinishReason != fantasy.FinishReasonToolCalls {
		// FinishReason is a fixed SDK enum, so it is safe to relay.
		return Fallback(fmt.Sprintf("guide provider returned incomplete output (finish reason: %s)", response.FinishReason)), nil
	}
	encoded, err := json.Marshal(response.Object)
	if err != nil || len(encoded) > maxResponseBytes {
		return Fallback("guide provider returned invalid output"), nil
	}
	var output structured
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return Fallback("guide provider returned invalid output"), nil
	}
	items := make([]Item, 0, len(output.Guides))
	for _, g := range output.Guides {
		item := Item{Title: g.Title, Description: g.Description}
		for _, s := range g.Sections {
			item.Sections = append(item.Sections, Section{Title: s.Title, Description: s.Description, UnitIDs: s.UnitIDs})
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return Fallback("guide provider returned no guides"), nil
	}
	return Bundle{Status: Generated, Items: items, Provider: f.provider, Model: f.modelID, PromptVersion: PromptVersion, SchemaName: SchemaName}, nil
}

var _ Analyzer = (*Fantasy)(nil)

const maxErrorDetail = 200

// maxOutputTokens bounds guide output. Anthropic's default of 4096 truncates
// guides for large PRs; its SDK rejects non-streaming requests above ~21k.
const maxOutputTokens = 16384

// providerErrorDetail returns the HTTP status and the provider's own error
// message from the response body. The request URL, request body, and SDK wrapping
// text are never used. The API key is redacted, control characters are
// dropped, and the result is truncated.
func providerErrorDetail(err error, apiKey string) string {
	var pe *fantasy.ProviderError
	if !errors.As(err, &pe) || pe.StatusCode == 0 {
		return ""
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	// Anthropic and OpenAI store a full HTTP dump; the JSON follows the headers.
	raw := pe.ResponseBody
	if _, after, ok := bytes.Cut(raw, []byte("\r\n\r\n")); ok {
		raw = after
	}
	msg := ""
	if json.Unmarshal(raw, &body) == nil {
		msg = body.Error.Message
	} else if pe.URL == "" && len(pe.RequestBody) == 0 {
		// Google stores only the server's message text, with no request data.
		msg = string(raw)
	}
	if apiKey != "" {
		msg = strings.ReplaceAll(msg, apiKey, "[redacted]")
	}
	msg = strings.Join(strings.FieldsFunc(msg, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if r := []rune(msg); len(r) > maxErrorDetail {
		msg = string(r[:maxErrorDetail]) + "…"
	}
	if msg == "" {
		return fmt.Sprintf("HTTP %d", pe.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", pe.StatusCode, msg)
}
