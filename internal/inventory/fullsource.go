package inventory

import (
	"bytes"
	"context"
	"crypto/sha1" // Git blob identity, not a security primitive.
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"prui/internal/source"
)

const FullSourceBytes = 4 << 20
const FullSourceLines = 100000

// FullSource is optional frozen local source for navigation, never analysis input.
// Missing blobs mean unavailable, including sessions created before this feature.
type FullSource struct {
	Blobs map[string][]byte `json:"blobs"`
}

// CaptureFullSource reads only OIDs already captured by the canonical inventory.
// It performs no fetch and never substitutes partial bytes for a complete blob.
func CaptureFullSource(ctx context.Context, o Objects, inv Inventory, l source.Limits) *FullSource {
	ctx, cancel := context.WithTimeout(ctx, min(l.Operation, 60*time.Second))
	defer cancel()
	c := &FullSource{Blobs: map[string][]byte{}}
	used, lines := 0, 0
	attempted := map[string]bool{}
	for _, f := range inv.Files {
		for _, side := range []struct{ oid, mode string }{{f.OldOID, f.OldMode}, {f.NewOID, f.NewMode}} {
			if ctx.Err() != nil {
				return c
			}
			if attempted[side.oid] || side.mode == "000000" || side.mode == "160000" {
				continue
			}
			attempted[side.oid] = true
			if used >= FullSourceBytes {
				continue
			}
			b, err := o.Blob(ctx, side.oid, min(l.BlobBytes, 1<<20, FullSourceBytes-used))
			n := bytes.Count(b, []byte{'\n'}) + 1
			if err != nil || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 || lines+n > FullSourceLines || !ValidBlob(side.oid, b) {
				continue
			}
			c.Blobs[side.oid] = bytes.Clone(b)
			used += len(b)
			lines += n
		}
	}
	return c
}

func ValidBlob(oid string, b []byte) bool {
	//nolint:gosec // SHA-1 is required by the repository's Git object format.
	h := sha1.New()
	_, _ = fmt.Fprintf(h, "blob %d%c", len(b), 0)
	_, _ = h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil)) == oid
}

func (c *FullSource) Valid(files []FileChange) bool {
	if c == nil {
		return true
	}
	allowed := map[string]bool{}
	for _, f := range files {
		if f.OldMode != "000000" && f.OldMode != "160000" {
			allowed[f.OldOID] = true
		}
		if f.NewMode != "000000" && f.NewMode != "160000" {
			allowed[f.NewOID] = true
		}
	}
	used, lines := 0, 0
	for oid, b := range c.Blobs {
		used += len(b)
		lines += bytes.Count(b, []byte{'\n'}) + 1
		if !allowed[oid] || len(b) > 1<<20 || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 || !ValidBlob(oid, b) {
			return false
		}
	}
	return used <= FullSourceBytes && lines <= FullSourceLines
}

func (inv Inventory) SourceLines(file int, old bool) ([]string, bool) {
	if file < 0 || file >= len(inv.Files) || inv.FullSource == nil {
		return nil, false
	}
	f := inv.Files[file]
	oid, mode := f.NewOID, f.NewMode
	if old {
		oid, mode = f.OldOID, f.OldMode
	}
	if mode == "000000" {
		return nil, true
	}
	b, ok := inv.FullSource.Blobs[oid]
	if !ok {
		return nil, false
	}
	if len(b) == 0 {
		return nil, true
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"), true
}
