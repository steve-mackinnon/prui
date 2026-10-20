package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prui/internal/testutil"
)

func TestTreeDiffBatchMatchesCanonicalBlobPatches(t *testing.T) {
	repo := testutil.NewRepo(t)
	repo.Write(".gitattributes", "*.c diff=cpp\n")
	oldC := "  int func() {\n" + strings.Repeat("    // context\n", 12) + "    return 1;\n  }\n"
	newC := strings.Replace(oldC, "return 1", "return 2", 1)
	repo.Write("code.c", oldC)
	repo.Write("plain", "old\ndiff --git a/f b/f\n")
	repo.Write("gone", "gone")
	repo.Write("mode", "old\n")
	repo.Write("link", "target-old")
	if err := os.Remove(filepath.Join(repo.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-old", filepath.Join(repo.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	base := repo.Commit()
	repo.Write("code.c", newC)
	repo.Write("plain", "new\ndiff --git a/x b/x\n")
	repo.Git("rm", "gone")
	repo.Write("added", "added\n")
	repo.Write("mode", "new\n")
	if err := os.Chmod(filepath.Join(repo.Dir, "mode"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target-new", filepath.Join(repo.Dir, "link")); err != nil {
		t.Fatal(err)
	}
	head := repo.Commit()
	runner := &countingBlobRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	empty, err := v.Git(context.Background(), 100, "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	emptyID := strings.TrimSpace(string(empty))
	var requests []TreeDiffRequest
	for _, path := range []string{"plain", "gone", "mode", "added", "code.c", "link"} {
		old, next, status := emptyID, emptyID, "M"
		if path != "added" {
			old = repo.Git("rev-parse", base+":"+path)
		} else {
			status = "A"
		}
		if path != "gone" {
			next = repo.Git("rev-parse", head+":"+path)
		} else {
			status = "D"
		}
		requests = append(requests, TreeDiffRequest{BlobDiffRequest: BlobDiffRequest{OldOID: old, NewOID: next, Limit: 10000}, Path: path, Status: status})
	}
	before := runner.calls
	got := v.TreeDiffBatch(context.Background(), base, head, requests)
	if len(got) != len(requests) || runner.calls != before+1 {
		t.Fatalf("tree batch results=%d calls=%d, want %d/1", len(got), runner.calls-before, len(requests))
	}
	// A new view ensures the oracle cannot read results seeded by the batch.
	oracle, err := NewView(context.Background(), repo.Dir, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	if _, err := oracle.Git(context.Background(), 100, "hash-object", "-w", "--stdin"); err != nil {
		t.Fatal(err)
	}
	for i, request := range requests {
		want, err := oracle.Git(context.Background(), request.Limit, BlobDiffArgs(request.OldOID, request.NewOID)...)
		if err != nil || got[i].Err != nil || !bytes.Equal(got[i].Data, want) {
			t.Fatalf("tree patch changed %s: err=%v/%v\ngot=%q\nwant=%q", request.Path, err, got[i].Err, got[i].Data, want)
		}
		got[i].Data[0] = '!'
		cached, err := v.Git(context.Background(), request.Limit, BlobDiffArgs(request.OldOID, request.NewOID)...)
		if err != nil || !bytes.Equal(cached, want) {
			t.Fatal("tree result changed cached patch ownership", request.Path, err)
		}
	}
	if runner.calls != before+1 {
		t.Fatal("tree batch did not seed canonical diff reuse")
	}
}

func treeFixtureRequests() []TreeDiffRequest {
	return []TreeDiffRequest{
		{BlobDiffRequest: BlobDiffRequest{OldOID: strings.Repeat("a", 40), NewOID: strings.Repeat("b", 40), Limit: 10000}, Path: "first", Status: "M"},
		{BlobDiffRequest: BlobDiffRequest{OldOID: strings.Repeat("c", 40), NewOID: strings.Repeat("d", 40), Limit: 10000}, Path: "second", Status: "M"},
	}
}

func treeFixturePatch(r TreeDiffRequest) string {
	return fmt.Sprintf("diff --git a/%s b/%s\nindex %s..%s 100644\n--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-old\n+new\n", r.Path, r.Path, r.OldOID, r.NewOID, r.Path, r.Path)
}

func TestTreeDiffBatchRejectsMalformedFramesWithoutRetentionOrRetry(t *testing.T) {
	requests := treeFixtureRequests()
	first, second := treeFixturePatch(requests[0]), treeFixturePatch(requests[1])
	for name, output := range map[string]string{
		"missing frame":   first,
		"duplicate frame": first + first,
		"wrong identity":  first + strings.Replace(second, requests[1].OldOID, strings.Repeat("e", 40), 1),
		"wrong mode":      first + strings.Replace(second, "100644", "160000", 1),
		"wrong path":      first + strings.ReplaceAll(second, "second", "third"),
		"truncated body":  first + strings.TrimSuffix(second, "\n"),
		"unknown header":  first + strings.Replace(second, "index ", "unrecognized ", 1),
		"no hunk":         first + second[:strings.Index(second, "@@ ")],
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			v := &View{limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
				calls++
				return []byte(output), nil
			})}
			got := v.TreeDiffBatch(context.Background(), strings.Repeat("1", 40), strings.Repeat("2", 40), requests)
			if len(got) != 2 || calls != 1 || len(v.diffs) != 0 {
				t.Fatal("malformed batch retained data or retried", got, calls)
			}
			for _, result := range got {
				if result.Data != nil || !errors.Is(result.Err, ErrCommand) {
					t.Fatal("malformed frame became source", result)
				}
			}
		})
	}
}

func TestTreeDiffBatchPreservesOutputFailuresAndCancellation(t *testing.T) {
	requests := treeFixtureRequests()
	calls := 0
	v := &View{limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
		calls++
		return nil, ErrLimit
	})}
	got := v.TreeDiffBatch(context.Background(), strings.Repeat("1", 40), strings.Repeat("2", 40), requests)
	for _, result := range got {
		if result.Data != nil || !errors.Is(result.Err, ErrLimit) {
			t.Fatal("tree output failure hidden", result)
		}
	}
	if len(got) != 2 || calls != 1 || len(v.diffs) != 0 {
		t.Fatal("tree output failure retried or retained", got, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got = v.TreeDiffBatch(ctx, strings.Repeat("1", 40), strings.Repeat("2", 40), requests)
	if len(got) != 2 || calls != 1 {
		t.Fatal("canceled tree batch launched Git", got, calls)
	}
	for _, result := range got {
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatal("tree batch ignored cancellation", result)
		}
	}
}

func TestTreeDiffBatchLeavesUnsupportedPathsAndUnreservedJobsUnread(t *testing.T) {
	requests := treeFixtureRequests()
	calls := 0
	v := &View{limits: Defaults(), runner: requestFunc(func(context.Context, Request) ([]byte, error) {
		calls++
		return nil, ErrCommand
	})}
	for _, path := range []string{"space name", "bad\xff", "/absolute", "a/../b", "a//b", ":(glob)*", strings.Repeat("x", 257)} {
		for i := range requests {
			requests[i].Path = path
		}
		if got := v.TreeDiffBatch(context.Background(), strings.Repeat("1", 40), strings.Repeat("2", 40), requests); len(got) != 0 || calls != 0 {
			t.Fatal("unsupported path was attempted", path, got, calls)
		}
	}
	requests = treeFixtureRequests()
	v.limits.ContentBytes = requests[0].Limit
	if got := v.TreeDiffBatch(context.Background(), strings.Repeat("1", 40), strings.Repeat("2", 40), requests); len(got) != 0 || calls != 0 {
		t.Fatal("unreserved jobs were attempted", got, calls)
	}
}
