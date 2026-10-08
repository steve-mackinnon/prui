package theme

import (
	"testing"
)

func TestResolveBuiltInsProvideEverySemanticToken(t *testing.T) {
	for _, name := range BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			got, err := Resolve(name, nil)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", name, err)
			}
			if got.Name != name {
				t.Fatalf("resolved name = %q, want %q", got.Name, name)
			}
			for _, token := range Tokens() {
				if _, ok := got.Color(token); !ok {
					t.Fatalf("missing color for %q", token)
				}
			}
		})
	}
}

func TestTerminalUsesInheritedChromeAndSemanticAccents(t *testing.T) {
	got, err := Resolve(Terminal, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[Token]string{
		Title:         "default",
		FileHeader:    "default",
		Hunk:          "default",
		Added:         "green",
		Removed:       "red",
		Metadata:      "default",
		Warning:       "bright-yellow",
		Unavailable:   "bright-red",
		Selection:     "236",
		FocusedBorder: "cyan",
		Border:        "240",
	}
	for token, expected := range want {
		if actual := got.Syntax(token); actual != expected {
			t.Errorf("%s = %q, want %q", token, actual, expected)
		}
	}
}

func TestSymbolPaletteIsIndependentOfChromeOverrides(t *testing.T) {
	for _, name := range BuiltInNames() {
		base, err := Resolve(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		custom, err := Resolve(name, map[Token]string{Warning: "red", FocusedBorder: "red", SyntaxFunction: "#123456"})
		if err != nil {
			t.Fatal(err)
		}
		if custom.Syntax(SyntaxFunction) != "#123456" || custom.Syntax(SyntaxType) != base.Syntax(SyntaxType) {
			t.Fatalf("%s: syntax override mixed with chrome", name)
		}
		if base.Syntax(SyntaxFunction) == base.Syntax(FocusedBorder) {
			t.Fatalf("%s: functions indistinguishable from keywords", name)
		}
		if base.Syntax(SyntaxType) == base.Syntax(Foreground) || base.Syntax(SyntaxMacro) == base.Syntax(Foreground) {
			t.Fatalf("%s: symbols use plain foreground", name)
		}
	}
}

func TestParseColorAcceptsDocumentedSyntax(t *testing.T) {
	for _, input := range []string{
		"#112233", "#AABBCC", "0", "15", "255", "default",
		"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
		"bright-black", "bright-red", "bright-green", "bright-yellow", "bright-blue", "bright-magenta", "bright-cyan", "bright-white",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := ParseColor(input)
			if err != nil {
				t.Fatalf("ParseColor(%q): %v", input, err)
			}
			if got.Syntax() != input {
				t.Fatalf("syntax = %q, want %q", got.Syntax(), input)
			}
		})
	}
}

func TestParseColorRejectsUndocumentedOrUnsafeSyntax(t *testing.T) {
	for _, input := range []string{
		"", "#123", "#12345", "#1234567", "#gg0000", "-1", "256", "01", "RED", "orange", "\x1b[31m", "red;1",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseColor(input); err == nil {
				t.Fatalf("ParseColor(%q) succeeded", input)
			}
		})
	}
}

func TestResolveRejectsUnknownThemeAndOverrideToken(t *testing.T) {
	if _, err := Resolve("solarized", nil); err == nil {
		t.Fatal("unknown theme succeeded")
	}
	if _, err := Resolve(Dark, map[Token]string{"not-a-token": "red"}); err == nil {
		t.Fatal("unknown token override succeeded")
	}
	if _, err := Resolve(Dark, map[Token]string{Added: "#123"}); err == nil {
		t.Fatal("invalid color override succeeded")
	}
}

func TestResolveAppliesValidatedOverridesWithoutMutatingBuiltIn(t *testing.T) {
	got, err := Resolve(Dark, map[Token]string{Added: "#112233", Selection: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Syntax(Added) != "#112233" || got.Syntax(Selection) != "default" {
		t.Fatalf("overrides = added %q, selection %q", got.Syntax(Added), got.Syntax(Selection))
	}
	base, err := Resolve(Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	if base.Syntax(Added) == "#112233" || base.Syntax(Selection) == "default" {
		t.Fatalf("base theme was mutated: added %q, selection %q", base.Syntax(Added), base.Syntax(Selection))
	}
}
