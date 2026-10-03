// The palette values below adapt pinned upstream themes into app semantic roles.
// See docs/THEME-ATTRIBUTION.md for source revisions, credits, and license notices.
package theme

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

// Independent roles in each row are foreground, background, metadata, added,
// removed, warning, selection, focus, and border. Catalog order remains stable;
// the picker groups entries by Appearance without rewriting persisted IDs.
var additionalPresets = []preset{
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
