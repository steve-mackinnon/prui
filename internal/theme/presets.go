// Named palettes use exact NvChad Base46 RGB values mapped to application roles.
// Pinned sources and license notices are recorded in docs/THEME-ATTRIBUTION.md.
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

// Legacy terminal presets retain their original syntax defaults. Named presets
// define all syntax roles explicitly from their NvChad Base46 palette.
func syntaxPalette(base map[Token]string, appearance Appearance) map[Token]string {
	typeColor, macroColor, constantColor := "#79c0c0", "#d2a8ff", "#ffa657"
	switch appearance {
	case AppearanceLight:
		typeColor, macroColor, constantColor = "#055d73", "#6639ba", "#953800"
	case AppearanceTerminal:
		typeColor, macroColor, constantColor = "cyan", "magenta", "bright-yellow"
	}
	return map[Token]string{
		SyntaxFunction: base[Warning], SyntaxType: typeColor,
		SyntaxMacro: macroColor, SyntaxConstant: constantColor,
		SyntaxAttribute: macroColor, SyntaxBuiltin: typeColor,
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
	{Definition: Definition{CatppuccinMocha, "Catppuccin Mocha", "Soft pastel accents on a deep blue background.", AppearanceDark},
		// NvChad: catppuccin
		colors: map[Token]string{
			Foreground: "#bfc6d4", Background: "#1e1d2d",
			Title: "#bfc6d4", FileHeader: "#bfc6d4", Hunk: "#4e4d5d",
			Added: "#abe9b3", Removed: "#f38ba8", Unavailable: "#f38ba8",
			Metadata: "#4e4d5d", Warning: "#fae3b0", Selection: "#2f2e3e",
			FocusedBorder: "#89b4fa", Border: "#383747",
			SyntaxFunction: "#89b4fa", SyntaxType: "#fae3b0", SyntaxMacro: "#f38ba8",
			SyntaxConstant: "#f8bd96", SyntaxAttribute: "#fae3b0", SyntaxBuiltin: "#f8bd96",
		}},
	{Definition: Definition{OneDark, "One Dark", "Muted gray surfaces with familiar Atom accents.", AppearanceDark},
		// NvChad: onedark
		colors: map[Token]string{
			Foreground: "#abb2bf", Background: "#1e222a",
			Title: "#abb2bf", FileHeader: "#abb2bf", Hunk: "#565c64",
			Added: "#98c379", Removed: "#e06c75", Unavailable: "#e06c75",
			Metadata: "#565c64", Warning: "#e7c787", Selection: "#3e4451",
			FocusedBorder: "#61afef", Border: "#31353d",
			SyntaxFunction: "#61afef", SyntaxType: "#e5c07b", SyntaxMacro: "#e06c75",
			SyntaxConstant: "#d19a66", SyntaxAttribute: "#e5c07b", SyntaxBuiltin: "#d19a66",
		}},
	{Definition: Definition{TokyoNight, "Tokyo Night", "Cool blue accents on a midnight background.", AppearanceDark},
		// NvChad: tokyonight
		colors: map[Token]string{
			Foreground: "#a9b1d6", Background: "#1a1b26",
			Title: "#a9b1d6", FileHeader: "#a9b1d6", Hunk: "#565f89",
			Added: "#9ece6a", Removed: "#f7768e", Unavailable: "#f7768e",
			Metadata: "#565f89", Warning: "#e0af68", Selection: "#2f3549",
			FocusedBorder: "#7aa2f7", Border: "#32333e",
			SyntaxFunction: "#2ac3de", SyntaxType: "#0db9d7", SyntaxMacro: "#73daca",
			SyntaxConstant: "#ff9e64", SyntaxAttribute: "#0db9d7", SyntaxBuiltin: "#ff9e64",
		}},
	{Definition: Definition{GruvboxDark, "Gruvbox Dark", "Warm neutrals and bright earthy accents.", AppearanceDark},
		// NvChad: gruvbox
		colors: map[Token]string{
			Foreground: "#d5c4a1", Background: "#282828",
			Title: "#d5c4a1", FileHeader: "#d5c4a1", Hunk: "#4e4e4e",
			Added: "#b8bb26", Removed: "#fb4934", Unavailable: "#fb4934",
			Metadata: "#4e4e4e", Warning: "#d79921", Selection: "#423e3c",
			FocusedBorder: "#458588", Border: "#36393a",
			SyntaxFunction: "#83a598", SyntaxType: "#fabd2f", SyntaxMacro: "#fb4934",
			SyntaxConstant: "#fe8019", SyntaxAttribute: "#fabd2f", SyntaxBuiltin: "#fe8019",
		}},
	{Definition: Definition{Ayu, "Ayu", "Warm accents on a clean light surface.", AppearanceLight},
		// NvChad: ayu_light
		colors: map[Token]string{
			Foreground: "#5c6166", Background: "#fafafa",
			Title: "#5c6166", FileHeader: "#5c6166", Hunk: "#b9b9b9",
			Added: "#6cbf43", Removed: "#e65050", Unavailable: "#e65050",
			Metadata: "#b9b9b9", Warning: "#e6ba7e", Selection: "#eeeeee",
			FocusedBorder: "#399ee6", Border: "#e1e1e1",
			SyntaxFunction: "#55b4d4", SyntaxType: "#399ee6", SyntaxMacro: "#f07171",
			SyntaxConstant: "#a37acc", SyntaxAttribute: "#399ee6", SyntaxBuiltin: "#a37acc",
		}},
	{Definition: Definition{AyuDark, "Ayu Dark", "Amber accents on a cool dark surface.", AppearanceDark},
		// NvChad: ayu_dark
		colors: map[Token]string{
			Foreground: "#c9c7be", Background: "#0b0e14",
			Title: "#c9c7be", FileHeader: "#c9c7be", Hunk: "#3d4046",
			Added: "#aad84c", Removed: "#f07178", Unavailable: "#f07178",
			Metadata: "#3d4046", Warning: "#e7c547", Selection: "#24272d",
			FocusedBorder: "#36a3d9", Border: "#24272d",
			SyntaxFunction: "#f07174", SyntaxType: "#56c3f9", SyntaxMacro: "#c9c7be",
			SyntaxConstant: "#ffee99", SyntaxAttribute: "#56c3f9", SyntaxBuiltin: "#ffee99",
		}},
	{Definition: Definition{GitHubDark, "GitHub Dark", "GitHub dark chrome and semantic accents.", AppearanceDark},
		// NvChad: github_dark
		colors: map[Token]string{
			Foreground: "#c9d1d9", Background: "#24292e",
			Title: "#c9d1d9", FileHeader: "#c9d1d9", Hunk: "#565b60",
			Added: "#56d364", Removed: "#ff7f8d", Unavailable: "#ff7f8d",
			Metadata: "#565b60", Warning: "#ffdf5d", Selection: "#383d42",
			FocusedBorder: "#79c0ff", Border: "#33383d",
			SyntaxFunction: "#6ab1f0", SyntaxType: "#ffdf5d", SyntaxMacro: "#b392e9",
			SyntaxConstant: "#ffab70", SyntaxAttribute: "#ffdf5d", SyntaxBuiltin: "#ffab70",
		}},
	{Definition: Definition{GitHubLight, "GitHub Light", "GitHub light chrome and semantic accents.", AppearanceLight},
		// NvChad: github_light
		colors: map[Token]string{
			Foreground: "#383d42", Background: "#ffffff",
			Title: "#383d42", FileHeader: "#383d42", Hunk: "#bcbec0",
			Added: "#18654b", Removed: "#de2c2e", Unavailable: "#de2c2e",
			Metadata: "#bcbec0", Warning: "#dbab09", Selection: "#e1e3e5",
			FocusedBorder: "#0d7fdd", Border: "#eaecee",
			SyntaxFunction: "#005cc5", SyntaxType: "#b08800", SyntaxMacro: "#5a32a3",
			SyntaxConstant: "#b93a86", SyntaxAttribute: "#b08800", SyntaxBuiltin: "#b93a86",
		}},
	{Definition: Definition{GruvboxLight, "Gruvbox Light", "Warm paper with earthy accents.", AppearanceLight},
		// NvChad: gruvbox_light
		colors: map[Token]string{
			Foreground: "#504945", Background: "#f2e5bc",
			Title: "#504945", FileHeader: "#504945", Hunk: "#b6a980",
			Added: "#79740e", Removed: "#d65d0e", Unavailable: "#d65d0e",
			Metadata: "#b6a980", Warning: "#d79921", Selection: "#e5d8af",
			FocusedBorder: "#458588", Border: "#ded1a8",
			SyntaxFunction: "#076678", SyntaxType: "#b57614", SyntaxMacro: "#9d0006",
			SyntaxConstant: "#af3a03", SyntaxAttribute: "#b57614", SyntaxBuiltin: "#af3a03",
		}},
	{Definition: Definition{Horizon, "Horizon", "Coral and turquoise against charcoal.", AppearanceDark},
		// NvChad: horizon
		colors: map[Token]string{
			Foreground: "#d5d8da", Background: "#1d1f27",
			Title: "#d5d8da", FileHeader: "#d5d8da", Hunk: "#5b607b",
			Added: "#27d796", Removed: "#e95678", Unavailable: "#e95678",
			Metadata: "#5b607b", Warning: "#fac29a", Selection: "#3a324a",
			FocusedBorder: "#25b2bc", Border: "#3b3c41",
			SyntaxFunction: "#59c2ff", SyntaxType: "#fabd2f", SyntaxMacro: "#e95678",
			SyntaxConstant: "#fab795", SyntaxAttribute: "#fabd2f", SyntaxBuiltin: "#fab795",
		}},
	{Definition: Definition{MaterialDark, "Material Dark", "Material accents on a dark gray surface.", AppearanceDark},
		// NvChad: material-darker
		colors: map[Token]string{
			Foreground: "#dff0f0", Background: "#212121",
			Title: "#dff0f0", FileHeader: "#dff0f0", Hunk: "#545454",
			Added: "#c3e88d", Removed: "#f07178", Unavailable: "#f07178",
			Metadata: "#545454", Warning: "#ffcb6b", Selection: "#303030",
			FocusedBorder: "#82aaff", Border: "#383838",
			SyntaxFunction: "#82aaff", SyntaxType: "#ffcb6b", SyntaxMacro: "#b0bec5",
			SyntaxConstant: "#f78c6c", SyntaxAttribute: "#ffcb6b", SyntaxBuiltin: "#f78c6c",
		}},
	{Definition: Definition{MaterialDeepOcean, "Material Deep Ocean", "Material accents on a deep ocean surface.", AppearanceDark},
		// NvChad: material-deep-ocean
		colors: map[Token]string{
			Foreground: "#eeffff", Background: "#0f111a",
			Title: "#eeffff", FileHeader: "#eeffff", Hunk: "#46537d",
			Added: "#c3e88d", Removed: "#f07178", Unavailable: "#f07178",
			Metadata: "#46537d", Warning: "#ffcb6b", Selection: "#2d3550",
			FocusedBorder: "#82aaff", Border: "#313855",
			SyntaxFunction: "#82aaff", SyntaxType: "#ffcb6b", SyntaxMacro: "#f07178",
			SyntaxConstant: "#f78c6c", SyntaxAttribute: "#ffcb6b", SyntaxBuiltin: "#f78c6c",
		}},
	{Definition: Definition{Melange, "Melange", "Soft earth tones on warm charcoal.", AppearanceDark},
		// NvChad: melange
		colors: map[Token]string{
			Foreground: "#ece1d7", Background: "#2a2520",
			Title: "#ece1d7", FileHeader: "#ece1d7", Hunk: "#605b56",
			Added: "#86a3a3", Removed: "#b65c60", Unavailable: "#b65c60",
			Metadata: "#605b56", Warning: "#e3b865", Selection: "#433e39",
			FocusedBorder: "#9aacce", Border: "#39342f",
			SyntaxFunction: "#ebc06d", SyntaxType: "#99d59d", SyntaxMacro: "#ece1d7",
			SyntaxConstant: "#86a3a3", SyntaxAttribute: "#99d59d", SyntaxBuiltin: "#86a3a3",
		}},
	{Definition: Definition{Monokai, "Monokai", "Classic vivid accents on an olive-black base.", AppearanceDark},
		// NvChad: monekai
		colors: map[Token]string{
			Foreground: "#f8f8f2", Background: "#272822",
			Title: "#f8f8f2", FileHeader: "#f8f8f2", Hunk: "#555650",
			Added: "#96c367", Removed: "#e36d76", Unavailable: "#e36d76",
			Metadata: "#555650", Warning: "#e6c181", Selection: "#49483e",
			FocusedBorder: "#51afef", Border: "#363942",
			SyntaxFunction: "#66d9ef", SyntaxType: "#f4bf75", SyntaxMacro: "#fd971f",
			SyntaxConstant: "#ae81ff", SyntaxAttribute: "#f4bf75", SyntaxBuiltin: "#ae81ff",
		}},
	{Definition: Definition{NightOwl, "Night Owl", "Bright accents against midnight blue.", AppearanceDark},
		// NvChad: nightowl
		colors: map[Token]string{
			Foreground: "#ced6e3", Background: "#011627",
			Title: "#ced6e3", FileHeader: "#ced6e3", Hunk: "#34495a",
			Added: "#29e68e", Removed: "#f78c6c", Unavailable: "#f78c6c",
			Metadata: "#34495a", Warning: "#ffcb8b", Selection: "#172c3d",
			FocusedBorder: "#82aaff", Border: "#182d3e",
			SyntaxFunction: "#82aaff", SyntaxType: "#c792ea", SyntaxMacro: "#ecc48d",
			SyntaxConstant: "#f78c6c", SyntaxAttribute: "#c792ea", SyntaxBuiltin: "#f78c6c",
		}},
	{Definition: Definition{OneLight, "One Light", "Atom light neutrals and balanced accents.", AppearanceLight},
		// NvChad: one_light
		colors: map[Token]string{
			Foreground: "#383a42", Background: "#fafafa",
			Title: "#383a42", FileHeader: "#383a42", Hunk: "#b0b0b1",
			Added: "#50a14f", Removed: "#d84a3d", Unavailable: "#d84a3d",
			Metadata: "#b0b0b1", Warning: "#c18401", Selection: "#e5e5e6",
			FocusedBorder: "#4078f2", Border: "#e2e2e2",
			SyntaxFunction: "#4078f2", SyntaxType: "#c18401", SyntaxMacro: "#d84a3d",
			SyntaxConstant: "#d75f00", SyntaxAttribute: "#c18401", SyntaxBuiltin: "#d75f00",
		}},
	{Definition: Definition{Poimandres, "Poimandres", "Teal and pale blue on a deep slate base.", AppearanceDark},
		// NvChad: poimandres
		colors: map[Token]string{
			Foreground: "#a6accd", Background: "#1b1e28",
			Title: "#a6accd", FileHeader: "#a6accd", Hunk: "#515977",
			Added: "#5fb3a1", Removed: "#d0679d", Unavailable: "#d0679d",
			Metadata: "#515977", Warning: "#fffac2", Selection: "#32384a",
			FocusedBorder: "#89ddff", Border: "#303340",
			SyntaxFunction: "#add7ff", SyntaxType: "#5de4c7", SyntaxMacro: "#a6accd",
			SyntaxConstant: "#d0679d", SyntaxAttribute: "#5de4c7", SyntaxBuiltin: "#d0679d",
		}},
	{Definition: Definition{RosePine, "Rose Pine", "Muted rose and foam on a violet base.", AppearanceDark},
		// NvChad: rosepine
		colors: map[Token]string{
			Foreground: "#e0def4", Background: "#191724",
			Title: "#e0def4", FileHeader: "#e0def4", Hunk: "#474552",
			Added: "#abe9b3", Removed: "#eb6f92", Unavailable: "#eb6f92",
			Metadata: "#474552", Warning: "#f6c177", Selection: "#26233a",
			FocusedBorder: "#8bbec7", Border: "#2e2c39",
			SyntaxFunction: "#c4a7e7", SyntaxType: "#ebbcba", SyntaxMacro: "#eb6f92",
			SyntaxConstant: "#f6c177", SyntaxAttribute: "#ebbcba", SyntaxBuiltin: "#f6c177",
		}},
	{Definition: Definition{RosePineDawn, "Rose Pine Dawn", "Soft rose accents on warm cream.", AppearanceLight},
		// NvChad: rosepine-dawn
		colors: map[Token]string{
			Foreground: "#575279", Background: "#faf4ed",
			Title: "#575279", FileHeader: "#575279", Hunk: "#a39ead",
			Added: "#286983", Removed: "#b4637a", Unavailable: "#b4637a",
			Metadata: "#a39ead", Warning: "#ea9d34", Selection: "#f2e9e1",
			FocusedBorder: "#56949f", Border: "#eadccf",
			SyntaxFunction: "#907aa9", SyntaxType: "#d7827e", SyntaxMacro: "#b4637a",
			SyntaxConstant: "#ea9d34", SyntaxAttribute: "#d7827e", SyntaxBuiltin: "#ea9d34",
		}},
	{Definition: Definition{VSCodeDark, "VSCode Dark", "Visual Studio dark neutrals and blue focus.", AppearanceDark},
		// NvChad: vscode_dark
		colors: map[Token]string{
			Foreground: "#d4d4d4", Background: "#1e1e1e",
			Title: "#d4d4d4", FileHeader: "#d4d4d4", Hunk: "#4e4e4e",
			Added: "#b5cea8", Removed: "#d16969", Unavailable: "#d16969",
			Metadata: "#4e4e4e", Warning: "#d7ba7d", Selection: "#303030",
			FocusedBorder: "#569cd6", Border: "#2e2e2e",
			SyntaxFunction: "#dcdcaa", SyntaxType: "#d7ba7d", SyntaxMacro: "#d16969",
			SyntaxConstant: "#b5cea8", SyntaxAttribute: "#d7ba7d", SyntaxBuiltin: "#b5cea8",
		}},
	{Definition: Definition{VSCodeLight, "VSCode Light", "Visual Studio light neutrals and blue focus.", AppearanceLight},
		// NvChad: vscode_light
		colors: map[Token]string{
			Foreground: "#343434", Background: "#ffffff",
			Title: "#343434", FileHeader: "#343434", Hunk: "#c7c7c7",
			Added: "#008000", Removed: "#ff0000", Unavailable: "#ff0000",
			Metadata: "#c7c7c7", Warning: "#795e26", Selection: "#dfdfdf",
			FocusedBorder: "#007acc", Border: "#e6e6e6",
			SyntaxFunction: "#0000ff", SyntaxType: "#af00db", SyntaxMacro: "#007acc",
			SyntaxConstant: "#0451a5", SyntaxAttribute: "#af00db", SyntaxBuiltin: "#0451a5",
		}},
	{Definition: Definition{Wombat, "Wombat", "Warm gray with lively green and blue accents.", AppearanceDark},
		// NvChad: wombat
		colors: map[Token]string{
			Foreground: "#d6d2c9", Background: "#202020",
			Title: "#d6d2c9", FileHeader: "#d6d2c9", Hunk: "#535353",
			Added: "#aee474", Removed: "#ff8f7e", Unavailable: "#ff8f7e",
			Metadata: "#535353", Warning: "#efdeab", Selection: "#373737",
			FocusedBorder: "#88b8f6", Border: "#353535",
			SyntaxFunction: "#88b8f6", SyntaxType: "#efdeab", SyntaxMacro: "#ffcc66",
			SyntaxConstant: "#dc8cff", SyntaxAttribute: "#efdeab", SyntaxBuiltin: "#dc8cff",
		}},
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
