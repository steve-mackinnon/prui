package context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"pr-review/internal/privacy"
	"pr-review/internal/source"
	"strings"
	"testing"
)

func searchFixture(t *testing.T) *SearchCorpus {
	t.Helper()
	f := fakeObjects{tree: []byte("100644 blob a\tcode.go\x00100644 blob b\tdocs.md\x00100644 blob c\tconfig.go\x00120000 blob d\tlink\x00100644 blob e\tinvalid.go\x00"), blobs: map[string][]byte{"a": []byte("one\ntwo\nneedle\nfour\nfive\nsix\n"), "b": []byte("needle docs\n"), "c": []byte("safe first line\npassword = synthetic\n"), "d": []byte("needle"), "e": {0xff}}}
	c, err := PrepareSearch(context.Background(), f, source.PinnedComparison{MergeBaseSHA: "base", Metadata: source.Metadata{HeadSHA: "head", Identity: source.Identity{Repository: "o/r"}}}, privacy.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestSearchPinnedTextAndWholeBlobPrivacy(t *testing.T) {
	c := searchFixture(t)
	if len(c.Files) != 4 {
		t.Fatalf("files = %d", len(c.Files))
	}
	r, err := c.Search(context.Background(), "needle", "", "head")
	if err != nil || len(r.Evidence) != 2 {
		t.Fatalf("result=%+v error=%v", r, err)
	}
	e := r.Evidence[0]
	if string(e.Path) != "code.go" || e.CommitSHA != "head" || e.BlobID != "a" || e.LineStart != 1 || e.LineEnd != 5 || string(e.Excerpt) != "one\ntwo\nneedle\nfour\nfive\n" {
		t.Fatalf("incorrect provenance: %+v", e)
	}
	r2, _ := c.ReadLines(context.Background(), "code.go", 1, 5, "head")
	if r2.Evidence[0].EvidenceID != e.EvidenceID {
		t.Fatal("same excerpt must have stable ID across tools")
	}
	for _, p := range []string{"config.go", "link", "invalid.go", "missing"} {
		r, err = c.ReadLines(context.Background(), p, 1, 1, "head")
		if err != nil || len(r.Evidence) != 0 || r.Status != "unavailable" {
			t.Fatalf("denied result: %+v %v", r, err)
		}
	}
}
func TestSearchValidationCancellationAndLiteralMatching(t *testing.T) {
	c := searchFixture(t)
	for _, args := range [][3]string{{"", "", "head"}, {strings.Repeat("x", 257), "", "head"}, {"x", "[", "head"}, {"x", "", "working"}} {
		if _, err := c.Search(context.Background(), args[0], args[1], args[2]); err == nil {
			t.Fatal("accepted invalid arguments")
		}
	}
	r, _ := c.Search(context.Background(), ".*", "", "head")
	if len(r.Evidence) != 0 || r.Status != "no_matches" {
		t.Fatal("search was not literal")
	}
	r, _ = c.Search(context.Background(), "needle", "*.md", "merge_base")
	if len(r.Evidence) != 1 || r.Evidence[0].CommitSHA != "base" {
		t.Fatal("glob or revision ignored")
	}
	if _, err := c.ReadLines(context.Background(), "code.go", 1, 201, "head"); err == nil {
		t.Fatal("accepted oversized read")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Search(ctx, "needle", "", "head"); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := PrepareSearch(ctx, fakeObjects{}, source.PinnedComparison{}, privacy.Policy{}); err == nil {
		t.Fatal("ignored preparation cancellation")
	}
}
func TestSearchResultLimitsKeepWholeLines(t *testing.T) {
	c := &SearchCorpus{Files: []SearchFile{{Revision: "head", Evidence: Evidence{Path: []byte("a"), Excerpt: []byte(strings.Repeat("needle\n", 100)), CommitSHA: "head"}}}}
	r, err := c.Search(context.Background(), "needle", "", "head")
	if err != nil || !r.Incomplete || len(r.Evidence) != 20 {
		t.Fatalf("match limit: %+v %v", r, err)
	}
	c.Files[0].Evidence.Excerpt = []byte("short\n" + strings.Repeat("x", 20000) + "\nlast\n")
	r, err = c.ReadLines(context.Background(), "a", 1, 3, "head")
	raw, _ := json.Marshal(r)
	if err != nil || !r.Incomplete || len(raw) > 16<<10 || len(r.Evidence) != 1 || string(r.Evidence[0].Excerpt) != "short\n" || r.Evidence[0].LineEnd != 1 {
		t.Fatalf("byte limit: %+v %v bytes=%d", r, err, len(raw))
	}
}

func TestPrepareSearchBoundsAndCancellationDuringRead(t *testing.T) {
	t.Run("entry budget includes both revisions", func(t *testing.T) {
		var tree strings.Builder
		for i := 0; i < 6000; i++ {
			fmt.Fprintf(&tree, "100644 blob a\tfile-%05d\x00", i)
		}
		calls := 0
		c, err := PrepareSearch(context.Background(), fakeObjects{tree: []byte(tree.String()), blobs: map[string][]byte{"a": []byte("x\n")}, blobCalls: &calls}, searchComparison(), privacy.Policy{})
		if err != nil || !c.Incomplete || len(c.Files) != 10000 || calls != 10000 {
			t.Fatalf("files=%d calls=%d incomplete=%v err=%v", len(c.Files), calls, c.Incomplete, err)
		}
	})
	t.Run("oversized blobs and invalid paths withheld", func(t *testing.T) {
		c, err := PrepareSearch(context.Background(), fakeObjects{tree: []byte("100644 blob a\tlarge.go\x00100644 blob b\tbad\xff\x00"), blobs: map[string][]byte{"a": []byte(strings.Repeat("x", (1<<20)+1)), "b": []byte("safe")}}, searchComparison(), privacy.Policy{})
		if err != nil || !c.Incomplete || len(c.Files) != 0 || len(c.Omissions) != 4 {
			t.Fatalf("result=%+v err=%v", c, err)
		}
	})
	t.Run("parent cancellation in object read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := PrepareSearch(ctx, cancellingSearchObjects{cancel: cancel}, searchComparison(), privacy.Policy{})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	})
}
func searchComparison() source.PinnedComparison {
	return source.PinnedComparison{MergeBaseSHA: "base", Metadata: source.Metadata{HeadSHA: "head"}}
}

type cancellingSearchObjects struct{ cancel context.CancelFunc }

func (f cancellingSearchObjects) Git(context.Context, int, ...string) ([]byte, error) {
	return []byte("100644 blob a\tfile.go\x00"), nil
}
func (f cancellingSearchObjects) Blob(context.Context, string, int) ([]byte, error) {
	f.cancel()
	return []byte("safe"), nil
}

func TestSearchWholeLineByteLimitAndTrailingNewline(t *testing.T) {
	c := searchFixture(t)
	r, err := c.ReadLines(context.Background(), "code.go", 6, 9, "head")
	if err != nil || len(r.Evidence) != 1 || r.Evidence[0].LineEnd != 6 || string(r.Evidence[0].Excerpt) != "six\n" {
		t.Fatalf("result=%+v error=%v", r, err)
	}
	c.Files = []SearchFile{{Revision: "head", Evidence: Evidence{Path: []byte("code.go"), Excerpt: []byte("before\n" + strings.Repeat("needle", 5000) + "\nafter\n")}}}
	r, err = c.Search(context.Background(), "needle", "", "head")
	if err != nil || !r.Incomplete || r.Status != "unavailable" || len(r.Evidence) != 0 {
		t.Fatalf("oversized match must not become truncated evidence: %+v %v", r, err)
	}
}

func TestSearchOversizedMatchesNeverReportEmptySuccess(t *testing.T) {
	c := &SearchCorpus{Files: []SearchFile{{Revision: "head", Evidence: Evidence{Path: []byte("a"), Excerpt: []byte(strings.Repeat(strings.Repeat("x", 20000)+"needle\n", 21))}}}}
	r, err := c.Search(context.Background(), "needle", "", "head")
	if err != nil || r.Status != "unavailable" || !r.Incomplete || len(r.Evidence) != 0 {
		t.Fatalf("result=%+v err=%v", r, err)
	}
}
