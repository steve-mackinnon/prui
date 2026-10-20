package theme

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestBuiltInCatalogOrderAndIsolation(t *testing.T) {
	want := []string{"terminal", "light", "dark", "high-contrast", "catppuccin-mocha", "one-dark", "tokyo-night", "gruvbox-dark", "ayu", "ayu-dark", "gh-dark", "gh-light", "gruvbox-light", "horizon", "material-dark", "material-deep-ocean", "melange", "monokai", "night-owl", "one-light", "poimandres", "rose-pine", "rose-pine-dawn", "vscode-dark", "vscode-light", "wombat"}
	if !reflect.DeepEqual(BuiltInNames(), want) {
		t.Fatalf("names = %v, want %v", BuiltInNames(), want)
	}
	definitions := BuiltIns()
	if len(definitions) != len(want) {
		t.Fatalf("catalog length = %d, want %d", len(definitions), len(want))
	}
	for i, definition := range definitions {
		if definition.Name != want[i] || definition.DisplayName == "" || definition.Description == "" {
			t.Fatalf("incomplete catalog entry: %+v", definition)
		}
	}
	definitions[0].Name = "changed"
	if BuiltIns()[0].Name != Terminal {
		t.Fatal("catalog mutated through returned metadata")
	}
}

func TestBaseOverridesPreserveInheritedDefaultsAndProvenance(t *testing.T) {
	for _, name := range []string{Terminal, Light, Dark, HighContrast} {
		got, err := Resolve(name, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Syntax(Foreground) != "default" || got.Syntax(Background) != "default" {
			t.Fatalf("%s changed base defaults", name)
		}
	}
	overrides := map[Token]string{Foreground: "#ffffff", Background: "default"}
	got, err := Resolve("catppuccin-mocha", overrides)
	if err != nil {
		t.Fatal(err)
	}
	overrides[Foreground] = "red"
	if got.Syntax(Foreground) != "#ffffff" || got.Syntax(Background) != "default" || got.Overrides()[Foreground] != "#ffffff" {
		t.Fatal("base overrides or provenance lost")
	}
	fresh, _ := Resolve("catppuccin-mocha", nil)
	if fresh.Syntax(Foreground) != "#bfc6d4" {
		t.Fatal("resolved palette mutated catalog")
	}
}

func TestLegacyPresetSemanticColorsRemainUnchanged(t *testing.T) {
	roles := []Token{Title, FileHeader, Hunk, Added, Removed, Metadata, Warning, Unavailable, Selection, FocusedBorder, Border}
	expected := map[string][]string{
		Terminal:     {"default", "default", "default", "green", "red", "default", "bright-yellow", "bright-red", "236", "cyan", "240"},
		Light:        {"#24292f", "#24292f", "#57606a", "#116329", "#cf222e", "#57606a", "#9a6700", "#cf222e", "#d0d7de", "#0969da", "#8c959f"},
		Dark:         {"#c9d1d9", "#c9d1d9", "#8b949e", "#7ee787", "#ff7b72", "#8b949e", "#e3b341", "#ff7b72", "#30363d", "#58a6ff", "#6e7681"},
		HighContrast: {"#00ffff", "#ffff00", "#ff00ff", "#00ff00", "#ff5555", "#55aaff", "#ffff00", "#ff5555", "#555555", "#ffffff", "#aaaaaa"},
	}
	for name, colors := range expected {
		t.Run(name, func(t *testing.T) {
			got, err := Resolve(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i, role := range roles {
				if got.Syntax(role) != colors[i] {
					t.Errorf("%s = %s, want %s", role, got.Syntax(role), colors[i])
				}
			}
		})
	}
}

func TestPresetAppearanceClassification(t *testing.T) {
	light := map[string]bool{"light": true, "ayu": true, "gh-light": true, "gruvbox-light": true, "one-light": true, "rose-pine-dawn": true, "vscode-light": true}
	seen := make(map[string]bool)
	for _, definition := range BuiltIns() {
		if seen[definition.Name] {
			t.Fatalf("duplicate ID %s", definition.Name)
		}
		seen[definition.Name] = true
		want := AppearanceDark
		if definition.Name == Terminal {
			want = AppearanceTerminal
		} else if light[definition.Name] {
			want = AppearanceLight
		}
		if definition.Appearance != want {
			t.Errorf("%s appearance = %s, want %s", definition.Name, definition.Appearance, want)
		}
		resolved, err := Resolve(definition.Name, map[Token]string{Background: "default"})
		if err != nil {
			t.Fatal(err)
		}
		if resolved.IsLight() != light[definition.Name] {
			t.Errorf("%s IsLight = %v", definition.Name, resolved.IsLight())
		}
		for _, token := range Tokens() {
			if _, err := ParseColor(resolved.Syntax(token)); err != nil {
				t.Errorf("%s invalid %s: %v", definition.Name, token, err)
			}
		}
	}
	definitions := BuiltIns()
	definitions[0].Appearance = AppearanceLight
	if BuiltIns()[0].Appearance != AppearanceTerminal {
		t.Fatal("returned metadata mutated appearance")
	}
}

// Source values are pinned NvChad Base46 colors, independent of our role maps.
func TestPresetsMatchNvChadVerbatim(t *testing.T) {
	data, err := os.ReadFile("testdata/nvchad-palettes.json")
	if err != nil {
		t.Fatal(err)
	}
	var palettes map[string]struct {
		Source string
		Colors map[string]string
	}
	if err := json.Unmarshal(data, &palettes); err != nil {
		t.Fatal(err)
	}
	if len(palettes) != len(BuiltIns())-4 {
		t.Fatalf("fixture has %d palettes, want %d", len(palettes), len(BuiltIns())-4)
	}
	roles := map[Token]string{
		Foreground:      "base_16.base05",
		Background:      "base_16.base00",
		Title:           "base_16.base05",
		FileHeader:      "base_16.base05",
		Hunk:            "base_30.grey_fg",
		Metadata:        "base_30.grey_fg",
		Added:           "base_30.green",
		Removed:         "base_30.red",
		Unavailable:     "base_30.red",
		Warning:         "base_30.yellow",
		Selection:       "base_16.base02",
		FocusedBorder:   "base_30.blue",
		Border:          "base_30.line",
		SyntaxFunction:  "base_16.base0D",
		SyntaxType:      "base_16.base0A",
		SyntaxMacro:     "base_16.base08",
		SyntaxConstant:  "base_16.base09",
		SyntaxAttribute: "base_16.base0A",
		SyntaxBuiltin:   "base_16.base09",
	}
	for _, definition := range BuiltIns()[4:] {
		t.Run(definition.Name, func(t *testing.T) {
			source, ok := palettes[definition.Name]
			if !ok || source.Source == "" {
				t.Fatal("missing upstream palette")
			}
			palette, err := Resolve(definition.Name, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, token := range Tokens() {
				upstream := roles[token]
				want := source.Colors[upstream]
				if want == "" || palette.Syntax(token) != want {
					t.Errorf("%s = %s, want NvChad %s = %s", token, palette.Syntax(token), upstream, want)
				}
			}
			overrides := map[Token]string{Foreground: palette.Syntax(Foreground), Background: "default", Added: "green"}
			custom, err := Resolve(definition.Name, overrides)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(custom.Overrides(), overrides) || custom.Syntax(Background) != "default" || custom.Syntax(Added) != "green" {
				t.Fatal("override values/provenance lost")
			}
			custom.colors[Foreground] = Color{}
			original, err := Resolve(definition.Name, nil)
			if err != nil {
				t.Fatal(err)
			}
			if original.Syntax(Foreground) != source.Colors["base_16.base05"] {
				t.Fatal("resolved palette mutated built-in")
			}
		})
	}
}
