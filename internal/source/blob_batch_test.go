package source

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"prui/internal/testutil"
)

func TestBlobBatchMatchesIndividualReads(t *testing.T) {
	repo := testutil.NewRepo(t)
	want := map[string]string{}
	var ids []string
	for i := 0; i < 20; i++ {
		content := fmt.Sprintf("binary\x00source %d\n", i)
		oid := repo.GitInput(content, "hash-object", "-w", "--stdin")
		ids = append(ids, oid)
		want[oid] = content
	}
	runner := &countingBlobRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	got := v.BlobBatch(context.Background(), append(ids, ids[0]), 100)
	if runner.calls != 2 || len(got) != len(ids) {
		t.Fatalf("batch calls=%d results=%d", runner.calls, len(got))
	}
	for _, oid := range ids {
		result := got[oid]
		if result.Err != nil || string(result.Data) != want[oid] {
			t.Fatalf("batch %s = %q, %v", oid, result.Data, result.Err)
		}
		result.Data[0] = '!'
		data, err := v.Blob(context.Background(), oid, 100)
		if err != nil || string(data) != want[oid] {
			t.Fatalf("batch changed cache ownership: %q, %v", data, err)
		}
	}
	if runner.calls != 2 {
		t.Fatal("batch did not populate bounded reuse", runner.calls)
	}
}

func TestBlobBatchMissingOversizeEmptyAndCancellation(t *testing.T) {
	repo := testutil.NewRepo(t)
	empty := repo.GitInput("", "hash-object", "-w", "--stdin")
	large := repo.GitInput(strings.Repeat("x", 20), "hash-object", "-w", "--stdin")
	missing := strings.Repeat("a", 40)
	runner := &countingBlobRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	got := v.BlobBatch(context.Background(), []string{empty, large, missing}, 10)
	if got[empty].Err != nil || len(got[empty].Data) != 0 || !errors.Is(got[large].Err, ErrLimit) || !errors.Is(got[missing].Err, ErrCommand) {
		t.Fatalf("batch results: %#v", got)
	}
	if runner.calls != 2 {
		t.Fatal("unexpected batch calls", runner.calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = v.BlobBatch(ctx, []string{empty}, 10)
	if !errors.Is(got[empty].Err, context.Canceled) || runner.calls != 2 {
		t.Fatal("canceled batch read cached bytes or ran Git", got, runner.calls)
	}
}

func TestBlobBatchRejectsMalformedOutputAndDoesNotRetry(t *testing.T) {
	oid := strings.Repeat("a", 40)
	header := oid + " blob 3\n"
	for _, body := range []string{header + "abc", header + "abc!", header + "abc\nextra", strings.Repeat("b", 40) + " blob 3\nabc\n"} {
		t.Run(fmt.Sprintf("%q", body), func(t *testing.T) {
			calls := 0
			v := &View{limits: Defaults(), runner: requestFunc(func(_ context.Context, request Request) ([]byte, error) {
				calls++
				if calls == 1 {
					return []byte(header), nil
				}
				return []byte(body), nil
			})}
			got := v.BlobBatch(context.Background(), []string{oid}, 10)
			if !errors.Is(got[oid].Err, ErrCommand) || got[oid].Data != nil || len(v.blobs) != 0 || calls != 2 {
				t.Fatal("accepted malformed batch or retried", got, calls)
			}
		})
	}
}

func TestBlobBatchMetadataFailuresAndOutputLimit(t *testing.T) {
	oid := strings.Repeat("a", 40)
	for _, metadata := range []string{oid + " blob -1\n", oid + " blob 03\n", oid + " blob 9999999999999999999999999\n", oid + " blob 3", oid + " blob 3\nextra\n", strings.Repeat("b", 40) + " blob 3\n"} {
		t.Run(fmt.Sprintf("%q", metadata), func(t *testing.T) {
			calls := 0
			v := &View{limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
				calls++
				return []byte(metadata), nil
			})}
			got := v.BlobBatch(context.Background(), []string{oid}, 10)
			if !errors.Is(got[oid].Err, ErrCommand) || got[oid].Data != nil || calls != 1 {
				t.Fatal("invalid metadata caused content read or retry", got, calls)
			}
		})
	}
	calls := 0
	v := &View{limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
		calls++
		return nil, ErrLimit
	})}
	got := v.BlobBatch(context.Background(), []string{oid}, 10)
	if !errors.Is(got[oid].Err, ErrLimit) || got[oid].Data != nil || calls != 1 {
		t.Fatal("resource limit was retried or discarded", got, calls)
	}
}

func TestBlobBatchFitsCacheBeforeReadingContents(t *testing.T) {
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)
	header := first + " blob 3\n"
	calls := 0
	v := &View{blobSize: blobCacheBytes - 3, limits: Defaults(), runner: requestFunc(func(_ context.Context, q Request) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte(header + second + " blob 4\n"), nil
		}
		if string(q.Stdin) != first+"\n" || q.Limit != len(header)+4 {
			t.Fatalf("unbounded batch content request: input=%q limit=%d", q.Stdin, q.Limit)
		}
		return []byte(header + "abc\n"), nil
	})}
	got := v.BlobBatch(context.Background(), []string{first, second}, 10)
	if len(got) != 1 || string(got[first].Data) != "abc" || v.blobSize != blobCacheBytes || calls != 2 {
		t.Fatal("cache budget not enforced before content read", got, v.blobSize, calls)
	}
	if _, attempted := got[second]; attempted {
		t.Fatal("unread object reported as attempted")
	}
}
