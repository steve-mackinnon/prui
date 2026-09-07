package context

import (
	"context"
	"testing"
	"time"

	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"pr-review/internal/source"
)

type fakeObjects struct {
	tree  []byte
	blobs map[string][]byte
}

func (f fakeObjects) Git(context.Context, int, ...string) ([]byte, error) { return f.tree, nil }
func (f fakeObjects) Blob(_ context.Context, oid string, limit int) ([]byte, error) {
	b := f.blobs[oid]
	if len(b) > limit {
		return nil, source.ErrLimit
	}
	return b, nil
}

func TestRetrieveReportsPinnedEvidenceAndOmissions(t *testing.T) {
	base, head := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	tree := []byte("100644 blob " + "cccccccccccccccccccccccccccccccccccccccc" + "\tREADME.md\x00" + "100644 blob " + "dddddddddddddddddddddddddddddddddddddddd" + "\t.env\x00" + "100644 blob " + "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" + "\tinternal/handler.go\x00")
	f := fakeObjects{tree: tree, blobs: map[string][]byte{
		"cccccccccccccccccccccccccccccccccccccccc": []byte("# Architecture\n"),
		"dddddddddddddddddddddddddddddddddddddddd": []byte("TOKEN=secret\n"),
		"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee": []byte("package handler\n"),
	}}
	p := source.PinnedComparison{InventoryID: "comparison", MergeBaseSHA: base, Metadata: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, HeadSHA: head}}
	inv := inventory.Inventory{Files: []inventory.FileChange{{OldPath: []byte("internal/handler.go"), NewPath: []byte("internal/handler.go")}}}
	c := Retrieve(context.Background(), f, p, inv, privacy.Policy{}, Limits{Files: 2, ExcerptBytes: 32, Bytes: 64, Duration: time.Second})
	if len(c.Evidence) == 0 || string(c.Evidence[0].Path) != "README.md" {
		t.Fatalf("unexpected evidence: %+v", c.Evidence)
	}
	if len(c.OmittedPaths) == 0 {
		t.Fatal("scope omissions not reported")
	}
	for _, e := range c.Evidence {
		if string(e.Path) == ".env" || string(e.Excerpt) == "TOKEN=secret\n" {
			t.Fatal("secret leaked into evidence")
		}
	}
}

func TestRetrieveBudgetIsBounded(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	head := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	f := fakeObjects{tree: []byte("100644 blob " + sha + "\ta.md\x00"), blobs: map[string][]byte{sha: []byte("1234")}}
	p := source.PinnedComparison{InventoryID: "c", MergeBaseSHA: sha, Metadata: source.Metadata{Identity: source.Identity{Repository: "o/r"}, HeadSHA: head}}
	c := Retrieve(context.Background(), f, p, inventory.Inventory{}, privacy.Policy{}, Limits{Files: 1, ExcerptBytes: 4, Bytes: 4, Duration: time.Second})
	if len(c.Evidence) != 1 || len(c.Evidence[0].Excerpt) > 4 {
		t.Fatalf("budget exceeded: %+v", c)
	}
}
