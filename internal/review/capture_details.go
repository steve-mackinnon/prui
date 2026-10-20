package review

import (
	"context"

	"prui/internal/commits"
	reviewcontext "prui/internal/context"
)

// Both readers use the already-pinned view and independent budgets. Join the
// evidence reader on every path before the caller can close or retry that view.
// Unadvertised runners keep the original serial ordering.
func captureDetails(ctx context.Context, parallel bool, capture func(context.Context) (*commits.Bundle, error), retrieve func(context.Context) reviewcontext.ContextBundle) (*commits.Bundle, reviewcontext.ContextBundle, error) {
	if !parallel {
		bundle, err := capture(ctx)
		if err != nil {
			return nil, reviewcontext.ContextBundle{}, err
		}
		return bundle, retrieve(ctx), nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ready := make(chan reviewcontext.ContextBundle, 1)
	go func() { ready <- retrieve(ctx) }()
	bundle, err := capture(ctx)
	if err != nil {
		cancel()
	}
	evidence := <-ready
	return bundle, evidence, err
}
