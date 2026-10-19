package syntax

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func prefixLines(lines Lines, through int) Lines {
	result := Lines{}
	for line, spans := range lines {
		if through <= 0 || line <= through {
			result[line] = spans
		}
	}
	return result
}
func TestTokenizeThroughMatchesFullContext(t *testing.T) {
	for path, text := range map[string]string{
		"x.go":   "package p\n/* multi\nline comment\n*/\nfunc value() string { return `multi\nline string` }\n",
		"x.ts":   "/* comment\ncontinued */\nexport const template = `hello\n${value}`;\nconst next = 42;\n",
		"x.py":   "value = '''multi\nline string\nend'''\ndef next():\n  return 42\n",
		"x.rs":   "/* multi\nline */\nfn value() { let x = r#\"hello\nworld\"#; println!(\"{}\", x); }\n",
		"x.html": "<html>\n<script>\nconst x = `multi\nline`;\n</script>\n</html>\n",
		"x.yaml": "key: value\nnext: true\n",
	} {
		t.Run(path, func(t *testing.T) {
			source := []byte(text)
			full, status := TokenizeWithStatus(context.Background(), path, source)
			if status != Ready {
				t.Fatal(status)
			}
			for _, ctx := range []context.Context{context.Background(), WithCache(context.Background())} {
				for _, through := range []int{1, 3, 2, 100, 0, 4, 1} {
					got, status := TokenizeThrough(ctx, path, source, through)
					if status != Ready || !reflect.DeepEqual(got, prefixLines(full, through)) {
						t.Fatalf("line %d: %v, %#v; want %#v", through, status, got, prefixLines(full, through))
					}
				}
			}
		})
	}
}
func TestTokenizeThroughKeepsBoundsAndFullCacheIndependent(t *testing.T) {
	text := []byte(strings.Repeat("const x = 1;\n", 50))
	ctx := WithCache(context.Background())
	limited, status := TokenizeThrough(ctx, "x.ts", text, 2)
	if status != Ready || len(limited) == 0 {
		t.Fatal(status)
	}
	full, status := TokenizeWithStatus(ctx, "x.ts", text)
	if status != Ready || len(full[50]) == 0 {
		t.Fatal("partial cache replaced full capture")
	}
	limited[1][0].Kind = Plain
	again, status := TokenizeThrough(ctx, "x.ts", text, 2)
	if status != Ready || !reflect.DeepEqual(again, prefixLines(full, 2)) {
		t.Fatal("partial cache aliases callers")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, status := TokenizeThrough(cancelled, "x.ts", text, 2); status != Canceled {
		t.Fatal(status)
	}
	for _, tc := range []struct {
		input []byte
		want  Status
	}{
		{[]byte("const x=1;\ninvalid\xff"), InvalidText},
		{[]byte(strings.Repeat("x", MaxBytes+1)), ByteLimit},
	} {
		if _, status := TokenizeThrough(ctx, "x.ts", tc.input, 1); status != tc.want {
			t.Fatal(status)
		}
	}
}
func TestTokenizeThroughConcurrentCacheExtents(t *testing.T) {
	ctx := WithCache(context.Background())
	text := []byte(strings.Repeat("const x = 1;\n", 100))
	full, _ := TokenizeWithStatus(context.Background(), "x.ts", text)
	var group sync.WaitGroup
	for _, through := range []int{1, 3, 10, 0, 50, 100} {
		group.Go(func() {
			for range 3 {
				got, status := TokenizeThrough(ctx, "x.ts", text, through)
				if status != Ready || !reflect.DeepEqual(got, prefixLines(full, through)) {
					t.Errorf("extent %d changed", through)
				}
			}
		})
	}
	group.Wait()
	cache := ctx.Value(tokenCacheKey{}).(*tokenCache)
	if cache.bytes > tokenCacheBytes || len(cache.entries) > tokenCacheEntries {
		t.Fatal("unbounded prefix cache")
	}
}
func BenchmarkTokenizeThrough(b *testing.B) {
	source := []byte("package p\n" + strings.Repeat("func value() string { return \"example\" }\n", 1000))
	for _, tc := range []struct {
		name    string
		through int
	}{{"full", 0}, {"first-10", 10}, {"first-100", 100}} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_, status := TokenizeThrough(context.Background(), "x.go", source, tc.through)
				if status != Ready {
					b.Fatal(status)
				}
			}
		})
	}
}

func TestTokenizeThroughPreservesSpanCapAndDeadline(t *testing.T) {
	source := []byte(strings.Repeat("1\n", MaxSpans+1))
	got, status := tokenizeThrough(context.Background(), "x.ts", source, 10*time.Second, 2)
	if status != Ready || len(got) != 2 {
		t.Fatal(status, len(got))
	}
	if _, status := tokenizeThrough(context.Background(), "x.ts", source, 10*time.Second, MaxSpans+1); status != SpanLimit {
		t.Fatal(status)
	}
	if _, status := tokenizeThrough(context.Background(), "x.ts", source, 0, 2); status != Timeout {
		t.Fatal(status)
	}
}

func TestTokenizeThroughRetainsOffsetRejection(t *testing.T) {
	// This lexer normalizes a YAML block-scalar token. Preserve the original
	// rejection when that token is in the requested extent or full capture.
	source := []byte("key: |\n  multi\n  line\nnext: true\n")
	for _, ctx := range []context.Context{context.Background(), WithCache(context.Background())} {
		_, _ = TokenizeThrough(ctx, "x.yaml", source, 1)
		if _, status := TokenizeThrough(ctx, "x.yaml", source, 100); status != InvalidOffsets {
			t.Fatal(status)
		}
		if _, status := TokenizeWithStatus(ctx, "x.yaml", source); status != InvalidOffsets {
			t.Fatal(status)
		}
	}
}
