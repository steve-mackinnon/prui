package guide

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"

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
}

// FantasyOptions are invocation-local. APIKey is held only in memory and is
// never copied into a guide bundle, config file, or cache fingerprint.
type FantasyOptions struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

func newFantasy(model fantasy.LanguageModel, provider, modelID string) *Fantasy {
	return &Fantasy{model: model, provider: provider, modelID: modelID}
}

func (f *Fantasy) Analyze(ctx context.Context, in Input) (Bundle, error) {
	if f == nil || f.model == nil {
		return Fallback("guide model is unavailable"), nil
	}
	response, err := f.model.GenerateObject(ctx, fantasy.ObjectCall{
		Prompt:     fantasy.Prompt{fantasy.NewUserMessage(prompt(in))},
		Schema:     fantasyschema.Generate(reflect.TypeOf(structured{})),
		SchemaName: SchemaName,
	})
	if err != nil {
		if ctx.Err() != nil {
			return Bundle{}, ctx.Err()
		}
		// SDK and provider errors may include a request URL, source excerpt, or
		// credential. None belongs in a saved session or terminal view.
		return Fallback("guide provider request failed"), nil
	}
	if response == nil || (response.FinishReason != fantasy.FinishReasonStop && response.FinishReason != fantasy.FinishReasonToolCalls) {
		return Fallback("guide provider returned incomplete output"), nil
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
