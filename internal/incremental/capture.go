// Package incremental captures prior-head comparisons and assesses anchors.
// It never reads drafts, sends source to a model, or performs remote writes.
package incremental

import (
	"context"
	"errors"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/source"
)

type Bundle struct {
	PreviousID, PreviousReference string
	PreviousGeneration            uint64
	Previous, Current             source.Metadata
	Status, Reason, Relation      string
	Diff                          *commits.Diff
}

// Capture builds a direct old-head/new-head tree diff, without a merge base.
// Missing objects or limits produce an explicit unavailable comparison.
func Capture(parent context.Context, v *source.View, id, reference string, old, next source.Metadata, gh source.GitHub, l source.Limits, notify func(string)) (*Bundle, error) {
	ctx, cancel := context.WithTimeout(parent, l.Operation)
	defer cancel()
	b := &Bundle{PreviousID: id, PreviousReference: reference, Previous: old, Current: next, Status: "unavailable", Reason: "Prior-head comparison unavailable; no progress proof."}
	relation, err := v.PrepareHeadComparison(ctx, old, next, gh, notify)
	if err == nil {
		b.Relation = relation
		var inv inventory.Inventory
		inv, err = inventory.BuildCommit(ctx, v, inventory.CommitComparison{ParentSHA: old.HeadSHA, CommitSHA: next.HeadSHA}, l)
		if err == nil {
			b.Status, b.Reason = "captured", ""
			b.Diff = &commits.Diff{Files: inv.Files, Units: inv.Units, Patches: inv.Patches, Syntax: inv.Syntax, Complete: inv.Complete, Problems: inv.Problems}
		}
	}
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	// A pre-existing view normally needs no fetch. If capture fetched a missing
	// old head, recheck the current full pins before reporting this capture.
	if gh != nil {
		current, e := gh.Metadata(parent, next.Identity)
		if e != nil {
			b.Status, b.Reason, b.Diff = "unavailable", "Comparison freshness check unavailable.", nil
		} else if !source.SamePinnedRevision(current, next) {
			return nil, commits.ErrRevisionChanged
		}
	}
	if err != nil && errors.Is(err, source.ErrLimit) {
		b.Reason = "Prior-head comparison exceeded resource limits; no progress proof."
	}
	return b, nil
}
