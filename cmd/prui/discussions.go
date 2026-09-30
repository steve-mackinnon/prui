package main

import (
	"context"
	"errors"
	"time"

	"prui/internal/review"
	"prui/internal/source"
	"prui/internal/tui"
)

// listDiscussions keeps historical material readable while allowing current-diff
// placement only when the live PR still matches the immutable comparison.
func (a *application) listDiscussions(ctx context.Context, s *review.Session) (tui.DiscussionSnapshot, error) {
	if err := a.online(ctx); err != nil {
		return tui.DiscussionSnapshot{}, err
	}
	if s == nil {
		return tui.DiscussionSnapshot{}, errors.New("discussion review unavailable")
	}
	if a.setupError != nil {
		return tui.DiscussionSnapshot{}, a.setupError
	}
	reader, ok := a.gh.(source.DiscussionReader)
	if !ok {
		return tui.DiscussionSnapshot{}, errors.New("GitHub discussions unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	frozen := s.Inventory.Comparison.Metadata
	before, beforeErr := a.Metadata(ctx, frozen.Identity)
	snapshot, err := reader.ListDiscussions(ctx, frozen.Identity)
	if err != nil {
		return tui.DiscussionSnapshot{}, err
	}
	after, afterErr := a.Metadata(ctx, frozen.Identity)
	verified := beforeErr == nil && afterErr == nil && source.SamePinnedRevision(before, frozen) && source.SamePinnedRevision(after, frozen)
	result := tui.DiscussionSnapshot{Snapshot: snapshot, CurrentVerified: verified}
	if !verified {
		result.Reason = "Discussions from live PR · snapshot differs"
		if beforeErr != nil || afterErr != nil {
			result.Reason = "Discussions from live PR · snapshot freshness unknown"
		}
	}
	return result, nil
}
