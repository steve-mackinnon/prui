package syntax

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestTokenCacheExactIdentityAndIndependentResults(t *testing.T) {
	c := &tokenCache{}
	calls := 0
	compute := func(context.Context, string, []byte) (Lines, Status) {
		calls++
		return Lines{1: {{Start: 0, End: 1, Kind: Keyword}}}, Ready
	}
	first, _ := c.capture(context.Background(), "a.go", []byte("a"), compute)
	first[1][0].Kind = String
	delete(first, 1)
	second, status := c.capture(context.Background(), "a.go", []byte("a"), compute)
	if calls != 1 || status != Ready || second[1][0].Kind != Keyword {
		t.Fatal("cached result was aliased or recomputed")
	}
	second[1][0].Kind = Number
	third, _ := c.capture(context.Background(), "a.go", []byte("a"), compute)
	if third[1][0].Kind != Keyword {
		t.Fatal("cache hit returned shared spans")
	}
	c.capture(context.Background(), "b.py", []byte("a"), compute)
	c.capture(context.Background(), "a.go", []byte("b"), compute)
	if calls != 3 {
		t.Fatal("path or source was omitted from identity")
	}
}

func TestTokenCacheDoesNotRetainFailuresOrBypassCancellation(t *testing.T) {
	for _, status := range []Status{Unsupported, ByteLimit, SpanLimit, Timeout, Canceled, LexerError, InvalidOffsets, InvalidText} {
		c := &tokenCache{}
		calls := 0
		compute := func(context.Context, string, []byte) (Lines, Status) { calls++; return nil, status }
		for range 2 {
			c.capture(context.Background(), "a.go", []byte("x"), compute)
		}
		if calls != 2 || len(c.entries) != 0 || c.bytes != 0 {
			t.Fatalf("failure %v cached", status)
		}
	}
	c := &tokenCache{}
	compute := func(context.Context, string, []byte) (Lines, Status) { return Lines{}, Ready }
	c.capture(context.Background(), "a.go", nil, compute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lines, status := c.capture(ctx, "a.go", nil, compute); lines != nil || status != Canceled {
		t.Fatal("cached success bypassed cancellation")
	}
}

func TestTokenCacheCapsAndOpenScope(t *testing.T) {
	c := &tokenCache{}
	compute := func(context.Context, string, []byte) (Lines, Status) { return Lines{1: {{0, 1, Keyword}}}, Ready }
	for i := 0; i < tokenCacheEntries+10; i++ {
		c.capture(context.Background(), fmt.Sprintf("%d.go", i), nil, compute)
	}
	if len(c.entries) != tokenCacheEntries || c.bytes > tokenCacheBytes {
		t.Fatal("unbounded entry storage")
	}
	c = &tokenCache{}
	large := Lines{}
	for i := 0; i < MaxSpans; i++ {
		large[i] = []Span{{0, 1, Keyword}}
	}
	for i := 0; i < 10; i++ {
		c.capture(context.Background(), fmt.Sprintf("%d.go", i), nil, func(context.Context, string, []byte) (Lines, Status) { return large, Ready })
	}
	if c.bytes > tokenCacheBytes || len(c.entries) != 1 {
		t.Fatal("unbounded span storage")
	}
	a, b := WithCache(context.Background()), WithCache(context.Background())
	if a.Value(tokenCacheKey{}) == b.Value(tokenCacheKey{}) {
		t.Fatal("cache shared across opens")
	}
}

func TestTokenCacheMatchesUncachedAndConcurrentUse(t *testing.T) {
	ctx := WithCache(context.Background())
	var group sync.WaitGroup
	for _, path := range []string{"a.go", "a.py", "a.rs", "a.unknown"} {
		path := path
		group.Go(func() {
			for _, text := range []string{"package main\nfunc main() {}\n", "", "invalid\xff"} {
				want, ws := TokenizeWithStatus(context.Background(), path, []byte(text))
				for range 2 {
					got, gs := TokenizeWithStatus(ctx, path, []byte(text))
					if gs != ws || !reflect.DeepEqual(got, want) {
						t.Errorf("cached %s differs: %v/%v", path, gs, ws)
					}
				}
			}
		})
	}
	group.Wait()
}

// Two captures model the same pinned blob in the main comparison and its PR
// commit. Each iteration starts a new scope, so no warm cross-open hit is used.
func BenchmarkTokenizeCaptureReuse(b *testing.B) {
	source := []byte("package example\n" + strings.Repeat("func value() string { return \"example\" }\n", 100))
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached=%t", cached), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				ctx := context.Background()
				if cached {
					ctx = WithCache(ctx)
				}
				for range 2 {
					if _, status := TokenizeWithStatus(ctx, "example.go", source); status != Ready {
						b.Fatal(status)
					}
				}
			}
		})
	}
}
