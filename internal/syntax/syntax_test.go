package syntax

import (
	"context"
	"strings"
	"testing"
)

func TestFullContextAndBounds(t *testing.T) {
	source := "package p\n/*\n" + strings.Repeat("comment\n", 12) + "*/\nvar s = `hello`\n"
	lines := Tokenize(context.Background(), "x.go", []byte(source))
	if len(lines[10]) == 0 || lines[10][0].Kind != Comment {
		t.Fatalf("missing multiline context: %#v", lines[10])
	}
	if Tokenize(context.Background(), "x.go", []byte(strings.Repeat("x", MaxBytes+1))) != nil {
		t.Fatal("oversize source highlighted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Tokenize(ctx, "x.go", []byte(source)) != nil {
		t.Fatal("canceled work highlighted")
	}
	if Tokenize(context.Background(), "x.unknown-extension", []byte(source)) != nil {
		t.Fatal("unknown language highlighted")
	}
}
