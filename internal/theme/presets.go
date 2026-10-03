// Palette values are adapted into application semantic roles. Pinned upstream
// sources and license notices are recorded in docs/THEME-ATTRIBUTION.md.
package theme

const (
	CatppuccinMocha   = "catppuccin-mocha"
	OneDark           = "one-dark"
	TokyoNight        = "tokyo-night"
	GruvboxDark       = "gruvbox-dark"
	Ayu               = "ayu"
	AyuDark           = "ayu-dark"
	GitHubDark        = "gh-dark"
	GitHubLight       = "gh-light"
	GruvboxLight      = "gruvbox-light"
	Horizon           = "horizon"
	MaterialDark      = "material-dark"
	MaterialDeepOcean = "material-deep-ocean"
	Melange           = "melange"
	Monokai           = "monokai"
	NightOwl          = "night-owl"
	OneLight          = "one-light"
	Poimandres        = "poimandres"
	RosePine          = "rose-pine"
	RosePineDawn      = "rose-pine-dawn"
	VSCodeDark        = "vscode-dark"
	VSCodeLight       = "vscode-light"
	Wombat            = "wombat"
)

// Appearance identifies a preset family for presentation and Markdown styles.
// It describes the preset even when users override its inherited color channels.
type Appearance string

const (
	AppearanceTerminal Appearance = "terminal"
	AppearanceDark     Appearance = "dark"
	AppearanceLight    Appearance = "light"
)

// Definition describes a built-in palette without exposing its mutable colors.
type Definition struct {
	Name        string
	DisplayName string
	Description string
	Appearance  Appearance
}

type preset struct {
	Definition
	colors map[Token]string
}

// BuiltIns returns independent metadata in stable catalog order.
func BuiltIns() []Definition {
	definitions := make([]Definition, len(presets))
	for i, preset := range presets {
		definitions[i] = preset.Definition
	}
	return definitions
}

// paletteColors stores the independent roles. Title/file header reuse foreground,
// hunk reuses metadata, and unavailable reuses removed, yielding all 13 tokens.
type paletteColors struct {
	foreground, background, metadata, added, removed, warning, selection, focus, border string
}

func completePalette(c paletteColors) map[Token]string {
	return map[Token]string{
		Foreground: c.foreground, Background: c.background,
		Title: c.foreground, FileHeader: c.foreground, Hunk: c.metadata,
		Metadata: c.metadata, Added: c.added, Removed: c.removed,
		Unavailable: c.removed, Warning: c.warning, Selection: c.selection,
		FocusedBorder: c.focus, Border: c.border,
	}
}

