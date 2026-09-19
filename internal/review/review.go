package review

import (
	"context"
	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"time"
)

type Slice = session.Slice
type Session = session.Record

// Config carries the optional decisions of one invocation. A zero value is the
// deterministic, upload-free review: no exclusions beyond the defaults and no
// analysis.
type Config struct {
	Policy   privacy.Policy
	Analyzer guide.Analyzer // nil means no analysis for this invocation
	Timing   *Timing
}

// Timing records optional stage durations for one comparison opening.
type Timing struct {
	Pin       source.PinTiming
	Inventory time.Duration
	Evidence  time.Duration
}

func Open(ctx context.Context, checkout string, id source.Identity, gh source.GitHub, r source.Runner, l source.Limits, notify func(string)) (*Session, error) {
	return OpenWithConfig(ctx, checkout, id, gh, r, l, notify, Config{})
}

func OpenWithConfig(ctx context.Context, checkout string, id source.Identity, gh source.GitHub, r source.Runner, l source.Limits, notify func(string), cfg Config) (*Session, error) {
	var pinTiming *source.PinTiming
	if cfg.Timing != nil {
		pinTiming = &cfg.Timing.Pin
	}
	v, p, e := source.PinWithTiming(ctx, checkout, id, gh, r, l, notify, pinTiming)
	if e != nil {
		return nil, e
	}
	defer func() { _ = v.Close() }()
	started := time.Now()
	inv, e := inventory.Build(ctx, v, p, l)
	if cfg.Timing != nil {
		cfg.Timing.Inventory += time.Since(started)
	}
	if e != nil {
		return nil, e
	}
	started = time.Now()
	contextBundle := reviewcontext.Retrieve(ctx, v, p, inv, cfg.Policy, reviewcontext.Defaults)
	if cfg.Timing != nil {
		cfg.Timing.Evidence += time.Since(started)
	}
	// Analysis consumes only frozen material and runs before slicing, so it can
	// interpret the change but never influence file ownership or progress.
	b := guide.Analyze(ctx, cfg.Analyzer, inv, guide.InputFrom(inv, contextBundle, cfg.Policy, guide.Defaults))
	s := &Session{Snapshot: session.Snapshot{Inventory: inv, Checkout: []byte(checkout), Slices: make([]Slice, len(inv.Files)), UnitFiles: make([]int, len(inv.Units))}}
	s.Context = contextBundle
	s.Guides = &b
	index := map[string]int{}
	for i, f := range inv.Files {
		index[f.ID] = i
		s.Slices[i] = Slice{FileID: f.ID, Units: []int{}}
	}
	for i, u := range inv.Units {
		f := index[u.FileChangeID]
		s.Slices[f].Units = append(s.Slices[f].Units, i)
		s.UnitFiles[i] = f
	}
	return s, nil
}

// DeriveGuide adds a guide bundle to a copy of an already-pinned snapshot.
// It never reads the checkout or remote metadata.
func DeriveGuide(ctx context.Context, original *Session, analyzer guide.Analyzer, cfg Config) session.Snapshot {
	derived := original.Snapshot
	bundle := guide.Analyze(ctx, analyzer, original.Inventory, guide.InputFrom(original.Inventory, original.Context, cfg.Policy, guide.Defaults))
	derived.Guides = &bundle
	derived.DerivedFrom = original.ID
	return derived
}
