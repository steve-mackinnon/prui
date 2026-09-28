package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	reviewcontext "prui/internal/context"
	"prui/internal/inventory"
)

const sqlitePayloadVersion = 1
const sqliteMaxPayloadBytes = 128 << 20

// ErrInvalidRecord identifies an individual session or source row whose stored
// values fail validation. Lookup callers may skip this row without hiding SQL
// engine errors.
var ErrInvalidRecord = errors.New("invalid stored session record")

type sqliteSourcePayload struct {
	Version                int                         `json:"version"`
	Inventory              inventory.Inventory         `json:"inventory"`
	Slices                 []Slice                     `json:"slices"`
	UnitFiles              []int                       `json:"unit_files"`
	Context                reviewcontext.ContextBundle `json:"context"`
	PullRequestDescription *string                     `json:"pull_request_description,omitempty"`
}

func encodeSource(snapshot Snapshot) (string, []byte, error) {
	payload := sqliteSourcePayload{
		Version: sqlitePayloadVersion, Inventory: snapshot.Inventory, Slices: snapshot.Slices,
		UnitFiles: snapshot.UnitFiles, Context: snapshot.Context,
		PullRequestDescription: snapshot.PullRequestDescription,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	if len(b) == 0 || len(b) > sqliteMaxPayloadBytes {
		return "", nil, errors.New("session source exceeds storage limit")
	}
	return digest(b), b, nil
}

func decodeSource(expectedDigest string, b []byte) (Snapshot, error) {
	if len(b) == 0 || len(b) > sqliteMaxPayloadBytes || digest(b) != expectedDigest {
		return Snapshot{}, fmt.Errorf("%w: source checksum or size mismatch", ErrInvalidRecord)
	}
	var payload sqliteSourcePayload
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return Snapshot{}, fmt.Errorf("%w: source decode: %v", ErrInvalidRecord, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF || payload.Version != sqlitePayloadVersion {
		return Snapshot{}, fmt.Errorf("%w: unsupported or trailing source payload", ErrInvalidRecord)
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(canonical, b) {
		return Snapshot{}, fmt.Errorf("%w: noncanonical source payload", ErrInvalidRecord)
	}
	return Snapshot{Inventory: payload.Inventory, Slices: payload.Slices,
		UnitFiles: payload.UnitFiles, Context: payload.Context,
		PullRequestDescription: payload.PullRequestDescription}, nil
}

func logicalSnapshotReference(sourceDigest, bundleDigest string, checkout []byte, derivedFrom string) (string, error) {
	// The length and order of these fields are fixed by the payload version.
	// The source digest itself commits to all frozen source data.
	// SQLite returns a zero-length BLOB as nil, so normalize that one value.
	if len(checkout) == 0 {
		checkout = nil
	}
	b, err := json.Marshal(struct {
		Version      int    `json:"version"`
		SourceDigest string `json:"source_digest"`
		BundleDigest string `json:"bundle_digest"`
		Checkout     []byte `json:"checkout"`
		DerivedFrom  string `json:"derived_from"`
	}{sqlitePayloadVersion, sourceDigest, bundleDigest, checkout, derivedFrom})
	if err != nil {
		return "", err
	}
	return digest(b), nil
}
