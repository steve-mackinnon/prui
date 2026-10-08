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
	Foreground      Token = "foreground"
	Background      Token = "background"
	Title           Token = "title"
	FileHeader      Token = "fileHeader"
	Hunk            Token = "hunk"
	Added           Token = "added"
	Removed         Token = "removed"
	Metadata        Token = "metadata"
	Warning         Token = "warning"
	Unavailable     Token = "unavailable"
	Selection       Token = "selection"
	FocusedBorder   Token = "focusedBorder"
	Border          Token = "border"
	SyntaxFunction  Token = "syntaxFunction"
	SyntaxType      Token = "syntaxType"
	SyntaxMacro     Token = "syntaxMacro"
	SyntaxConstant  Token = "syntaxConstant"
	SyntaxAttribute Token = "syntaxAttribute"
	SyntaxBuiltin   Token = "syntaxBuiltin"
)

const (
	Terminal     = "terminal"
	Light        = "light"
	Dark         = "dark"
	HighContrast = "high-contrast"
)

var tokens = []Token{
	Foreground, Background,
	Title, FileHeader, Hunk, Added, Removed, Metadata, Warning, Unavailable,
	Selection, FocusedBorder, Border,
	SyntaxFunction, SyntaxType, SyntaxMacro, SyntaxConstant, SyntaxAttribute, SyntaxBuiltin,
}

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

// BuiltInNames returns the supported built-in theme names in stable catalog order.
func BuiltInNames() []string {
	names := make([]string, len(presets))
	for i, preset := range presets {
		names[i] = preset.Name
	}
	return names
}

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

// Resolve builds a complete palette from a named built-in and optional,
// already-keyed token overrides. Validation happens before a Theme is returned.
func Resolve(name string, overrides map[Token]string) (Theme, error) {
	var base map[Token]string
	var appearance Appearance
	for _, preset := range presets {
		if preset.Name == name {
			base = preset.colors
			appearance = preset.Appearance
			break
		}
	}
	if base == nil {
		return Theme{}, fmt.Errorf("unknown theme %q", name)
	}
	colors := make(map[Token]Color, len(tokens))
	symbols := syntaxPalette(base, appearance)
	for _, token := range tokens {
		raw := base[token]
		if raw == "" {
			raw = symbols[token]
		}
		parsed, err := ParseColor(raw)
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
