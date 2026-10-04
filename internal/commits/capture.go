package commits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"prui/internal/inventory"
	"prui/internal/source"
	"time"
)

var ErrRevisionChanged = errors.New("PR revisions changed during commit capture")

// Capture snapshots optional PR membership and first-parent material while the
// isolated view is open. Failure never substitutes checkout history for PR data.
func Capture(parent context.Context, v *source.View, p source.PinnedComparison, gh source.GitHub, l source.Limits, notify func(string)) (bundle *Bundle, err error) {
	return capture(parent, v, p, gh, l, notify, 55*time.Second, 60*time.Second)
}

// Reserve the last five seconds of the single overall deadline for freshness.
func capture(parent context.Context, v *source.View, p source.PinnedComparison, gh source.GitHub, l source.Limits, notify func(string), workDuration, totalDuration time.Duration) (bundle *Bundle, err error) {
	if parent.Err() != nil {
		return nil, parent.Err()
	}
	totalCtx, cancel := context.WithTimeout(parent, totalDuration)
	defer cancel()
	ctx, stopWork := context.WithTimeout(totalCtx, workDuration)
	defer stopWork()
	bundle = &Bundle{BaseSHA: p.Metadata.BaseSHA, HeadSHA: p.Metadata.HeadSHA, Status: Unavailable, Reason: "Commit capture unavailable."}
	defer func() {
		if parent.Err() != nil {
			bundle = nil
			err = parent.Err()
			return
		}
		latest, e := gh.Metadata(totalCtx, p.Metadata.Identity)
		if parent.Err() != nil {
			bundle = nil
			err = parent.Err()
			return
		}
		if e != nil {
			bundle = &Bundle{BaseSHA: p.Metadata.BaseSHA, HeadSHA: p.Metadata.HeadSHA, Status: Unavailable, Reason: "Commit freshness check unavailable."}
			return
		}
		if !source.SamePinnedRevision(latest, p.Metadata) {
			bundle = nil
			err = ErrRevisionChanged
		}
	}()
	reader, ok := gh.(source.PullRequestCommitReader)
	if !ok {
		bundle.Reason = "Commit capture is unavailable with this client."
		return bundle, nil
	}
	if notify != nil {
		notify("Capturing commits")
	}
	entries, e := reader.ListPullRequestCommits(ctx, p.Metadata.Identity)
	if e != nil || source.ValidatePullRequestCommits(entries) != nil {
		return bundle, nil
	}
	bundle.Status = Captured
	bundle.Reason = ""
	bundle.Complete = len(entries) < 100
	bundle.Entries = make([]Entry, len(entries))
	for i, e := range entries {
		bundle.Entries[i] = Entry{SHA: e.SHA, Subject: e.Subject, Author: e.Author, Status: Unavailable, Reason: "Commit diff unavailable."}
	}
	remaining := min(l.ContentBytes, 50<<20)
	metadata, _ := json.Marshal(bundle)
	remaining -= len(metadata)
	lines := min(l.DiffLines, 100000)
	budget := &captureObjects{view: v, remaining: remaining}
	for i := range bundle.Entries {
		entry := &bundle.Entries[i]
		if ctx.Err() != nil || budget.remaining <= 0 || lines <= 0 {
			entry.Reason = "Commit capture budget exhausted."
			continue
		}
		if v == nil {
			continue
		}
		parents, e := v.CommitParents(ctx, entry.SHA, p.Metadata.HeadSHA)
		if e != nil {
			continue
		}
		entry.Parents = parents
		var old string
		if len(parents) > 0 {
			old = parents[0]
		} else {
			old, e = v.EmptyTree(ctx)
		}
		if e != nil {
			continue
		}
		limits := l
		limits.ContentBytes = budget.remaining
		limits.DiffLines = lines
		inv, e := inventory.BuildCommit(ctx, budget, inventory.CommitComparison{ParentSHA: old, CommitSHA: entry.SHA}, limits)
		if e != nil {
			if errors.Is(e, source.ErrLimit) {
				entry.Reason = "Commit capture budget exhausted."
			}
			continue
		}
		diff := &Diff{Files: inv.Files, Units: inv.Units, Patches: inv.Patches, Syntax: inv.Syntax, Complete: inv.Complete, Problems: inv.Problems}
		// Include structural metadata as well as the bounded source read above.
		structural := *diff
		structural.Patches = nil
		structural.Syntax = nil
		encoded, _ := json.Marshal(structural)
		budget.remaining -= len(encoded) + len(parents)*40
		if budget.remaining < 0 {
			entry.Reason = "Commit capture budget exhausted."
			continue
		}
		for _, patch := range diff.Patches {
			lines -= bytes.Count(patch, []byte{'\n'})
		}
		entry.Diff = diff
		entry.Status = Captured
		entry.Reason = ""
	}
	return bundle, nil
}

type captureObjects struct {
	view      *source.View
	remaining int
}

func (o *captureObjects) Git(ctx context.Context, limit int, args ...string) ([]byte, error) {
	if o.remaining <= 0 {
		return nil, source.ErrLimit
	}
	data, err := o.view.Git(ctx, min(limit, o.remaining), args...)
	if errors.Is(err, source.ErrLimit) {
		o.remaining = 0
	}
	if err == nil {
		o.remaining -= len(data)
	}
	return data, err
}
func (o *captureObjects) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	if o.remaining <= 0 {
		return nil, source.ErrLimit
	}
	data, err := o.view.Blob(ctx, oid, min(limit, o.remaining))
	// A per-blob failure doesn't consume the aggregate budget unless it was the tighter bound.
	if errors.Is(err, source.ErrLimit) && o.remaining <= limit {
		o.remaining = 0
	}
	if err == nil {
		o.remaining -= len(data)
	}
	return data, err
}
