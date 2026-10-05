package inventory

import (
	"context"
	"fmt"
	"prui/internal/source"
	"testing"
	"time"
)

type navigationObjects struct {
	data  map[string][]byte
	calls int
}

func (o *navigationObjects) Git(context.Context, int, ...string) ([]byte, error) {
	return nil, fmt.Errorf("unexpected Git command")
}
func (o *navigationObjects) Blob(_ context.Context, id string, limit int) ([]byte, error) {
	o.calls++
	b, ok := o.data[id]
	if !ok {
		return nil, fmt.Errorf("missing")
	}
	if len(b) > limit {
		return nil, source.ErrLimit
	}
	return b, nil
}
func TestFullSourcePinsBoundsAndMissing(t *testing.T) {
	const oid = "ce013625030ba8dba906f756967f9e9ca394464a"
	o := &navigationObjects{data: map[string][]byte{oid: []byte("hello\n")}}
	inv := Inventory{Files: []FileChange{{OldOID: oid, NewOID: oid, OldMode: "100644", NewMode: "100644"}}}
	inv.FullSource = CaptureFullSource(context.Background(), o, inv, source.Limits{BlobBytes: 1024, Operation: time.Minute})
	if o.calls != 1 || !inv.FullSource.Valid(inv.Files) {
		t.Fatal("deduplicated pinned blob not retained")
	}
	lines, ok := inv.SourceLines(0, true)
	if !ok || len(lines) != 1 || lines[0] != "hello" {
		t.Fatal(lines, ok)
	}
	inv.FullSource.Blobs[oid][0] = 'x'
	if inv.FullSource.Valid(inv.Files) {
		t.Fatal("tampered blob accepted")
	}
	o.calls = 0
	inv.FullSource = CaptureFullSource(context.Background(), o, inv, source.Limits{BlobBytes: 2, Operation: time.Minute})
	if _, ok = inv.SourceLines(0, false); ok {
		t.Fatal("limited content claimed available")
	}
	inv.FullSource = nil
	if _, ok = inv.SourceLines(0, true); ok {
		t.Fatal("legacy session invented source")
	}
}

func TestFullSourceAbsentSideNeedsNoCache(t *testing.T) {
	inv := Inventory{Files: []FileChange{{OldMode: "000000", NewMode: "100644"}, {OldMode: "100644", NewMode: "000000"}}}
	for _, side := range []struct {
		file int
		old  bool
	}{{0, true}, {1, false}} {
		lines, ok := inv.SourceLines(side.file, side.old)
		if !ok || len(lines) != 0 {
			t.Fatal("known absent side claimed unavailable")
		}
	}
	if _, ok := inv.SourceLines(0, false); ok {
		t.Fatal("missing existing source claimed available")
	}
}
