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
		snapshot.Complete = false
		snapshot.Reason = "Inline discussions unavailable"
	}
	if conversation, ok := a.gh.(source.ConversationReader); ok {
		activity, activityErr := conversation.ListConversation(ctx, frozen.Identity)
		snapshot.Timeline = true
		snapshot.Events = activity.Events
		if activityErr != nil || !activity.Complete {
			snapshot.Complete = false
			snapshot.Reason = activity.Reason
		}
	}
	if err != nil && len(snapshot.Events) == 0 && len(snapshot.Threads) == 0 {
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

// General comments have no code anchor but retain the same freshness boundary.
func (a *application) submitGeneralComment(ctx context.Context, frozen source.Metadata, body string) (source.ConversationEvent, error) {
	if err := a.online(ctx); err != nil {
		return source.ConversationEvent{}, err
	}
	if !validIdentity(frozen.Identity) || !validSHA(frozen.BaseSHA) || !validSHA(frozen.HeadSHA) || !validRepository(frozen.BaseRepository) || !validRepository(frozen.HeadRepository) {
		return source.ConversationEvent{}, errors.New("invalid PR comment comparison")
	}
	if a.setupError != nil {
		return source.ConversationEvent{}, a.setupError
	}
	if err := source.ValidateGeneralComment(body); err != nil {
		return source.ConversationEvent{}, err
	}
	writer, ok := a.gh.(source.GeneralCommentWriter)
	if !ok {
		return source.ConversationEvent{}, errors.New("PR comment submission unavailable")
	}
	current, err := a.Metadata(ctx, frozen.Identity)
	if err != nil {
		return source.ConversationEvent{}, err
	}
	if !source.SamePinnedRevision(current, frozen) {
		return source.ConversationEvent{}, errors.New("pull request changed; open a new comparison before posting a PR comment")
	}
	event, err := writer.CreateGeneralComment(ctx, frozen.Identity, body)
	if err != nil {
		return source.ConversationEvent{}, errors.Join(source.ErrCommentDeliveryUnknown, err)
	}
	return event, nil
}
