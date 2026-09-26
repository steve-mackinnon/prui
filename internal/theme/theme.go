// Package theme defines the bounded, presentation-only color vocabulary used
// by the interactive TUI. It deliberately accepts color values only through
// ParseColor so raw terminal control sequences can never reach rendering.
package theme

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"

	"charm.land/lipgloss/v2"
)

// Token identifies one stable semantic display role. Tokens do not control
// text, attributes, layout, or interaction behavior.
type Token string

const (
	Title         Token = "title"
	FileHeader    Token = "fileHeader"
	Hunk          Token = "hunk"
	Added         Token = "added"
	Removed       Token = "removed"
	Metadata      Token = "metadata"
	Warning       Token = "warning"
	Unavailable   Token = "unavailable"
	Selection     Token = "selection"
	FocusedBorder Token = "focusedBorder"
	Border        Token = "border"
)

const (
	Terminal     = "terminal"
	Light        = "light"
	Dark         = "dark"
	HighContrast = "high-contrast"
)

var tokens = []Token{
	Title, FileHeader, Hunk, Added, Removed, Metadata, Warning, Unavailable,
	Selection, FocusedBorder, Border,
}

var builtInNames = []string{Terminal, Light, Dark, HighContrast}

// Color is a validated color value. Its source syntax is retained for stable
// configuration encoding and compatibility tests; rendering consumes Value.
type Color struct {
	syntax string
	value  color.Color
}

// Syntax returns the documented, validated color spelling.
func (c Color) Syntax() string { return c.syntax }

// Value returns the color safe to pass to Lip Gloss.
func (c Color) Value() color.Color { return c.value }

// Theme is a resolved, complete palette. Its colors map is private so callers
// cannot mutate a built-in or another resolved theme.
type Theme struct {
	Name      string
	colors    map[Token]Color
	overrides map[Token]string
}

// Color returns a safe color for token. The bool is false only for a token
// outside the fixed vocabulary.
func (t Theme) Color(token Token) (color.Color, bool) {
	c, ok := t.colors[token]
	if !ok {
		return nil, false
	}
	return c.Value(), true
}

// Syntax returns the validated spelling for token, or an empty string if it
// is not part of the theme vocabulary.
func (t Theme) Syntax(token Token) string {
	return t.colors[token].Syntax()
}

// Overrides returns a copy of the validated token overrides used to resolve
// this palette. It preserves configuration provenance even when an override's
// color happens to match the selected built-in's current value.
func (t Theme) Overrides() map[Token]string {
	return cloneOverrides(t.overrides)
}

// Tokens returns the complete semantic vocabulary in display-independent
// deterministic order.
func Tokens() []Token { return append([]Token(nil), tokens...) }

// BuiltInNames returns the supported built-in theme names in picker order.
func BuiltInNames() []string { return append([]string(nil), builtInNames...) }

var ansiColors = map[string]color.Color{
	"black":          lipgloss.Black,
	"red":            lipgloss.Red,
	"green":          lipgloss.Green,
	"yellow":         lipgloss.Yellow,
	"blue":           lipgloss.Blue,
	"magenta":        lipgloss.Magenta,
	"cyan":           lipgloss.Cyan,
	"white":          lipgloss.White,
	"bright-black":   lipgloss.BrightBlack,
	"bright-red":     lipgloss.BrightRed,
	"bright-green":   lipgloss.BrightGreen,
	"bright-yellow":  lipgloss.BrightYellow,
	"bright-blue":    lipgloss.BrightBlue,
	"bright-magenta": lipgloss.BrightMagenta,
	"bright-cyan":    lipgloss.BrightCyan,
	"bright-white":   lipgloss.BrightWhite,
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var paletteIndex = regexp.MustCompile(`^(0|[1-9][0-9]{0,2})$`)

// ParseColor accepts only the documented color syntax: #RRGGBB, the fixed
// ANSI color names, default, or an ANSI-256 palette index from 0 through 255.
func ParseColor(s string) (Color, error) {
	if s == "default" {
		return Color{syntax: s, value: lipgloss.NoColor{}}, nil
	}
	if c, ok := ansiColors[s]; ok {
		return Color{syntax: s, value: c}, nil
	}
	if hexColor.MatchString(s) {
		return Color{syntax: s, value: lipgloss.Color(s)}, nil
	}
	if paletteIndex.MatchString(s) {
		i, _ := strconv.Atoi(s)
		if i <= 255 {
			return Color{syntax: s, value: lipgloss.Color(s)}, nil
		}
	}
	return Color{}, fmt.Errorf("invalid theme color")
}

var builtIns = map[string]map[Token]string{
	// Terminal inherits the user's foreground for routine chrome. Keep accents
	// and change colors in ANSI names so they follow the terminal palette too.
	Terminal: {
		Title: "default", FileHeader: "default", Hunk: "default", Added: "green",
		Removed: "red", Metadata: "default", Warning: "bright-yellow",
		Unavailable: "bright-red", Selection: "236", FocusedBorder: "cyan", Border: "240",
	},
	Light: {
		Title: "#24292f", FileHeader: "#24292f", Hunk: "#57606a", Added: "#116329",
		Removed: "#cf222e", Metadata: "#57606a", Warning: "#9a6700",
		Unavailable: "#cf222e", Selection: "#d0d7de", FocusedBorder: "#0969da", Border: "#8c959f",
	},
	Dark: {
		Title: "#c9d1d9", FileHeader: "#c9d1d9", Hunk: "#8b949e", Added: "#7ee787",
		Removed: "#ff7b72", Metadata: "#8b949e", Warning: "#e3b341",
		Unavailable: "#ff7b72", Selection: "#30363d", FocusedBorder: "#58a6ff", Border: "#6e7681",
	},
	HighContrast: {
		Title: "#00ffff", FileHeader: "#ffff00", Hunk: "#ff00ff", Added: "#00ff00",
		Removed: "#ff5555", Metadata: "#55aaff", Warning: "#ffff00",
		Unavailable: "#ff5555", Selection: "#555555", FocusedBorder: "#ffffff", Border: "#aaaaaa",
	},
}

// Resolve builds a complete palette from a named built-in and optional,
// already-keyed token overrides. Validation happens before a Theme is returned.
func Resolve(name string, overrides map[Token]string) (Theme, error) {
	base, ok := builtIns[name]
	if !ok {
		return Theme{}, fmt.Errorf("unknown theme %q", name)
	}
	colors := make(map[Token]Color, len(tokens))
	for _, token := range tokens {
		parsed, err := ParseColor(base[token])
		if err != nil {
			return Theme{}, fmt.Errorf("invalid built-in %q color for %q: %w", name, token, err)
		}
		colors[token] = parsed
	}
	for token, raw := range overrides {
		if !isToken(token) {
			return Theme{}, fmt.Errorf("unknown theme token %q", token)
		}
		parsed, err := ParseColor(raw)
		if err != nil {
			return Theme{}, fmt.Errorf("invalid color for theme token %q: %w", token, err)
		}
		colors[token] = parsed
	}
	return Theme{Name: name, colors: colors, overrides: cloneOverrides(overrides)}, nil
}

func cloneOverrides(overrides map[Token]string) map[Token]string {
	if len(overrides) == 0 {
		return nil
	}
	cloned := make(map[Token]string, len(overrides))
	for token, syntax := range overrides {
		cloned[token] = syntax
	}
	return cloned
}

func isToken(token Token) bool {
	for _, known := range tokens {
		if token == known {
			return true
		}
	}
	return false
}
