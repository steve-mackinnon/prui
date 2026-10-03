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

var presets = append([]preset{
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
}, additionalPresets...)

// IsLight reports the catalog appearance, preserving it across color overrides.
func (t Theme) IsLight() bool {
	for _, preset := range presets {
		if preset.Name == t.Name {
			return preset.Appearance == AppearanceLight
		}
	}
	return false
}
