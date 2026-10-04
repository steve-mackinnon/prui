package source

import (
	"strings"
	"testing"
)

func TestSuggestionRoundTripAndReplacement(t *testing.T) {
	for _, replacement := range []string{"new\ntext", "", "```\ncode", "tab\tvalue"} {
		body, err := SuggestionBody(replacement)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseSuggestion(body)
		if err != nil || got != replacement {
			t.Fatalf("roundtrip %q: %q %v", replacement, got, err)
		}
	}
	target := ReviewCommentTarget{Identity: Identity{"o/r", 1}, CommitID: strings.Repeat("a", 40), Path: "a.go", Side: "RIGHT", StartLine: 2, StartSide: "RIGHT", Line: 3}
	got, err := ReplaceSuggestion("one\ntwo\nthree\nfour\n", target, "two\nthree", "new")
	if err != nil || got != "one\nnew\nfour\n" {
		t.Fatalf("replacement %q %v", got, err)
	}
	if _, err = ReplaceSuggestion("one\nchanged\nthree\n", target, "two\nthree", "new"); err == nil {
		t.Fatal("conflict accepted")
	}
	target.Side = "LEFT"
	if _, err = ReplaceSuggestion("one\ntwo\nthree\n", target, "two\nthree", "new"); err == nil {
		t.Fatal("old side accepted")
	}
	for _, body := range []string{"plain", "```suggestion:-1+0\nx\n```", "```suggestion\nx", "```suggestion\nx\n```\n```suggestion\ny\n```"} {
		if _, err := ParseSuggestion(body); err == nil {
			t.Fatalf("unsupported body accepted %q", body)
		}
	}
}

func TestReplaceSuggestionPreservesEOFAndDeletion(t *testing.T) {
	target := ReviewCommentTarget{Identity: Identity{"o/r", 1}, CommitID: strings.Repeat("a", 40), Path: "a", Side: "RIGHT", Line: 2}
	for _, tc := range []struct{ in, before, replacement, want string }{{"first\nlast", "last", "new", "first\nnew"}, {"first\nlast\n", "last", "", "first\n"}, {"first\r\nlast\r\n", "last\r", "new\r", "first\r\nnew\r\n"}} {
		got, err := ReplaceSuggestion(tc.in, target, tc.before, tc.replacement)
		if err != nil || got != tc.want {
			t.Fatalf("%q %v", got, err)
		}
	}
}

func TestSuggestionLiteralNestedFenceAndUnchanged(t *testing.T) {
	replacement := "```suggestion\nexample\n```"
	body, err := SuggestionBody(replacement)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSuggestion(body)
	if err != nil || got != replacement {
		t.Fatal(got, err)
	}
	if _, err = ParseSuggestion("````markdown\n```suggestion\nnot real\n```\n````"); err == nil {
		t.Fatal("nested prose fence treated as suggestion")
	}
	target := ReviewCommentTarget{Identity: Identity{"o/r", 1}, CommitID: strings.Repeat("a", 40), Path: "a", Side: "RIGHT", Line: 1}
	if _, err = ReplaceSuggestion("same\n", target, "same", "same"); err == nil {
		t.Fatal("already applied replacement accepted")
	}
}

func TestSuggestionMarkdownExamplesAreNotActionable(t *testing.T) {
	for _, body := range []string{"~~~markdown\n```suggestion\nexample\n```\n~~~", "    ```suggestion\n    example\n    ```", "\t```suggestion\n\texample\n\t```", "> ```suggestion\n> example\n> ```"} {
		if _, err := ParseSuggestion(body); err == nil {
			t.Fatalf("literal example accepted %q", body)
		}
	}
	got, err := ParseSuggestion("```suggestion\nnew\n````")
	if err != nil || got != "new" {
		t.Fatal("long closing fence", got, err)
	}
}
