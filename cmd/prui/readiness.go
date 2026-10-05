package main

import (
	"context"
	"errors"
	"prui/internal/source"
)

func (a *application) readReadiness(ctx context.Context, id source.Identity) (source.Readiness, error) {
	if err := a.online(ctx); err != nil {
		return source.Readiness{}, err
	}
	if a.setupError != nil {
		return source.Readiness{}, a.setupError
	}
	reader, ok := a.gh.(source.ReadinessReader)
	if !ok {
		return source.Readiness{}, errors.New("GitHub readiness unavailable")
	}
	return reader.ReadReadiness(ctx, id)
}
