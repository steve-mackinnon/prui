package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"prui/internal/session/storage"
	"prui/internal/source"
)

// Entry is a session summary that does not load source payloads.
type Entry struct {
	ID, Repository, HeadSHA           string
	Number, ReviewedCount, SliceCount int
	RevisionStatus                    RevisionStatus
	UpdatedAt                         time.Time
	Err                               error
}

func (s *Store) List() ([]Entry, error) {
	db, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT s.id,s.repository,s.pr_number,s.head_sha,
  (SELECT count(*) FROM progress p WHERE p.session_id=s.id),
  COALESCE(n.file_count,-1),s.revision_status,s.updated_at_ns
  FROM sessions s LEFT JOIN snapshots n ON n.digest=s.snapshot_digest
  ORDER BY s.updated_at_ns DESC,s.id ASC`)
	if err != nil {
		return nil, storage.Classify(err)
	}
	defer func() { _ = rows.Close() }()
	entries := []Entry{}
	for rows.Next() {
		var e Entry
		var updated int64
		if err := rows.Scan(&e.ID, &e.Repository, &e.Number, &e.HeadSHA, &e.ReviewedCount, &e.SliceCount, &e.RevisionStatus, &updated); err != nil {
			return nil, err
		}
		e.UpdatedAt = time.Unix(0, updated).UTC()
		normalized, normalizeErr := normalizeRepository(e.Repository)
		validStatus := e.RevisionStatus == Unchecked || e.RevisionStatus == Current || e.RevisionStatus == Stale || e.RevisionStatus == CheckFailed
		if !idPattern.MatchString(e.ID) || normalizeErr != nil || normalized != e.Repository || e.Number <= 0 || !shaPattern.MatchString(e.HeadSHA) || e.SliceCount < 0 || e.ReviewedCount > e.SliceCount || updated <= 0 || !validStatus {
			e.Err = fmt.Errorf("%w: invalid session summary", ErrInvalidRecord)
		}
		entries = append(entries, e)
	}
	return entries, storage.Classify(rows.Err())
}

// comparisonCandidates closes its cursor before callers load source, since each
// Store intentionally uses one database connection.
func (s *Store) comparisonCandidates(id source.Identity, metadata *source.Metadata) ([]string, error) {
	repository, err := normalizeRepository(id.Repository)
	if err != nil || id.Number <= 0 {
		return nil, errors.New("invalid pull request identity")
	}
	db, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	query := `SELECT s.id FROM sessions s WHERE s.repository=? AND s.pr_number=?`
	args := []any{repository, id.Number}
	if metadata != nil {
		if !shaPattern.MatchString(metadata.BaseSHA) || !shaPattern.MatchString(metadata.HeadSHA) {
			return nil, errors.New("invalid pinned comparison")
		}
		query = `SELECT s.id FROM sessions s JOIN snapshots n ON n.digest=s.snapshot_digest
   WHERE s.repository=? AND s.pr_number=? AND n.base_sha=? AND n.head_sha=?`
		args = append(args, metadata.BaseSHA, metadata.HeadSHA)
	}
	query += ` ORDER BY s.updated_at_ns DESC,s.id ASC`
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, storage.Classify(err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return nil, err
		}
		ids = append(ids, candidate)
	}
	return ids, storage.Classify(rows.Err())
}

func (s *Store) LatestComparison(id source.Identity) (*Record, error) {
	candidates, err := s.comparisonCandidates(id, nil)
	if err != nil {
		return nil, err
	}
	repository, _ := normalizeRepository(id.Repository)
	for _, candidate := range candidates {
		r, err := s.Load(candidate)
		if errors.Is(err, ErrInvalidRecord) || errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		current := r.Inventory.Comparison.Metadata.Identity
		normalized, err := normalizeRepository(current.Repository)
		if err == nil && normalized == repository && current.Number == id.Number {
			return r, nil
		}
	}
	return nil, nil
}

func (s *Store) HasComparisonSnapshot(id source.Identity) (bool, error) {
	r, err := s.LatestComparison(id)
	return r != nil, err
}

func (s *Store) LoadComparisonSnapshot(metadata source.Metadata) (*Snapshot, error) {
	candidates, err := s.comparisonCandidates(metadata.Identity, &metadata)
	if err != nil {
		return nil, err
	}
	normalized, _ := normalizeRepository(metadata.Identity.Repository)
	expected := metadata
	expected.Identity.Repository = normalized
	for _, candidate := range candidates {
		r, err := s.Load(candidate)
		if errors.Is(err, ErrInvalidRecord) || errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		cached := r.Inventory.Comparison.Metadata
		cached.Identity.Repository, _ = normalizeRepository(cached.Identity.Repository)
		if !source.SamePinnedRevision(cached, expected) {
			continue
		}
		snapshot := r.Snapshot
		snapshot.Guides, snapshot.DerivedFrom = nil, ""
		if snapshot.PullRequestDescription == nil || *snapshot.PullRequestDescription != metadata.Description {
			description := metadata.Description
			snapshot.PullRequestDescription = &description
			snapshot.Inventory.Comparison.Metadata = metadata
		}
		return &snapshot, nil
	}
	return nil, nil
}

func (s *Store) LoadComparisonSnapshotForIdentity(id source.Identity) (*Snapshot, error) {
	r, err := s.LatestComparison(id)
	if err != nil || r == nil {
		return nil, err
	}
	snapshot := r.Snapshot
	snapshot.Guides, snapshot.DerivedFrom = nil, ""
	return &snapshot, nil
}
