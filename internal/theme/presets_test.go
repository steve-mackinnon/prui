package theme

import (
	"math"
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

func TestFullPresetPalettes(t *testing.T) {
	roles := []Token{Foreground, Background, Title, FileHeader, Hunk, Metadata, Added, Removed, Unavailable, Warning, Selection, FocusedBorder, Border}
	cases := map[string][]string{
		"catppuccin-mocha": {"#cdd6f4", "#1e1e2e", "#cdd6f4", "#cdd6f4", "#a6adc8", "#a6adc8", "#a6e3a1", "#f38ba8", "#f38ba8", "#f9e2af", "#313244", "#89b4fa", "#6c7086"},
		"one-dark":         {"#abb2bf", "#282c34", "#abb2bf", "#abb2bf", "#abb2bf", "#abb2bf", "#98c379", "#e06c75", "#e06c75", "#e5c07b", "#3e4451", "#61afef", "#5c6370"},
		"tokyo-night":      {"#c0caf5", "#1a1b26", "#c0caf5", "#c0caf5", "#a9b1d6", "#a9b1d6", "#9ece6a", "#f7768e", "#f7768e", "#e0af68", "#292e42", "#7aa2f7", "#737aa2"},
		"gruvbox-dark":     {"#ebdbb2", "#282828", "#ebdbb2", "#ebdbb2", "#bdae93", "#bdae93", "#b8bb26", "#fb4934", "#fb4934", "#fabd2f", "#3c3836", "#83a598", "#928374"},
	}
	for name, colors := range cases {
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
	if fresh.Syntax(Foreground) != "#cdd6f4" {
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

func TestAdditionalPresetPaletteValues(t *testing.T) {
	roles := []Token{Foreground, Background, Title, FileHeader, Hunk, Metadata, Added, Removed, Unavailable, Warning, Selection, FocusedBorder, Border}
	expected := map[string][]string{
		"ayu":                 {"#5c6773", "#fafafa", "#5c6773", "#5c6773", "#5c6773", "#5c6773", "#5d7c00", "#ff3333", "#ff3333", "#a46309", "#f0eee3", "#41a6d9", "#5c6773"},
		"ayu-dark":            {"#e6e1cf", "#0f1419", "#e6e1cf", "#e6e1cf", "#828e9f", "#828e9f", "#b8cc51", "#ff3333", "#ff3333", "#e7c547", "#243340", "#36a3d9", "#828e9f"},
		"gh-dark":             {"#e6edf3", "#0d1117", "#e6edf3", "#e6edf3", "#7d8590", "#7d8590", "#3fb950", "#f85149", "#f85149", "#d29922", "#30363d", "#2f81f7", "#8b949e"},
		"gh-light":            {"#1f2328", "#ffffff", "#1f2328", "#1f2328", "#656d76", "#656d76", "#1f883d", "#cf222e", "#cf222e", "#9a6700", "#ddf4ff", "#0969da", "#6e7781"},
		"gruvbox-light":       {"#3c3836", "#fbf1c7", "#3c3836", "#3c3836", "#7c6f64", "#7c6f64", "#79740e", "#9d0006", "#9d0006", "#b57614", "#d5c4a1", "#076678", "#7c6f64"},
		"horizon":             {"#d5d8da", "#1c1e26", "#d5d8da", "#d5d8da", "#bbbbbb", "#bbbbbb", "#29d398", "#e95678", "#e95678", "#fab795", "#2e303e", "#26bbd9", "#6c6f93"},
		"material-dark":       {"#eeffff", "#263238", "#eeffff", "#eeffff", "#b2ccd6", "#b2ccd6", "#c3e88d", "#ff5370", "#ff5370", "#ffcb6b", "#2c3b41", "#82aaff", "#65738e"},
		"material-deep-ocean": {"#8f93a2", "#0f111a", "#8f93a2", "#8f93a2", "#80869e", "#80869e", "#c3e88d", "#ff5370", "#ff5370", "#ffcb6b", "#1f2233", "#82aaff", "#80869e"},
		"melange":             {"#ece1d7", "#292522", "#ece1d7", "#ece1d7", "#c1a78e", "#c1a78e", "#85b695", "#d47766", "#d47766", "#ebc06d", "#403a36", "#a3a9ce", "#867462"},
		"monokai":             {"#f8f8f2", "#272822", "#f8f8f2", "#f8f8f2", "#99947c", "#99947c", "#a6e22e", "#f92672", "#f92672", "#e2e22e", "#414339", "#66d9ef", "#99947c"},
		"night-owl":           {"#d6deeb", "#011627", "#d6deeb", "#d6deeb", "#7e97ac", "#7e97ac", "#22da6e", "#ef5350", "#ef5350", "#ffeb95", "#1d3b53", "#82aaff", "#7e97ac"},
		"one-light":           {"#383a42", "#fafafa", "#383a42", "#383a42", "#696c77", "#696c77", "#50a14f", "#e45649", "#e45649", "#986801", "#e5e5e6", "#4078f2", "#a0a1a7"},
		"poimandres":          {"#a6accd", "#1b1e28", "#a6accd", "#a6accd", "#91b4d5", "#91b4d5", "#5fb3a1", "#d0679d", "#d0679d", "#fffac2", "#303340", "#89ddff", "#767c9d"},
		"rose-pine":           {"#e0def4", "#191724", "#e0def4", "#e0def4", "#908caa", "#908caa", "#9ccfd8", "#eb6f92", "#eb6f92", "#f6c177", "#403d52", "#c4a7e7", "#6e6a86"},
		"rose-pine-dawn":      {"#464261", "#faf4ed", "#464261", "#464261", "#797593", "#797593", "#286983", "#b4637a", "#b4637a", "#9d6110", "#dfdad9", "#907aa9", "#797593"},
		"vscode-dark":         {"#d4d4d4", "#1e1e1e", "#d4d4d4", "#d4d4d4", "#a6a6a6", "#a6a6a6", "#6a9955", "#f44747", "#f44747", "#dcdcaa", "#3a3d41", "#007acc", "#6b6b6b"},
		"vscode-light":        {"#000000", "#ffffff", "#000000", "#000000", "#6f6f6f", "#6f6f6f", "#008000", "#cd3131", "#cd3131", "#795e26", "#e5ebf1", "#007acc", "#919191"},
		"wombat":              {"#dedacf", "#171717", "#dedacf", "#dedacf", "#99968b", "#99968b", "#b1e969", "#ff6159", "#ff6159", "#ebd99c", "#453b38", "#5da9f6", "#99968b"},
	}
	for name, colors := range expected {
		t.Run(name, func(t *testing.T) {
			got, err := Resolve(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			for i, token := range roles {
				if got.Syntax(token) != colors[i] {
					t.Errorf("%s = %s, want %s", token, got.Syntax(token), colors[i])
				}
			}
			overrides := map[Token]string{Foreground: got.Syntax(Foreground), Background: "default", Added: "green"}
			custom, err := Resolve(name, overrides)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(custom.Overrides(), overrides) || custom.Syntax(Background) != "default" || custom.Syntax(Added) != "green" {
				t.Fatal("override values/provenance lost")
			}
			custom.colors[Foreground] = Color{}
			original, err := Resolve(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			if original.Syntax(Foreground) != colors[0] {
				t.Fatal("resolved palette mutated built-in")
			}
		})
	}
}

func TestAdaptedLightTextAccentsRemainReadable(t *testing.T) {
	// These light accents are intentionally darkened from upstream, so guard the
	// readable result independently of the exact color fixture above.
	luminance := func(hex string) float64 {
		value, err := ParseColor(hex)
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := value.Value().RGBA()
		linear := func(v uint32) float64 {
			channel := float64(v) / 65535
			if channel <= 0.04045 {
				return channel / 12.92
			}
			return math.Pow((channel+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
	}
	for _, tc := range []struct {
		name  string
		token Token
	}{{Ayu, Added}, {Ayu, Warning}, {RosePineDawn, Warning}} {
		palette, err := Resolve(tc.name, nil)
		if err != nil {
			t.Fatal(err)
		}
		text, background := luminance(palette.Syntax(tc.token)), luminance(palette.Syntax(Background))
		contrast := (max(text, background) + 0.05) / (min(text, background) + 0.05)
		if contrast < 4.5 {
			t.Errorf("%s %s contrast = %.2f, want >= 4.5", tc.name, tc.token, contrast)
		}
	}
}
