package session

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/session/storage"
)

const sqliteGuidePayloadVersion = 1
const sqliteMaxGuidePayloadBytes = 128 << 20

type sqliteGuidePayload struct {
	Version int          `json:"version"`
	Bundle  guide.Bundle `json:"bundle"`
}

// The version participates in the digest so a later format cannot reuse an
// earlier bundle's address, even when its visible guide fields happen to match.
func encodeGuideBundle(bundle guide.Bundle) (string, []byte, error) {
	payload, err := json.Marshal(sqliteGuidePayload{Version: sqliteGuidePayloadVersion, Bundle: bundle})
	if err != nil {
		return "", nil, err
	}
	if len(payload) == 0 || len(payload) > sqliteMaxGuidePayloadBytes {
		return "", nil, errors.New("guide bundle exceeds storage limit")
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), payload, nil
}

func decodeGuideBundle(digest string, payload []byte) (*guide.Bundle, error) {
	if len(payload) == 0 || len(payload) > sqliteMaxGuidePayloadBytes {
		return nil, errors.New("invalid guide bundle size")
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("guide bundle checksum mismatch")
	}
	var decoded sqliteGuidePayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, err
	}
	if decoded.Version != sqliteGuidePayloadVersion {
		return nil, errors.New("unsupported guide bundle payload version")
	}
	// JSON struct marshaling is deterministic; reject alternate representations
	// so the address always identifies one canonical payload.
	canonical, err := json.Marshal(decoded)
	if err != nil {
		return nil, err
	}
	if string(canonical) != string(payload) {
		return nil, errors.New("noncanonical guide bundle payload")
	}
	return &decoded.Bundle, nil
}

func (s *Store) SaveGeneratedGuide(key GuideCacheKey, bundle guide.Bundle, inv inventory.Inventory) error {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	conn, err := s.db.SQL()
	if err != nil {
		return err
	}
	if s.db.ReadOnly() {
		return storage.ErrReadOnly
	}
	key, err = normalizeGuideCacheKey(key)
	if err != nil {
		return err
	}
	if !guideCacheMatchesInventory(key, inv) {
		return errors.New("guide cache key does not match inventory comparison")
	}
	if bundle.Status != guide.Generated || bundle.PromptVersion != guide.PromptVersion || guide.Validate(bundle, inv) != nil {
		return errors.New("only valid current-prompt generated guides may be cached")
	}
	digest, payload, err := encodeGuideBundle(bundle)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin guide cache write: %w", storage.Classify(err))
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO guide_bundles(digest,payload_version,payload) VALUES(?,?,?) ON CONFLICT(digest) DO NOTHING", digest, sqliteGuidePayloadVersion, payload); err != nil {
		return fmt.Errorf("store guide bundle: %w", storage.Classify(err))
	}
	var storedVersion int
	var storedPayload []byte
	if err := tx.QueryRowContext(ctx, "SELECT payload_version,CASE WHEN length(payload)<=? THEN payload END FROM guide_bundles WHERE digest=?", sqliteMaxGuidePayloadBytes, digest).Scan(&storedVersion, &storedPayload); err != nil {
		return fmt.Errorf("verify guide bundle: %w", storage.Classify(err))
	}
	if storedVersion != sqliteGuidePayloadVersion || !bytes.Equal(storedPayload, payload) {
		return errors.New("stored guide bundle content mismatch; original retained")
	}
	var oldDigest string
	err = tx.QueryRowContext(ctx, "SELECT bundle_digest FROM guide_cache WHERE repository=? AND pr_number=? AND base_sha=? AND head_sha=? AND inventory_id=? AND prompt_version=?", key.Repository, key.Number, key.BaseSHA, key.HeadSHA, inv.Comparison.InventoryID, guide.PromptVersion).Scan(&oldDigest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read guide cache entry: %w", storage.Classify(err))
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO guide_cache(repository,pr_number,base_sha,head_sha,inventory_id,prompt_version,bundle_digest) VALUES(?,?,?,?,?,?,?) ON CONFLICT(repository,pr_number,base_sha,head_sha,inventory_id,prompt_version) DO UPDATE SET bundle_digest=excluded.bundle_digest", key.Repository, key.Number, key.BaseSHA, key.HeadSHA, inv.Comparison.InventoryID, guide.PromptVersion, digest); err != nil {
		return fmt.Errorf("store guide cache entry: %w", storage.Classify(err))
	}
	if oldDigest != "" && oldDigest != digest {
		if _, err := tx.ExecContext(ctx, "DELETE FROM guide_bundles WHERE digest=? AND NOT EXISTS(SELECT 1 FROM sessions WHERE bundle_digest=?) AND NOT EXISTS(SELECT 1 FROM guide_cache WHERE bundle_digest=?)", oldDigest, oldDigest, oldDigest); err != nil {
			return fmt.Errorf("reclaim old guide bundle: %w", storage.Classify(err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit guide cache: %w", storage.Classify(err))
	}
	return nil
}

func (s *Store) LoadGeneratedGuide(key GuideCacheKey, inv inventory.Inventory) (*guide.Bundle, error) {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	conn, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	key, err = normalizeGuideCacheKey(key)
	if err != nil {
		return nil, err
	}
	if !guideCacheMatchesInventory(key, inv) {
		return nil, nil
	}
	var digest string
	var version int
	var payload []byte
	err = conn.QueryRowContext(ctx, "SELECT b.digest,b.payload_version,CASE WHEN length(b.payload)<=? THEN b.payload END FROM guide_cache AS c JOIN guide_bundles AS b ON b.digest=c.bundle_digest WHERE c.repository=? AND c.pr_number=? AND c.base_sha=? AND c.head_sha=? AND c.inventory_id=? AND c.prompt_version=?", sqliteMaxGuidePayloadBytes, key.Repository, key.Number, key.BaseSHA, key.HeadSHA, inv.Comparison.InventoryID, guide.PromptVersion).Scan(&digest, &version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load guide cache: %w", storage.Classify(err))
	}
	if version != sqliteGuidePayloadVersion {
		return nil, nil
	}
	bundle, err := decodeGuideBundle(digest, payload)
	if err != nil || bundle.Status != guide.Generated || bundle.PromptVersion != guide.PromptVersion || guide.Validate(*bundle, inv) != nil {
		return nil, nil
	}
	return bundle, nil
}
