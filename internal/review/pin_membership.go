package review

import (
	"context"

	"prui/internal/commits"
	"prui/internal/source"
)

// Membership needs the initial metadata, but not the local ancestry result.
// Keep its original deadlines running through setup, inventory and capture.
func pinWithMembership(ctx context.Context, checkout string, id source.Identity, gh source.GitHub, r source.Runner, l source.Limits, notify func(string), timing *source.PinTiming, parallel bool) (*source.View, source.PinnedComparison, *commits.PendingCapture, error) {
	var pending *commits.PendingCapture
	var first source.Metadata
	seen := false
	pinGH := gh
	if parallel {
		pinGH = metadataStarted{GitHub: gh, started: func(m source.Metadata) {
			if !seen && m.Identity == id {
				seen, first = true, m
				// Pin's metadata context is canceled after input setup, even on
				// success. The review context owns the preparation instead.
				pending = commits.PrepareCapture(ctx, source.PinnedComparison{Metadata: m}, gh)
			}
		}}
	}
	v, p, err := source.PinWithTiming(ctx, checkout, id, pinGH, r, l, notify, timing)
	if err != nil {
		pending.Close()
		return nil, p, nil, err
	}
	if parallel && (!seen || !source.SamePinnedRevision(first, p.Metadata)) {
		// Fetch can discover a changed revision. Never associate the earlier
		// membership with new pins, and join it before starting another read.
		pending.Close()
		pending = commits.PrepareCapture(ctx, p, gh)
	}
	return v, p, pending, nil
}

type metadataStarted struct {
	source.GitHub
	started func(source.Metadata)
}

func (g metadataStarted) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	m, err := g.GitHub.Metadata(ctx, id)
	if err == nil {
		g.started(m)
	}
	return m, err
}
