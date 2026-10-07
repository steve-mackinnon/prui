package source

import (
	"context"
	"strings"
	"testing"
)

func TestSourceBlobPinnedBoundedRead(t *testing.T) {
	oid := strings.Repeat("a", 40)
	calls := 0
	g := &GH{Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		if r.Limit != 1024 || r.Args[len(r.Args)-1] != "repos/o/r/git/blobs/"+oid {
			t.Fatal(r)
		}
		return []byte("hello\n"), nil
	})}
	b, err := g.SourceBlob(context.Background(), "o/r", oid, 1024)
	if err != nil || string(b) != "hello\n" {
		t.Fatal(string(b), err)
	}
	for _, bad := range []string{"main", "--help", "../"} {
		if _, err := g.SourceBlob(context.Background(), "o/r", bad, 1024); err == nil {
			t.Fatal("accepted mutable/invalid object")
		}
	}
	if calls != 1 {
		t.Fatal("invalid request ran command")
	}
}
