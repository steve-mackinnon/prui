package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"prui/internal/testutil"
)

type countingBlobRunner struct {
	Runner
	calls int
}

func (r *countingBlobRunner) Run(ctx context.Context, request Request) ([]byte, error) {
	r.calls++
	return r.Runner.Run(ctx, request)
}

func TestBlobSingleBoundedRead(t *testing.T) {
	repo := testutil.NewRepo(t)
	content := "hello\x00world\n"
	oid := repo.GitInput(content, "hash-object", "-w", "--stdin")
	empty := repo.GitInput("", "hash-object", "-w", "--stdin")
	large := repo.GitInput(strings.Repeat("x", 2<<20), "hash-object", "-w", "--stdin")
	runner := &countingBlobRunner{Runner: NewRunner()}
	view, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	got, err := view.Blob(context.Background(), oid, len(content))
	if err != nil || !bytes.Equal(got, []byte(content)) {
		t.Fatalf("exact limit read = %q, %v (oid %q)", got, err, oid)
	}
	if runner.calls != 1 {
		t.Fatalf("blob read launched %d Git processes; want one", runner.calls)
	}
	got, err = view.Blob(context.Background(), oid, len(content)-1)
	if !errors.Is(err, ErrLimit) || got != nil {
		t.Fatalf("oversized blob returned %q, %v", got, err)
	}
	got, err = view.Blob(context.Background(), large, 1024)
	if !errors.Is(err, ErrLimit) || got != nil {
		t.Fatalf("large blob returned partial data or wrong error: %v", err)
	}
	got, err = view.Blob(context.Background(), empty, 1)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty blob = %q, %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := view.Blob(ctx, oid, len(content)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read: %v", err)
	}
}

func BenchmarkBlobRead(b *testing.B) {
	repo := testutil.NewRepo(b)
	oid := repo.GitInput(strings.Repeat("context\n", 1024), "hash-object", "-w", "--stdin")
	view, err := NewView(context.Background(), repo.Dir, NewRunner(), Defaults())
	if err != nil {
		b.Fatal(err)
	}
	defer view.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := view.Blob(context.Background(), oid, 32<<10); err != nil {
			b.Fatal(err)
		}
	}
}

func TestBlobReusePreservesLimitsAndOwnership(t *testing.T) {
	repo := testutil.NewRepo(t)
	content := "frozen committed source\n"
	oid := repo.GitInput(content, "hash-object", "-w", "--stdin")
	runner := &countingBlobRunner{Runner: NewRunner()}
	view, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer view.Close()
	first, err := view.Blob(context.Background(), oid, len(content))
	if err != nil {
		t.Fatal(err)
	}
	first[0] = '!'
	second, err := view.Blob(context.Background(), oid, len(content)+1)
	if err != nil || string(second) != content {
		t.Fatalf("caller changed frozen bytes: %q, %v", second, err)
	}
	if runner.calls != 1 {
		t.Fatalf("repeated immutable blob launched %d processes, want 1", runner.calls)
	}
	if data, err := view.Blob(context.Background(), oid, len(content)-1); data != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("smaller limit: %q, %v", data, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := view.Blob(ctx, oid, len(content)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cached read ignored cancellation: %v", err)
	}
	if err := view.Close(); err != nil {
		t.Fatal(err)
	}
	if data, err := view.Blob(context.Background(), oid, len(content)); data != nil || err == nil {
		t.Fatal("closed view returned cached source")
	}
}

func TestBlobReuseIsBoundedAndDoesNotCacheFailures(t *testing.T) {
	for _, cap := range []string{"bytes", "entries"} {
		t.Run(cap, func(t *testing.T) {
			calls := 0
			runner := requestFunc(func(context.Context, Request) ([]byte, error) {
				calls++
				if calls == 1 {
					return nil, ErrCommand
				}
				return []byte("source"), nil
			})
			view := &View{runner: runner, limits: Defaults()}
			oid := fmt.Sprintf("%040x", 1)
			if _, err := view.Blob(context.Background(), oid, 10); !errors.Is(err, ErrCommand) {
				t.Fatal(err)
			}
			if _, err := view.Blob(context.Background(), oid, 10); err != nil || calls != 2 {
				t.Fatal("failure suppressed retry", err, calls)
			}
			if cap == "bytes" {
				view.blobSize = blobCacheBytes
			} else {
				for i := 0; i < blobCacheEntries-1; i++ {
					view.blobs[fmt.Sprintf("%040x", i+10)] = nil
				}
			}
			uncached := fmt.Sprintf("%040x", 2)
			for i := 0; i < 2; i++ {
				if got, err := view.Blob(context.Background(), uncached, 10); err != nil || string(got) != "source" {
					t.Fatal(got, err)
				}
			}
			if calls != 4 {
				t.Fatal("cache exceeded its cap", calls)
			}
		})
	}
}
