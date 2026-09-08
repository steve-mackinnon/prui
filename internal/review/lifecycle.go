package review

import (
	"context"
	"errors"
	"slices"
	"strings"

	"pr-review/internal/plan"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

func editablePlan(s *Session) plan.ValidatedPlan {
	if p := s.CurrentPlan(); p != nil {
		return *p
	}
	return plan.FileFallback(s.Inventory)
}

// MoveUnit updates ownership without changing source truth or retaining progress.
func MoveUnit(store *session.Store, s *Session, unitID, destination string) error {
	p, err := plan.MoveUnit(editablePlan(s), unitID, destination)
	if err != nil {
		return err
	}
	if err = store.ApplyPlan(s, p); err != nil {
		return err
	}
	next, err := store.Load(s.ID)
	if err != nil {
		return err
	}
	s.State = next.State
	return nil
}

// ReorderSlices updates advisory order and preserves dependency labels.
func ReorderSlices(store *session.Store, s *Session, order []string) error {
	p, err := plan.ReorderSlices(editablePlan(s), order)
	if err != nil {
		return err
	}
	if err = store.ApplyPlan(s, p); err != nil {
		return err
	}
	next, err := store.Load(s.ID)
	if err != nil {
		return err
	}
	s.State = next.State
	return nil
}

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
			if m.Identity == old.Identity && strings.EqualFold(m.BaseRepository, old.BaseRepository) && strings.EqualFold(m.HeadRepository, old.HeadRepository) && m.BaseSHA == old.BaseSHA && m.HeadSHA == old.HeadSHA {
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
