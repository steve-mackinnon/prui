package review

import (
	"context"
	"errors"
	"slices"

	"pr-review/internal/session"
	"pr-review/internal/source"
)

type MetadataReader interface {
	Metadata(context.Context, source.Identity) (source.Metadata, error)
}

func Mark(store *session.Store, s *Session, sliceID string, reviewed bool) error {
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
	if err := store.Save(&next); err != nil {
		return err
	}
	s.State = next.State
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
	if err := store.Save(&next); err != nil {
		return err
	}
	s.State = next.State
	return nil
}

func Resume(ctx context.Context, store *session.Store, id string, gh MetadataReader) (*Session, error) {
	s, err := store.Load(id)
	if err != nil {
		return nil, err
	}
	// Persist unknown freshness before any cancellable network work.
	s.RevisionStatus = session.Unchecked
	if err = store.Save(s); err != nil {
		return nil, err
	}
	if gh != nil {
		err = Refresh(ctx, store, s, gh)
	}
	return s, err
}