// Catalog order remains stable; the picker groups entries by Appearance.
var presets = []preset{
	{Definition: Definition{Terminal, "Terminal", "Inherits terminal colors with ANSI accents.", AppearanceTerminal}, colors: map[Token]string{Foreground: "default", Background: "default",
		Title: "default", FileHeader: "default", Hunk: "default", Added: "green",
		Removed: "red", Metadata: "default", Warning: "bright-yellow",
		Unavailable: "bright-red", Selection: "236", FocusedBorder: "cyan", Border: "240",
	}},
	{Definition: Definition{Light, "Light", "Cool accents for light terminals.", AppearanceLight}, colors: map[Token]string{Foreground: "default", Background: "default",
		Title: "#24292f", FileHeader: "#24292f", Hunk: "#57606a", Added: "#116329",
		Removed: "#cf222e", Metadata: "#57606a", Warning: "#9a6700",
		Unavailable: "#cf222e", Selection: "#d0d7de", FocusedBorder: "#0969da", Border: "#8c959f",
	}},
	{Definition: Definition{Dark, "Dark", "Cool accents for dark terminals.", AppearanceDark}, colors: map[Token]string{Foreground: "default", Background: "default",
		Title: "#c9d1d9", FileHeader: "#c9d1d9", Hunk: "#8b949e", Added: "#7ee787",
		Removed: "#ff7b72", Metadata: "#8b949e", Warning: "#e3b341",
		Unavailable: "#ff7b72", Selection: "#30363d", FocusedBorder: "#58a6ff", Border: "#6e7681",
	}},
	{Definition: Definition{HighContrast, "High Contrast", "Bright, distinct semantic accents.", AppearanceDark}, colors: map[Token]string{Foreground: "default", Background: "default",
		Title: "#00ffff", FileHeader: "#ffff00", Hunk: "#ff00ff", Added: "#00ff00",
		Removed: "#ff5555", Metadata: "#55aaff", Warning: "#ffff00",
		Unavailable: "#ff5555", Selection: "#555555", FocusedBorder: "#ffffff", Border: "#aaaaaa",
	}},
	{Definition: Definition{CatppuccinMocha, "Catppuccin Mocha", "Soft pastel accents on a deep blue background.", AppearanceDark}, colors: map[Token]string{
		Foreground:    "#cdd6f4",
		Background:    "#1e1e2e",
		Title:         "#cdd6f4",
		FileHeader:    "#cdd6f4",
		Hunk:          "#a6adc8",
		Metadata:      "#a6adc8",
		Added:         "#a6e3a1",
		Removed:       "#f38ba8",
		Unavailable:   "#f38ba8",
		Warning:       "#f9e2af",
		Selection:     "#313244",
		FocusedBorder: "#89b4fa",
		Border:        "#6c7086",
	}},
	{Definition: Definition{OneDark, "One Dark", "Muted gray surfaces with familiar Atom accents.", AppearanceDark}, colors: map[Token]string{
		Foreground:    "#abb2bf",
		Background:    "#282c34",
		Title:         "#abb2bf",
		FileHeader:    "#abb2bf",
		Hunk:          "#abb2bf",
		Metadata:      "#abb2bf",
		Added:         "#98c379",
		Removed:       "#e06c75",
		Unavailable:   "#e06c75",
		Warning:       "#e5c07b",
		Selection:     "#3e4451",
		FocusedBorder: "#61afef",
		Border:        "#5c6370",
	}},
	{Definition: Definition{TokyoNight, "Tokyo Night", "Cool blue accents on a midnight background.", AppearanceDark}, colors: map[Token]string{
		Foreground:    "#c0caf5",
		Background:    "#1a1b26",
		Title:         "#c0caf5",
		FileHeader:    "#c0caf5",
		Hunk:          "#a9b1d6",
		Metadata:      "#a9b1d6",
		Added:         "#9ece6a",
		Removed:       "#f7768e",
		Unavailable:   "#f7768e",
		Warning:       "#e0af68",
		Selection:     "#292e42",
		FocusedBorder: "#7aa2f7",
		Border:        "#737aa2",
	}},
	{Definition: Definition{GruvboxDark, "Gruvbox Dark", "Warm neutrals and bright earthy accents.", AppearanceDark}, colors: map[Token]string{
		Foreground:    "#ebdbb2",
		Background:    "#282828",
		Title:         "#ebdbb2",
		FileHeader:    "#ebdbb2",
		Hunk:          "#bdae93",
		Metadata:      "#bdae93",
		Added:         "#b8bb26",
		Removed:       "#fb4934",
		Unavailable:   "#fb4934",
		Warning:       "#fabd2f",
		Selection:     "#3c3836",
		FocusedBorder: "#83a598",
		Border:        "#928374",
	}},
	{Definition: Definition{Ayu, "Ayu", "Warm accents on a clean light surface.", AppearanceLight},
		colors: completePalette(paletteColors{"#5c6773", "#fafafa", "#5c6773", "#5d7c00", "#ff3333", "#a46309", "#f0eee3", "#41a6d9", "#5c6773"})},
	{Definition: Definition{AyuDark, "Ayu Dark", "Amber accents on a cool dark surface.", AppearanceDark},
		colors: completePalette(paletteColors{"#e6e1cf", "#0f1419", "#828e9f", "#b8cc51", "#ff3333", "#e7c547", "#243340", "#36a3d9", "#828e9f"})},
	{Definition: Definition{GitHubDark, "GitHub Dark", "GitHub dark chrome and semantic accents.", AppearanceDark},
		colors: completePalette(paletteColors{"#e6edf3", "#0d1117", "#7d8590", "#3fb950", "#f85149", "#d29922", "#30363d", "#2f81f7", "#8b949e"})},
	{Definition: Definition{GitHubLight, "GitHub Light", "GitHub light chrome and semantic accents.", AppearanceLight},
		colors: completePalette(paletteColors{"#1f2328", "#ffffff", "#656d76", "#1f883d", "#cf222e", "#9a6700", "#ddf4ff", "#0969da", "#6e7781"})},
	{Definition: Definition{GruvboxLight, "Gruvbox Light", "Warm paper with earthy accents.", AppearanceLight},
		colors: completePalette(paletteColors{"#3c3836", "#fbf1c7", "#7c6f64", "#79740e", "#9d0006", "#b57614", "#d5c4a1", "#076678", "#7c6f64"})},
	{Definition: Definition{Horizon, "Horizon", "Coral and turquoise against charcoal.", AppearanceDark},
		colors: completePalette(paletteColors{"#d5d8da", "#1c1e26", "#bbbbbb", "#29d398", "#e95678", "#fab795", "#2e303e", "#26bbd9", "#6c6f93"})},
	{Definition: Definition{MaterialDark, "Material Dark", "Material accents on a blue-gray surface.", AppearanceDark},
		colors: completePalette(paletteColors{"#eeffff", "#263238", "#b2ccd6", "#c3e88d", "#ff5370", "#ffcb6b", "#2c3b41", "#82aaff", "#65738e"})},
	{Definition: Definition{MaterialDeepOcean, "Material Deep Ocean", "Material accents on a deep ocean surface.", AppearanceDark},
		colors: completePalette(paletteColors{"#8f93a2", "#0f111a", "#80869e", "#c3e88d", "#ff5370", "#ffcb6b", "#1f2233", "#82aaff", "#80869e"})},
	{Definition: Definition{Melange, "Melange", "Soft earth tones on warm charcoal.", AppearanceDark},
		colors: completePalette(paletteColors{"#ece1d7", "#292522", "#c1a78e", "#85b695", "#d47766", "#ebc06d", "#403a36", "#a3a9ce", "#867462"})},
	{Definition: Definition{Monokai, "Monokai", "Classic vivid accents on an olive-black base.", AppearanceDark},
		colors: completePalette(paletteColors{"#f8f8f2", "#272822", "#99947c", "#a6e22e", "#f92672", "#e2e22e", "#414339", "#66d9ef", "#99947c"})},
	{Definition: Definition{NightOwl, "Night Owl", "Bright accents against midnight blue.", AppearanceDark},
		colors: completePalette(paletteColors{"#d6deeb", "#011627", "#7e97ac", "#22da6e", "#ef5350", "#ffeb95", "#1d3b53", "#82aaff", "#7e97ac"})},
	{Definition: Definition{OneLight, "One Light", "Atom light neutrals and balanced accents.", AppearanceLight},
		colors: completePalette(paletteColors{"#383a42", "#fafafa", "#696c77", "#50a14f", "#e45649", "#986801", "#e5e5e6", "#4078f2", "#a0a1a7"})},
	{Definition: Definition{Poimandres, "Poimandres", "Teal and pale blue on a deep slate base.", AppearanceDark},
		colors: completePalette(paletteColors{"#a6accd", "#1b1e28", "#91b4d5", "#5fb3a1", "#d0679d", "#fffac2", "#303340", "#89ddff", "#767c9d"})},
	{Definition: Definition{RosePine, "Rose Pine", "Muted rose and foam on a violet base.", AppearanceDark},
		colors: completePalette(paletteColors{"#e0def4", "#191724", "#908caa", "#9ccfd8", "#eb6f92", "#f6c177", "#403d52", "#c4a7e7", "#6e6a86"})},
	{Definition: Definition{RosePineDawn, "Rose Pine Dawn", "Soft rose accents on warm cream.", AppearanceLight},
		colors: completePalette(paletteColors{"#464261", "#faf4ed", "#797593", "#286983", "#b4637a", "#9d6110", "#dfdad9", "#907aa9", "#797593"})},
	{Definition: Definition{VSCodeDark, "VSCode Dark", "Visual Studio dark neutrals and blue focus.", AppearanceDark},
		colors: completePalette(paletteColors{"#d4d4d4", "#1e1e1e", "#a6a6a6", "#6a9955", "#f44747", "#dcdcaa", "#3a3d41", "#007acc", "#6b6b6b"})},
	{Definition: Definition{VSCodeLight, "VSCode Light", "Visual Studio light neutrals and blue focus.", AppearanceLight},
		colors: completePalette(paletteColors{"#000000", "#ffffff", "#6f6f6f", "#008000", "#cd3131", "#795e26", "#e5ebf1", "#007acc", "#919191"})},
	{Definition: Definition{Wombat, "Wombat", "Warm gray with lively green and blue accents.", AppearanceDark},
		colors: completePalette(paletteColors{"#dedacf", "#171717", "#99968b", "#b1e969", "#ff6159", "#ebd99c", "#453b38", "#5da9f6", "#99968b"})},
}

// IsLight reports the catalog appearance, preserving it across color overrides.
func (t Theme) IsLight() bool {
	for _, preset := range presets {
		if preset.Name == t.Name {
			return preset.Appearance == AppearanceLight
		}
	}
	return false
}
