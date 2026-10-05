package session

import (
	"context"
	"errors"
	"prui/internal/session/storage"
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
	if err = s.ensureDrafts(ctx); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	// Delete comparison-owned drafts only when this is its last saved session.
	if _, err = tx.ExecContext(ctx, `DELETE FROM review_drafts WHERE EXISTS (
 SELECT 1 FROM sessions s JOIN snapshots p ON p.digest=s.snapshot_digest
 WHERE s.id=? AND p.repository=review_drafts.repository AND p.pr_number=review_drafts.pr_number
 AND p.base_sha=review_drafts.base_sha AND p.head_sha=review_drafts.head_sha
 AND lower(p.base_repository)=review_drafts.base_repository AND lower(p.head_repository)=review_drafts.head_repository
 AND NOT EXISTS (SELECT 1 FROM sessions other JOIN snapshots op ON op.digest=other.snapshot_digest WHERE other.id<>s.id AND op.repository=p.repository AND op.pr_number=p.pr_number AND op.base_sha=p.base_sha AND op.head_sha=p.head_sha AND lower(op.base_repository)=lower(p.base_repository) AND lower(op.head_repository)=lower(p.head_repository)))`, id); err != nil {
		return storage.Classify(err)
	}
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
