package main

import (
	"context"
	"crypto/sha1"
	"fmt"
	"prui/internal/inventory"
	"prui/internal/source"
	"testing"
)

type fileBlobGH struct {
	source.GitHub
	blobs        map[string][]byte
	repositories []string
}

func (g *fileBlobGH) SourceBlob(_ context.Context, repository, oid string, limit int) ([]byte, error) {
	g.repositories = append(g.repositories, repository)
	b, ok := g.blobs[oid]
	if !ok || len(b) > limit {
		return nil, source.ErrLimit
	}
	return b, nil
}
func TestLoadFileSourcePinnedSidesAndOffline(t *testing.T) {
	old, newText := []byte("old\n"), []byte("new\n")
	oid := func(b []byte) string {
		return fmt.Sprintf("%x", sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(b))), b...)))
	}
	f := inventory.FileChange{OldOID: oid(old), NewOID: oid(newText), OldMode: "100644", NewMode: "100644"}
	gh := &fileBlobGH{blobs: map[string][]byte{f.OldOID: old, f.NewOID: newText}}
	a := &application{gh: gh, limits: source.Defaults()}
	comparison := source.PinnedComparison{Metadata: source.Metadata{BaseRepository: "o/base", HeadRepository: "o/fork"}}
	data, err := a.loadFileSource(context.Background(), f, comparison)
	if err != nil || !data.Valid([]inventory.FileChange{f}) || len(gh.repositories) != 2 || gh.repositories[0] != "o/base" || gh.repositories[1] != "o/fork" {
		t.Fatal(data, err, gh.repositories)
	}
	a.offline = true
	if _, err := a.loadFileSource(context.Background(), f, comparison); err == nil || len(gh.repositories) != 2 {
		t.Fatal("offline source request ran")
	}
	a.offline = false
	gh.blobs[f.NewOID] = []byte("tampered")
	if _, err := a.loadFileSource(context.Background(), f, comparison); err == nil {
		t.Fatal("unverified source accepted")
	}
}
