package source

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"prui/internal/testutil"
)

func testBlobDiffArgs(old, next string) []string {
	return []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--text", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--inter-hunk-context=0", "--full-index", old, next, "--"}
}

func TestBlobDiffReuseBoundsFailuresCancellationAndClose(t *testing.T) {
	args := testBlobDiffArgs(fmt.Sprintf("%040x", 1), fmt.Sprintf("%040x", 2))
	calls := 0
	v := &View{dir: t.TempDir(), limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, ErrCommand
		}
		return []byte("patch"), nil
	})}
	if _, err := v.Git(context.Background(), 10, args...); !errors.Is(err, ErrCommand) {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if got, err := v.Git(context.Background(), 10, args...); err != nil || string(got) != "patch" {
			t.Fatal("failure suppressed later read", got, err)
		}
	}
	if calls != 2 {
		t.Fatal("successful diff not reused", calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := v.Git(ctx, 10, args...); !errors.Is(err, context.Canceled) {
		t.Fatal("cached diff ignored cancellation", err)
	}
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if len(v.diffs) != 0 || v.diffSize != 0 {
		t.Fatal("closed view retained patch cache")
	}
	if got, err := v.Git(context.Background(), 10, args...); got != nil || err == nil || calls != 2 {
		t.Fatal("closed view returned patch", got, err, calls)
	}
	for _, bound := range []string{"bytes", "entries"} {
		t.Run(bound, func(t *testing.T) {
			reads := 0
			view := &View{limits: Defaults(), diffs: map[string][]byte{}, runner: requestFunc(func(context.Context, Request) ([]byte, error) {
				reads++
				return []byte("patch"), nil
			})}
			if bound == "bytes" {
				view.diffSize = diffCacheBytes
			} else {
				for i := 0; i < diffCacheEntries; i++ {
					view.diffs[fmt.Sprint(i)] = nil
				}
			}
			for i := 0; i < 2; i++ {
				if _, err := view.Git(context.Background(), 10, args...); err != nil {
					t.Fatal(err)
				}
			}
			if reads != 2 {
				t.Fatal("diff cache exceeded its cap", reads)
			}
		})
	}
}

func TestBlobDiffReusePreservesOptionsLimitsAndOwnership(t *testing.T) {
	repo := testutil.NewRepo(t)
	old := repo.GitInput("old\n", "hash-object", "-w", "--stdin")
	next := repo.GitInput("new\n", "hash-object", "-w", "--stdin")
	runner := &countingBlobRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	args := testBlobDiffArgs(old, next)
	first, err := v.Git(context.Background(), 10000, args...)
	if err != nil || len(first) == 0 {
		t.Fatal(err)
	}
	want := string(first)
	first[0] = '!'
	second, err := v.Git(context.Background(), 10000, args...)
	if err != nil || string(second) != want || runner.calls != 1 {
		t.Fatalf("reused patch changed or relaunched Git: err=%v calls=%d", err, runner.calls)
	}
	if data, err := v.Git(context.Background(), len(second)-1, args...); data != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("reused patch ignored limit: %q %v", data, err)
	}
	args[8] = "--unified=0"
	for i := 0; i < 2; i++ {
		if _, err := v.Git(context.Background(), 10000, args...); err != nil {
			t.Fatal(err)
		}
	}
	if runner.calls != 3 {
		t.Fatal("noncanonical diff options reused the wrong cache entry", runner.calls)
	}
}
