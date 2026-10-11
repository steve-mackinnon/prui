package syntax

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTokenizeReportsUnavailableReasons(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		source     []byte
		want       Status
	}{
		{"unsupported", "x.unknown-extension", []byte("hello"), Unsupported},
		{"too large", "x.ts", []byte(strings.Repeat("x", MaxBytes+1)), ByteLimit},
		{"invalid UTF-8", "x.ts", []byte{0xff}, InvalidText},
		{"token free", "x.ts", []byte("}\n"), Ready},
		{"highlighted", "x.ts", []byte("export const value = 42;\n"), Ready},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spans, status := TokenizeWithStatus(context.Background(), tc.path, tc.source)
			if status != tc.want {
				t.Fatalf("got %q, want %q", status, tc.want)
			}
			if status == Ready && spans == nil {
				t.Fatal("successful empty captures must be distinguishable from failures")
			}
			if status != Ready && spans != nil {
				t.Fatal("failed capture retained partial spans")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, status := TokenizeWithStatus(ctx, "x.ts", []byte("export const value = 42;\n")); status != Canceled {
		t.Fatalf("cancellation: %q", status)
	}
	if _, status := tokenize(context.Background(), "x.ts", []byte("export const value = 42;\n"), 0); status != Timeout {
		t.Fatalf("deadline: %q", status)
	}
}

func TestTokenLimitIsReported(t *testing.T) {
	// Exercise the span boundary independently of the deadline. Race-instrumented
	// lexing can otherwise hit the time bound before reaching the token limit.
	spans, status := tokenize(context.Background(), "x.ts", []byte(strings.Repeat("1\n", MaxSpans+1)), 10*time.Second)
	if spans != nil || status != SpanLimit {
		t.Fatalf("token limit: %q", status)
	}
}

func TestUnknownStatusDoesNotRenderUntrustedText(t *testing.T) {
	status := Status("\x1b[31mRAW LEXER ERROR")
	if strings.Contains(status.Description(), "RAW LEXER") || strings.Contains(status.Description(), "\x1b") {
		t.Fatal("untrusted status became terminal output")
	}
}
