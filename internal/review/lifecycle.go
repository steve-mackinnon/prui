package review

import (
	"context"
	"errors"
	"slices"

	"prui/internal/session"
	"prui/internal/source"
)

type MetadataReader interface {
	Metadata(context.Context, source.Identity) (source.Metadata, error)
}

func Mark(ctx context.Context, store *session.Store, s *Session, sliceID string, reviewed bool) error {
	if !slices.ContainsFunc(s.Slices, func(slice Slice) bool { return slice.FileID == sliceID }) {
		return errors.New("unknown slice")
	}
	next := *s
	next.ReviewedSliceIDs = slices.Clone(s.ReviewedSliceIDs)
	i := slices.Index(next.ReviewedSliceIDs, sliceID)
	if reviewed && i < 0 {
		next.ReviewedSliceIDs = append(next.ReviewedSliceIDs, sliceID)
	}
	if !reviewed && i >= 0 {
		next.ReviewedSliceIDs = slices.Delete(next.ReviewedSliceIDs, i, i+1)
	}
	state, err := store.UpdateState(ctx, s.ID, s.Generation, s.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: next.ReviewedSliceIDs, RevisionStatus: next.RevisionStatus})
	if err != nil {
		return err
	}
	s.State = state
	return nil
}

func Refresh(ctx context.Context, store *session.Store, s *Session, gh MetadataReader) error {
	next := *s
	next.RevisionStatus = session.CheckFailed
	old := s.Inventory.Comparison.Metadata
	if gh != nil {
		m, err := gh.Metadata(ctx, old.Identity)
		if err == nil {
			next.RevisionStatus = session.Stale
			if source.SamePinnedRevision(m, old) {
				next.RevisionStatus = session.Current
			}
		}
	}
	state, err := store.UpdateState(ctx, s.ID, s.Generation, s.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: next.ReviewedSliceIDs, RevisionStatus: next.RevisionStatus})
	if err != nil {
		return err
	}
	s.State = state
	return nil
}

func Resume(ctx context.Context, store *session.Store, id string, gh MetadataReader) (*Session, error) {
	s, err := store.Load(id)
	if err != nil {
		return nil, err
	}
	// Persist unknown freshness before any cancellable network work.
	state, err := store.UpdateState(ctx, s.ID, s.Generation, s.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: s.ReviewedSliceIDs, RevisionStatus: session.Unchecked})
	if err != nil {
		return nil, err
	}
	s.State = state
	if gh != nil {
		err = Refresh(ctx, store, s, gh)
	}
	return s, err
}
