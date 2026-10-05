package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"prui/internal/incremental"
	"slices"
	"time"

	"prui/internal/session/storage"
)

// Store persists review sessions and reusable source data in SQLite.
type Store struct{ db *storage.DB }

func (s *Store) Path() string { return s.db.Path() }
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Create(snapshot Snapshot) (*Record, error) {
	return s.createWithProgress(snapshot, nil, false)
}

// CreateWithProgress atomically stores an immutable snapshot and proven initial
// local progress. It never changes the predecessor or its private drafts.
func (s *Store) CreateWithProgress(snapshot Snapshot, reviewed []string) (*Record, error) {
	if b := snapshot.Incremental; b != nil {
		old, err := s.Load(b.PreviousID)
		if err != nil {
			return nil, err
		}
		if old.SnapshotReference != b.PreviousReference || old.Generation != b.PreviousGeneration {
			return nil, ErrStateConflict
		}
		if !slices.Equal(reviewed, incremental.Carry(old.Inventory, snapshot.Inventory, old.ReviewedSliceIDs, b)) {
			return nil, ErrInvalidStateUpdate
		}
	} else if len(reviewed) != 0 {
		return nil, ErrInvalidStateUpdate
	}
	return s.createWithProgress(snapshot, reviewed, true)
}

func (s *Store) createWithProgress(snapshot Snapshot, reviewed []string, checkPrevious bool) (*Record, error) {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	if s.db.ReadOnly() {
		return nil, storage.ErrReadOnly
	}
	if snapshot.DerivedFrom != "" && !idPattern.MatchString(snapshot.DerivedFrom) {
		return nil, errors.New("invalid derived session source")
	}
	sourceDigest, sourceBytes, err := encodeSource(snapshot)
	if err != nil {
		return nil, err
	}
	var bundleDigest string
	var bundleBytes []byte
	if snapshot.Guides != nil {
		bundleDigest, bundleBytes, err = encodeGuideBundle(*snapshot.Guides)
		if err != nil {
			return nil, err
		}
	}
	checkout := append([]byte{}, snapshot.Checkout...)
	reference, err := logicalSnapshotReference(sourceDigest, bundleDigest, checkout, snapshot.DerivedFrom)
	if err != nil {
		return nil, err
	}
	var idBytes [16]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return nil, err
	}
	comparison := snapshot.Inventory.Comparison.Metadata
	repository, err := normalizeRepository(comparison.Identity.Repository)
	if err != nil {
		return nil, err
	}
	r := &Record{Snapshot: snapshot, State: State{
		SchemaVersion: SchemaVersion, ID: fmt.Sprintf("%x", idBytes),
		SnapshotReference: reference, ReviewedSliceIDs: append([]string{}, reviewed...),
		RevisionStatus: Unchecked, UpdatedAt: time.Now().UTC(), Generation: 1,
	}}
	if err := validate(r); err != nil {
		return nil, err
	}
	if snapshot.DerivedFrom != "" {
		parent, err := s.Load(snapshot.DerivedFrom)
		if err != nil {
			return nil, fmt.Errorf("derived session source unavailable: %w", err)
		}
		parentDigest, _, err := encodeSource(parent.Snapshot)
		if err != nil || parentDigest != sourceDigest {
			return nil, errors.New("derived session source evidence changed")
		}
	}
	sqlDB, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	if checkPrevious && snapshot.Incremental != nil {
		b := snapshot.Incremental
		var generation uint64
		var reference string
		if err := tx.QueryRowContext(ctx, `SELECT generation,snapshot_reference FROM sessions WHERE id=?`, b.PreviousID).Scan(&generation, &reference); err != nil {
			return nil, storage.Classify(err)
		}
		if generation != b.PreviousGeneration || reference != b.PreviousReference {
			return nil, ErrStateConflict
		}
	}
	if snapshot.DerivedFrom != "" {
		var parentDigest, parentReference, parentDerived string
		var parentBundle sql.NullString
		var parentCheckout []byte
		var parentGeneration int64
		err := tx.QueryRowContext(ctx, `SELECT snapshot_digest,bundle_digest,checkout,derived_from,snapshot_reference,generation
			FROM sessions WHERE id = ?`, snapshot.DerivedFrom).Scan(&parentDigest, &parentBundle, &parentCheckout,
			&parentDerived, &parentReference, &parentGeneration)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("derived session source unavailable")
		}
		if err != nil {
			return nil, storage.Classify(err)
		}
		if parentDigest != sourceDigest {
			return nil, errors.New("derived session source evidence changed")
		}
		validatedReference, err := logicalSnapshotReference(parentDigest, parentBundle.String, parentCheckout, parentDerived)
		if err != nil || validatedReference != parentReference || parentGeneration < 1 {
			return nil, fmt.Errorf("%w: derived parent reference", ErrInvalidRecord)
		}
	}
	inserted, err := tx.ExecContext(ctx, `INSERT INTO snapshots
		(digest,payload_version,payload,repository,pr_number,base_sha,head_sha,base_repository,head_repository,inventory_id,file_count)
		VALUES (?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(digest) DO NOTHING`,
		sourceDigest, sqlitePayloadVersion, sourceBytes, repository, comparison.Identity.Number,
		comparison.BaseSHA, comparison.HeadSHA, comparison.BaseRepository, comparison.HeadRepository,
		snapshot.Inventory.Comparison.InventoryID, len(snapshot.Inventory.Files))
	if err != nil {
		return nil, storage.Classify(err)
	}
	insertedRows, err := inserted.RowsAffected()
	if err != nil {
		return nil, storage.Classify(err)
	}
	if insertedRows != 0 && insertedRows != 1 {
		return nil, fmt.Errorf("%w: invalid source insert result", ErrInvalidRecord)
	}
	// Recheck a reused row: equality of a SHA-256 digest alone does not excuse
	// inconsistent indexed fields or tampering of its stored payload.
	var storedSize int64
	if err := tx.QueryRowContext(ctx, `SELECT length(payload) FROM snapshots WHERE digest=?`, sourceDigest).Scan(&storedSize); err != nil {
		return nil, storage.Classify(err)
	}
	if storedSize < 1 || storedSize > sqliteMaxPayloadBytes {
		return nil, fmt.Errorf("%w: reused source payload size", ErrInvalidRecord)
	}
	var stored []byte
	var storedRepo, storedBase, storedHead, storedBaseRepo, storedHeadRepo, storedInventory string
	var storedPR, storedFiles, storedVersion int
	err = tx.QueryRowContext(ctx, `SELECT payload,repository,pr_number,base_sha,head_sha,base_repository,head_repository,inventory_id,file_count,payload_version
		FROM snapshots WHERE digest = ?`, sourceDigest).Scan(&stored, &storedRepo, &storedPR, &storedBase, &storedHead,
		&storedBaseRepo, &storedHeadRepo, &storedInventory, &storedFiles, &storedVersion)
	if err != nil {
		return nil, storage.Classify(err)
	}
	if !bytes.Equal(stored, sourceBytes) || storedRepo != repository || storedPR != comparison.Identity.Number ||
		storedBase != comparison.BaseSHA || storedHead != comparison.HeadSHA ||
		storedBaseRepo != comparison.BaseRepository || storedHeadRepo != comparison.HeadRepository ||
		storedInventory != snapshot.Inventory.Comparison.InventoryID || storedFiles != len(snapshot.Inventory.Files) || storedVersion != sqlitePayloadVersion {
		return nil, fmt.Errorf("%w: source row identity mismatch", ErrInvalidRecord)
	}
	for i, file := range snapshot.Inventory.Files {
		if insertedRows == 1 {
			if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_files(snapshot_digest,file_id,ordinal)
				VALUES(?,?,?)`, sourceDigest, file.ID, i); err != nil {
				return nil, storage.Classify(err)
			}
		}
		var ordinal int
		if err := tx.QueryRowContext(ctx, `SELECT ordinal FROM snapshot_files WHERE snapshot_digest=? AND file_id=?`, sourceDigest, file.ID).Scan(&ordinal); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: missing source file membership", ErrInvalidRecord)
			}
			return nil, storage.Classify(err)
		}
		if ordinal != i {
			return nil, fmt.Errorf("%w: source file membership mismatch", ErrInvalidRecord)
		}
	}
	var membershipCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM snapshot_files WHERE snapshot_digest=?`, sourceDigest).Scan(&membershipCount); err != nil {
		return nil, storage.Classify(err)
	}
	if membershipCount != len(snapshot.Inventory.Files) {
		return nil, fmt.Errorf("%w: source file membership count mismatch", ErrInvalidRecord)
	}
	if bundleDigest != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO guide_bundles(digest,payload_version,payload) VALUES(?,?,?) ON CONFLICT(digest) DO NOTHING`,
			bundleDigest, sqlitePayloadVersion, bundleBytes); err != nil {
			return nil, storage.Classify(err)
		}
		var bundleSize int64
		if err := tx.QueryRowContext(ctx, `SELECT length(payload) FROM guide_bundles WHERE digest=?`, bundleDigest).Scan(&bundleSize); err != nil {
			return nil, storage.Classify(err)
		}
		if bundleSize < 1 || bundleSize > sqliteMaxPayloadBytes {
			return nil, fmt.Errorf("%w: reused guide bundle payload size", ErrInvalidRecord)
		}
		var storedBundle []byte
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT payload,payload_version FROM guide_bundles WHERE digest=?`, bundleDigest).Scan(&storedBundle, &version); err != nil {
			return nil, storage.Classify(err)
		}
		if version != sqlitePayloadVersion || !bytes.Equal(storedBundle, bundleBytes) {
			return nil, fmt.Errorf("%w: guide bundle row mismatch", ErrInvalidRecord)
		}
	}
	var nullableBundle any
	if bundleDigest != "" {
		nullableBundle = bundleDigest
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions
		(id,snapshot_digest,bundle_digest,checkout,derived_from,snapshot_reference,repository,pr_number,head_sha,revision_status,updated_at_ns,generation)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, r.ID, sourceDigest, nullableBundle, checkout, snapshot.DerivedFrom,
		reference, repository, comparison.Identity.Number, comparison.HeadSHA, r.RevisionStatus, r.UpdatedAt.UnixNano(), 1)
	if err != nil {
		return nil, storage.Classify(err)
	}
	for ordinal, fileID := range r.ReviewedSliceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO progress(session_id,file_id,ordinal) VALUES(?,?,?)`, r.ID, fileID, ordinal); err != nil {
			return nil, storage.Classify(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, storage.Classify(err)
	}
	return r, nil
}

func (s *Store) Load(id string) (*Record, error) {
	if !idPattern.MatchString(id) {
		return nil, errors.New("invalid session ID")
	}
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	sqlDB, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	tx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	var sourceDigest, reference, repository, headSHA, status, derived string
	var bundleDigest sql.NullString
	var checkout []byte
	var number, updatedNS, generation int64
	err = tx.QueryRowContext(ctx, `SELECT snapshot_digest,bundle_digest,checkout,derived_from,snapshot_reference,
		repository,pr_number,head_sha,revision_status,updated_at_ns,generation FROM sessions WHERE id=?`, id).
		Scan(&sourceDigest, &bundleDigest, &checkout, &derived, &reference, &repository, &number, &headSHA, &status, &updatedNS, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, storage.Classify(err)
	}
	var sourceSize, sourceVersion int64
	var indexedRepo, baseSHA, indexedHead, baseRepo, headRepo, inventoryID string
	var indexedPR, fileCount int64
	err = tx.QueryRowContext(ctx, `SELECT length(payload),payload_version,repository,pr_number,base_sha,head_sha,
		base_repository,head_repository,inventory_id,file_count FROM snapshots WHERE digest=?`, sourceDigest).
		Scan(&sourceSize, &sourceVersion, &indexedRepo, &indexedPR, &baseSHA, &indexedHead, &baseRepo, &headRepo, &inventoryID, &fileCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: missing source row", ErrInvalidRecord)
	}
	if err != nil {
		return nil, storage.Classify(err)
	}
	if sourceSize < 1 || sourceSize > sqliteMaxPayloadBytes || sourceVersion != sqlitePayloadVersion {
		return nil, fmt.Errorf("%w: source size or version", ErrInvalidRecord)
	}
	var sourceBytes []byte
	if err := tx.QueryRowContext(ctx, `SELECT payload FROM snapshots WHERE digest=?`, sourceDigest).Scan(&sourceBytes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: missing source row", ErrInvalidRecord)
		}
		return nil, storage.Classify(err)
	}
	snapshot, err := decodeSource(sourceDigest, sourceBytes)
	if err != nil {
		return nil, err
	}
	comparison := snapshot.Inventory.Comparison.Metadata
	normalizedRepo, err := normalizeRepository(comparison.Identity.Repository)
	if err != nil || normalizedRepo != repository || normalizedRepo != indexedRepo || int64(comparison.Identity.Number) != number || number != indexedPR ||
		comparison.BaseSHA != baseSHA || comparison.HeadSHA != headSHA || comparison.HeadSHA != indexedHead ||
		comparison.BaseRepository != baseRepo || comparison.HeadRepository != headRepo ||
		snapshot.Inventory.Comparison.InventoryID != inventoryID || int64(len(snapshot.Inventory.Files)) != fileCount {
		return nil, fmt.Errorf("%w: indexed source identity mismatch", ErrInvalidRecord)
	}
	if bundleDigest.Valid {
		var bundleSize, bundleVersion int64
		if err := tx.QueryRowContext(ctx, `SELECT length(payload),payload_version FROM guide_bundles WHERE digest=?`, bundleDigest.String).Scan(&bundleSize, &bundleVersion); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: missing guide bundle row", ErrInvalidRecord)
			}
			return nil, storage.Classify(err)
		}
		if bundleSize < 1 || bundleSize > sqliteMaxPayloadBytes || bundleVersion != sqlitePayloadVersion {
			return nil, fmt.Errorf("%w: guide bundle size or version", ErrInvalidRecord)
		}
		var bundleBytes []byte
		if err := tx.QueryRowContext(ctx, `SELECT payload FROM guide_bundles WHERE digest=?`, bundleDigest.String).Scan(&bundleBytes); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: missing guide bundle row", ErrInvalidRecord)
			}
			return nil, storage.Classify(err)
		}
		snapshot.Guides, err = decodeGuideBundle(bundleDigest.String, bundleBytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
		}
	}
	snapshot.Checkout = checkout
	snapshot.DerivedFrom = derived
	expectedRef, err := logicalSnapshotReference(sourceDigest, bundleDigest.String, checkout, derived)
	if err != nil || expectedRef != reference || generation < 1 || updatedNS < 1 {
		return nil, fmt.Errorf("%w: logical reference or state mismatch", ErrInvalidRecord)
	}
	r := &Record{Snapshot: snapshot, State: State{SchemaVersion: SchemaVersion, ID: id,
		SnapshotReference: reference, RevisionStatus: RevisionStatus(status),
		ReviewedSliceIDs: []string{}, UpdatedAt: time.Unix(0, updatedNS).UTC(), Generation: uint64(generation)}}
	rows, err := tx.QueryContext(ctx, `SELECT file_id FROM progress WHERE session_id=? ORDER BY ordinal`, id)
	if err != nil {
		return nil, storage.Classify(err)
	}
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err != nil {
			_ = rows.Close()
			return nil, storage.Classify(err)
		}
		r.ReviewedSliceIDs = append(r.ReviewedSliceIDs, fileID)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, storage.Classify(err)
	}
	if err := validate(r); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRecord, err)
	}
	fileRows, err := tx.QueryContext(ctx, `SELECT file_id,ordinal FROM snapshot_files WHERE snapshot_digest=? ORDER BY ordinal`, sourceDigest)
	if err != nil {
		return nil, storage.Classify(err)
	}
	fileIndex := 0
	for fileRows.Next() {
		var fileID string
		var ordinal int
		if err := fileRows.Scan(&fileID, &ordinal); err != nil {
			_ = fileRows.Close()
			return nil, storage.Classify(err)
		}
		if fileIndex >= len(r.Inventory.Files) || ordinal != fileIndex || fileID != r.Inventory.Files[fileIndex].ID {
			_ = fileRows.Close()
			return nil, fmt.Errorf("%w: source file membership mismatch", ErrInvalidRecord)
		}
		fileIndex++
	}
	err = fileRows.Err()
	_ = fileRows.Close()
	if err != nil {
		return nil, storage.Classify(err)
	}
	if fileIndex != len(r.Inventory.Files) {
		return nil, fmt.Errorf("%w: incomplete source file membership", ErrInvalidRecord)
	}
	if err := tx.Commit(); err != nil {
		return nil, storage.Classify(err)
	}
	return r, nil
}
