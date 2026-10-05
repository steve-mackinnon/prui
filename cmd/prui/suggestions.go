package main

import (
	"context"
	"errors"
	"prui/internal/source"
)

func (a *application) prepareSuggestion(ctx context.Context, m source.Metadata, c source.ReviewComment, before string) (source.SuggestionApplication, error) {
	if err := a.online(ctx); err != nil {
		return source.SuggestionApplication{}, err
	}
	if a.setupError != nil {
		return source.SuggestionApplication{}, a.setupError
	}
	applier, ok := a.gh.(source.SuggestionApplier)
	if !ok {
		return source.SuggestionApplication{}, errors.New("suggestion application unavailable")
	}
	return applier.PrepareSuggestion(ctx, m, c, before)
}
func (a *application) applySuggestion(ctx context.Context, s source.SuggestionApplication) (source.SuggestionResult, error) {
	if err := a.online(ctx); err != nil {
		return source.SuggestionResult{}, err
	}
	if a.setupError != nil {
		return source.SuggestionResult{}, a.setupError
	}
	applier, ok := a.gh.(source.SuggestionApplier)
	if !ok {
		return source.SuggestionResult{}, errors.New("suggestion application unavailable")
	}
	return applier.ApplySuggestion(ctx, s)
}
