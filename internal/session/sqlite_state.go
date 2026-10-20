package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"prui/internal/session/storage"
)

var ErrStateConflict = errors.New("session state changed; reload before updating")
var ErrInvalidStateUpdate = errors.New("invalid session state update")
var ErrCommitUncertain = errors.New("session state commit outcome uncertain")

// StateUpdate contains the complete mutable part of a review session.
type StateUpdate struct {
	ReviewedSliceIDs []string
	RevisionStatus   RevisionStatus
}

// UpdateState changes only the small session and progress rows. It never
// fetches or serializes source or guide payloads.
func (s *Store) UpdateState(ctx context.Context, id string, expectedGeneration uint64, snapshotReference string, update StateUpdate) (State, error) {
	ctx, cancel := storage.OperationContext(ctx)
	defer cancel()
	if s.db.ReadOnly() {
		return State{}, storage.ErrReadOnly
	}
	if !idPattern.MatchString(id) || len(snapshotReference) != 64 || expectedGeneration == 0 || expectedGeneration >= math.MaxInt64 {
		return State{}, ErrInvalidStateUpdate
	}
	switch update.RevisionStatus {
	case Unchecked, Current, Stale, CheckFailed:
	default:
		return State{}, ErrInvalidStateUpdate
	}
	if len(update.ReviewedSliceIDs) > 10000 {
		return State{}, ErrInvalidStateUpdate
	}
	seen := make(map[string]struct{}, len(update.ReviewedSliceIDs))
	for _, fileID := range update.ReviewedSliceIDs {
		if fileID == "" {
			return State{}, ErrInvalidStateUpdate
		}
		if _, exists := seen[fileID]; exists {
			return State{}, ErrInvalidStateUpdate
		}
		seen[fileID] = struct{}{}
	}
	sqlDB, err := s.db.SQL()
	if err != nil {
		return State{}, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return State{}, storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	updatedAt := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `UPDATE sessions SET revision_status=?,updated_at_ns=?,generation=generation+1
		WHERE id=? AND generation=? AND snapshot_reference=? AND generation<9223372036854775807`,
		update.RevisionStatus, updatedAt.UnixNano(), id, int64(expectedGeneration), snapshotReference)
	if err != nil {
		return State{}, storage.Classify(err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return State{}, storage.Classify(err)
	}
	if affected != 1 {
		return State{}, ErrStateConflict
	}
	unchanged, err := unchangedProgress(ctx, tx, id, update.ReviewedSliceIDs)
	if err != nil {
		return State{}, err
	}
	if !unchanged {
		for _, fileID := range update.ReviewedSliceIDs {
			var exists int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions s JOIN snapshot_files f ON f.snapshot_digest=s.snapshot_digest
			WHERE s.id=? AND f.file_id=?`, id, fileID).Scan(&exists)
			if errors.Is(err, sql.ErrNoRows) {
				return State{}, fmt.Errorf("%w: unknown file ID", ErrInvalidStateUpdate)
			}
			if err != nil {
				return State{}, storage.Classify(err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM progress WHERE session_id=?`, id); err != nil {
			return State{}, storage.Classify(err)
		}
		for ordinal, fileID := range update.ReviewedSliceIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO progress(session_id,file_id,ordinal) VALUES(?,?,?)`, id, fileID, ordinal); err != nil {
				return State{}, storage.Classify(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return State{}, fmt.Errorf("%w: %w", ErrCommitUncertain, storage.Classify(err))
	}
	return State{SchemaVersion: SchemaVersion, ID: id, SnapshotReference: snapshotReference,
		ReviewedSliceIDs: append([]string{}, update.ReviewedSliceIDs...), RevisionStatus: update.RevisionStatus,
		UpdatedAt: updatedAt, Generation: expectedGeneration + 1}, nil
}

// The CAS update above holds the write transaction while this query checks
// exact order, contiguous ordinals and every mark's current snapshot membership.
// Only an equivalent valid row set can skip the otherwise unchanged rewrite.
func unchangedProgress(ctx context.Context, tx *sql.Tx, id string, wanted []string) (bool, error) {
	if len(wanted) == 0 {
		// A single DELETE already handles this case without per-mark work.
		return false, nil
	}
	const maxProgressIDBytes = 256
	for _, fileID := range wanted {
		if len(fileID) > maxProgressIDBytes {
			return false, nil // Preserve the original path for unusual legacy IDs.
		}
	}
	// Bound Go-side materialization even if a private database contains a
	// malformed oversized progress ID. Such rows must use the repair path.
	rows, err := tx.QueryContext(ctx, `SELECT CASE WHEN length(CAST(p.file_id AS BLOB))<=? THEN p.file_id ELSE '' END,
		p.ordinal,f.file_id IS NOT NULL
		FROM progress p JOIN sessions s ON s.id=p.session_id
		LEFT JOIN snapshot_files f ON f.snapshot_digest=s.snapshot_digest AND f.file_id=p.file_id
		WHERE p.session_id=? ORDER BY p.ordinal`, maxProgressIDBytes, id)
	if err != nil {
		return false, storage.Classify(err)
	}
	defer func() { _ = rows.Close() }()
	position := 0
	for rows.Next() {
		var fileID string
		var ordinal int64
		var valid bool
		if err := rows.Scan(&fileID, &ordinal, &valid); err != nil {
			return false, storage.Classify(err)
		}
		if position >= len(wanted) || fileID != wanted[position] || ordinal != int64(position) || !valid {
			return false, storage.Classify(rows.Close())
		}
		position++
	}
	if err := rows.Err(); err != nil {
		return false, storage.Classify(err)
	}
	return position == len(wanted), nil
}
