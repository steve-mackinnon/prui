package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"prui/internal/testutil"
)

func TestCommitParentReaderBoundedOrderedAndNoRetry(t *testing.T) {
	for _, window := range []int{0, 1, 4} {
		t.Run(fmt.Sprint(window), func(t *testing.T) {
			var ids []string
			for i := range 100 {
				ids = append(ids, fmt.Sprint(i))
			}
			var started, consumed atomic.Int64
			calls := make([]atomic.Int64, len(ids))
			read := func(_ context.Context, id string) ([]string, error) {
				var index int
				if _, err := fmt.Sscan(id, &index); err != nil {
					return nil, err
				}
				calls[index].Add(1)
				if n := started.Add(1) - consumed.Load(); n > int64(window) && window > 0 {
					t.Errorf("unbounded read-ahead: %d", n)
				}
				if index == 2 {
					return nil, ErrLimit
				}
				return []string{id}, nil
			}
			r := newCommitParentReader(context.Background(), ids, window, read)
			defer r.Close()
			for i, id := range ids {
				consumed.Add(1)
				parents, err := r.Next()
				if i == 2 {
					if !errors.Is(err, ErrLimit) {
						t.Fatal(err)
					}
				} else if err != nil || len(parents) != 1 || parents[0] != id {
					t.Fatalf("result %d: %v, %v", i, parents, err)
				}
			}
			r.Close()
			for i := range calls {
				if calls[i].Load() != 1 {
					t.Fatalf("entry %d attempted %d times", i, calls[i].Load())
				}
			}
			if _, err := r.Next(); !errors.Is(err, io.EOF) {
				t.Fatal("reader did not finish", err)
			}
		})
	}
}

func TestCommitParentReaderDoesNotWaitForLaterSlowEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started := make(chan struct{})
	finished := make(chan struct{})
	read := func(ctx context.Context, id string) ([]string, error) {
		if id == "slow" {
			close(started)
			<-ctx.Done()
			close(finished)
			return nil, ctx.Err()
		}
		select {
		case <-started:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return []string{id}, nil
	}
	r := newCommitParentReader(ctx, []string{"fast", "slow"}, 2, read)
	parents, err := r.Next()
	if err != nil || len(parents) != 1 || parents[0] != "fast" {
		r.Close()
		t.Fatalf("first entry blocked: %v", err)
	}
	r.Close()
	select {
	case <-finished:
	default:
		t.Fatal("cancelled worker not joined")
	}
}

func TestCommitParentReaderSerialIsLazyAndOversizeIsBounded(t *testing.T) {
	calls := 0
	read := func(context.Context, string) ([]string, error) { calls++; return nil, nil }
	r := newCommitParentReader(context.Background(), []string{"a", "b"}, 0, read)
	if calls != 0 {
		t.Fatal("serial reader speculated")
	}
	if _, err := r.Next(); err != nil || calls != 1 {
		t.Fatal(err)
	}
	r.Close()
	if calls != 1 {
		t.Fatal("close read another commit")
	}
	r = newCommitParentReader(context.Background(), make([]string, 101), 4, read)
	defer r.Close()
	if _, err := r.Next(); !errors.Is(err, ErrLimit) || calls != 1 {
		t.Fatal("oversized reader attempted work", err)
	}
}

func TestCommitParentReaderMatchesRealGitValidation(t *testing.T) {
	repo := testutil.NewRepo(t)
	repo.Write("a.go", "package a\n")
	root := repo.Commit()
	repo.Write("a.go", "package a\nconst Value=1\n")
	head := repo.Commit()
	// An unreachable sibling tests that API membership is not treated as ancestry.
	tree := repo.Git("rev-parse", head+"^{tree}")
	sibling := repo.GitInput("sibling\n", "commit-tree", tree, "-p", root)
	blob := repo.Git("rev-parse", head+":a.go")
	view, err := NewView(context.Background(), repo.Dir, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	ids := []string{root, head, sibling, blob}
	reader := view.ReadCommitParents(context.Background(), head, ids)
	defer reader.Close()
	for _, id := range ids {
		want, wantErr := view.CommitParents(context.Background(), id, head)
		got, gotErr := reader.Next()
		if !reflect.DeepEqual(got, want) || (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("validation changed for %s: %v/%v, %v/%v", id, got, gotErr, want, wantErr)
		}
	}
}
