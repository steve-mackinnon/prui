package source

import (
	"context"
	"time"
)

// pinInputs overlaps independent acquisition only for runners that explicitly
// support parallel use. Pin validation and fetch wait for both inputs to succeed.
// Metadata errors precede setup errors; parent cancellation remains authoritative.
// Every speculative view is joined and cleaned before returning an error.
func pinInputs(parent context.Context, parallel bool, read func(context.Context) (Metadata, error), prepare func(context.Context) (*View, error)) (Metadata, *View, time.Duration, error) {
	if err := parent.Err(); err != nil {
		return Metadata{}, nil, 0, err
	}
	type prepared struct {
		view    *View
		err     error
		elapsed time.Duration
	}
	setup := func(ctx context.Context) prepared {
		started := time.Now()
		v, err := prepare(ctx)
		if err != nil && v != nil {
			_ = v.Close()
			v = nil
		}
		return prepared{v, err, time.Since(started)}
	}
	if !parallel {
		m, err := read(parent)
		if err != nil {
			return Metadata{}, nil, 0, err
		}
		result := setup(parent)
		return m, result.view, result.elapsed, result.err
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	ready := make(chan prepared, 1)
	go func() { ready <- setup(ctx) }()
	m, err := read(ctx)
	if err != nil {
		cancel()
	}
	result := <-ready
	if parent.Err() != nil {
		err = parent.Err()
	}
	if err != nil {
		if result.view != nil {
			_ = result.view.Close()
		}
		return Metadata{}, nil, result.elapsed, err
	}
	return m, result.view, result.elapsed, result.err
}
