package syntax

import (
	"context"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
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

func TestDistinctSymbolCategories(t *testing.T) {
	for token, want := range map[chroma.TokenType]Kind{
		chroma.NameFunction: Function, chroma.NameFunctionMagic: Function,
		chroma.NameClass: Type, chroma.KeywordType: Type,
		chroma.NameConstant: Constant, chroma.KeywordConstant: Constant,
		chroma.NameAttribute: Attribute, chroma.NameDecorator: Attribute,
		chroma.CommentPreproc: Attribute, chroma.NameBuiltin: Builtin,
		chroma.Name: Plain, chroma.NameOther: Plain,
	} {
		if got := category(token); got != want {
			t.Errorf("%s: got %d, want %d", token, got, want)
		}
	}
}

func TestRustSymbolHighlighting(t *testing.T) {
	source := "struct Cache {}\nconst CACHE_SIZE: usize = 10;\npub fn cache_dir() { format!(\"hello\"); }\n"
	lines := strings.Split(source, "\n")
	tokens := Tokenize(context.Background(), "assets.rs", []byte(source))
	for _, tc := range []struct {
		line int
		text string
		kind Kind
	}{
		{1, "Cache", Type}, {2, "CACHE_SIZE", Constant}, {2, "usize", Type},
		{3, "cache_dir", Function}, {3, "format!", Macro},
	} {
		found := false
		for _, span := range tokens[tc.line] {
			if lines[tc.line-1][span.Start:span.End] == tc.text && span.Kind == tc.kind {
				found = true
			}
		}
		if !found {
			t.Errorf("missing category %d for %q: %v", tc.kind, tc.text, tokens[tc.line])
		}
	}
}

func TestLegacyCategoryValuesRemainStable(t *testing.T) {
	for value, kind := range []Kind{Plain, Keyword, String, Number, Comment, Name, Operator} {
		if int(kind) != value {
			t.Fatalf("persisted kind changed: %d != %d", kind, value)
		}
	}
}

func TestPythonSpecialMethodsAreNotMacros(t *testing.T) {
	source := "class Cache:\n    @staticmethod\n    def __init__(self):\n        pass\n"
	lines := strings.Split(source, "\n")
	tokens := Tokenize(context.Background(), "cache.py", []byte(source))
	for _, tc := range []struct {
		line int
		text string
		kind Kind
	}{
		{1, "Cache", Type}, {2, "@staticmethod", Attribute}, {3, "__init__", Function},
	} {
		found := false
		for _, span := range tokens[tc.line] {
			if lines[tc.line-1][span.Start:span.End] == tc.text && span.Kind == tc.kind {
				found = true
			}
		}
		if !found {
			t.Errorf("missing category %d for %q: %v", tc.kind, tc.text, tokens[tc.line])
		}
	}
}
