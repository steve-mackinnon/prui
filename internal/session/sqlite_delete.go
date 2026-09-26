package session

import (
	"context"
	"errors"
	"pr-review/internal/session/storage"
)

// Delete removes the session and reclaims immutable data only when no remaining
// session or reusable guide cache owns it. DerivedFrom is provenance, not ownership.
func (s *Store) Delete(id string) error {
	if !idPattern.MatchString(id) {
		return errors.New("invalid session ID")
	}
	db, err := s.db.SQL()
	if err != nil {
		return err
	}
	if s.db.ReadOnly() {
		return storage.ErrReadOnly
	}
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, "DELETE FROM sessions WHERE id=?", id); err != nil {
		return storage.Classify(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM snapshots WHERE NOT EXISTS
  (SELECT 1 FROM sessions WHERE snapshot_digest=snapshots.digest)`); err != nil {
		return storage.Classify(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM guide_bundles WHERE NOT EXISTS
  (SELECT 1 FROM sessions WHERE bundle_digest=guide_bundles.digest) AND NOT EXISTS
  (SELECT 1 FROM guide_cache WHERE bundle_digest=guide_bundles.digest)`); err != nil {
		return storage.Classify(err)
	}
	return storage.Classify(tx.Commit())
}
