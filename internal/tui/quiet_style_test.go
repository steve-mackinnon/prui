package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/theme"
)

// Personal color overrides must not erase keyboard focus, even when every
// configured color is the terminal default or NO_COLOR removes the palette.
func TestQuietWorkspaceFocusSurvivesDefaultOverrides(t *testing.T) {
	overrides := make(map[theme.Token]string)
	for _, token := range theme.Tokens() {
		overrides[token] = "default"
	}
	for _, name := range theme.BuiltInNames() {
		t.Run(name, func(t *testing.T) {
			palette, err := theme.Resolve(name, overrides)
			if err != nil {
				t.Fatal(err)
			}
			m := New(context.Background(), nil)
			m.SetTheme(palette)
			const text = "› [x] main.go"
			idle := downsample(m.styleLine(classSelection, text), colorprofile.Ascii)
			focused := downsample(m.styleLine(classSelectionFocused, text), colorprofile.Ascii)
			if idle == focused || !strings.Contains(focused, "\x1b[1;7m") {
				t.Fatalf("focus lost bold/reverse: idle=%q focused=%q", idle, focused)
			}
			for _, rendered := range []string{idle, focused} {
				if ansi.Strip(rendered) != text || visibleWidth(rendered) != visibleWidth(text) {
					t.Fatalf("selection changed its text or width: %q", rendered)
				}
				if leaked := colorlessSequences(rendered); len(leaked) > 0 {
					t.Fatalf("colorless selection leaked colors: %q", leaked)
				}
			}
			boundary := downsample(m.styleLine(classPaneBorderFocused, "│"), colorprofile.Ascii)
			if !strings.Contains(boundary, "\x1b[1m") || ansi.Strip(boundary) != "│" {
				t.Fatalf("focused boundary lost its independent bold cue: %q", boundary)
			}
			header := downsample(m.styleLine(classPaneHeaderFocused, "┌ Diff ─┐"), colorprofile.Ascii)
			if !strings.Contains(header, "\x1b[1m") || ansi.Strip(header) != "┌ Diff ─┐" {
				t.Fatalf("focused pane header lost its bold cue: %q", header)
			}
		})
	}
}
