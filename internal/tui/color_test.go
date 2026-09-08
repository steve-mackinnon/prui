package tui

import (
	"bytes"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/review"
)

// withoutPalette renders f with every semantic style removed, which reproduces
// the exact output the app produced before styling existed.
func withoutPalette(f func() string) string {
	styled := palette
	palette = map[lineClass]lipgloss.Style{}
	defer func() { palette = styled }()
	return f()
}

func styledModel(t *testing.T, s *review.Session) *Model {
	t.Helper()
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return s, nil })
	m.Update(m.Init()())
	if m.Session == nil || m.Err != nil {
		t.Fatal("fixture session did not load", m.Err)
	}
	return m
}

// downsample writes text through the color profile writer Bubble Tea installs
// for the detected terminal. The profile is supplied rather than detected, so
// the fallback is deterministic.
func downsample(text string, p colorprofile.Profile) string {
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: p}
	_, _ = w.WriteString(text)
	return buf.String()
}

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;:]*m`)

// colorlessSequences reports any escape sequence that is not a bare reset or a
// non-color attribute. Bold and reverse carry the selection cues, so they are
// expected to survive; colorless profiles must not emit foreground,
// background, or underline colors.
var colorlessAttributes = []string{"\x1b[m", "\x1b[1m", "\x1b[7m", "\x1b[1;7m"}

func colorlessSequences(s string) []string {
	leaked := []string{}
	for _, seq := range sgrPattern.FindAllString(s, -1) {
		if !slices.Contains(colorlessAttributes, seq) {
			leaked = append(leaked, seq)
		}
	}
	if rest := sgrPattern.ReplaceAllString(s, ""); strings.ContainsAny(rest, "\x1b\a") {
		leaked = append(leaked, "non-SGR control byte")
	}
	return leaked
}

func TestColorProfileFallbackPreservesContent(t *testing.T) {
	s := kindsSession()
	m := styledModel(t, s)
	profiles := []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI, colorprofile.Ascii, colorprofile.NoTTY}
	for _, width := range []int{60, 100, 120} {
		for i := range s.Inventory.Units {
			for _, focus := range []pane{paneList, paneDiff} {
				m.Selected, m.Focus = i, focus
				m.Update(tea.WindowSizeMsg{Width: width, Height: 14})
				colored := m.View().Content
				unstyled := withoutPalette(func() string { return m.View().Content })
				for _, p := range profiles {
					got := downsample(colored, p)
					if ansi.Strip(got) != unstyled {
						t.Fatalf("profile %s width %d unit %d focus %v changed content:\n%q\n%q", p, width, i, focus, ansi.Strip(got), unstyled)
					}
					switch p {
					case colorprofile.NoTTY:
						// No terminal: styles are removed outright, so the view
						// is byte-identical to the pre-styling render.
						if got != unstyled {
							t.Fatalf("NoTTY width %d unit %d focus %v not byte-identical:\n%q\n%q", width, i, focus, got, unstyled)
						}
					case colorprofile.Ascii:
						// NO_COLOR and colorless terminals: the writer rewrites
						// each style, keeping only non-color attributes.
						if leaked := colorlessSequences(got); len(leaked) > 0 {
							t.Fatalf("Ascii width %d unit %d focus %v kept color: %q in %q", width, i, focus, leaked, got)
						}
					default:
						for _, line := range strings.Split(got, "\n") {
							if visibleWidth(line) > width {
								t.Fatalf("profile %s exceeded width %d: %q", p, width, line)
							}
						}
					}
				}
			}
		}
	}
}

func TestColorProfileKeepsStylesWhereSupported(t *testing.T) {
	s := kindsSession()
	m := styledModel(t, s)
	m.Focus = paneDiff
	for i, u := range s.Inventory.Units {
		m.Selected = i
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 14})
		colored := m.View().Content
		for _, p := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI} {
			if !strings.Contains(downsample(colored, p), "\x1b[") {
				t.Fatalf("%s lost all styling at profile %s", u.Kind, p)
			}
		}
		if strings.Contains(downsample(colored, colorprofile.NoTTY), "\x1b") {
			t.Fatalf("%s emitted escapes with no terminal", u.Kind)
		}
	}
}

// TestColorEnvironmentContract pins the environment variables that must turn
// color off without a CLI option. Bubble Tea detects the same profile at start.
func TestColorEnvironmentContract(t *testing.T) {
	for _, c := range []struct {
		env  []string
		want colorprofile.Profile
	}{
		{[]string{"TERM=xterm-256color", "NO_COLOR=1"}, colorprofile.Ascii},
		{[]string{"TERM=dumb"}, colorprofile.NoTTY},
		{[]string{"TERM=dumb", "COLORTERM=truecolor"}, colorprofile.NoTTY},
		{nil, colorprofile.NoTTY},
		{[]string{"TERM=xterm"}, colorprofile.ANSI},
		{[]string{"TERM=xterm-256color"}, colorprofile.ANSI256},
		{[]string{"TERM=xterm-256color", "COLORTERM=truecolor"}, colorprofile.TrueColor},
		{[]string{"CLICOLOR_FORCE=1"}, colorprofile.ANSI},
	} {
		if got := colorprofile.Env(c.env); got != c.want {
			t.Fatalf("env %v detected %s, want %s", c.env, got, c.want)
		}
	}
	// A non-terminal destination never gets color, whatever TERM claims.
	if got := colorprofile.Detect(&bytes.Buffer{}, []string{"TERM=xterm-256color", "COLORTERM=truecolor"}); got != colorprofile.NoTTY {
		t.Fatalf("redirected output detected %s, want NoTTY", got)
	}
}

// TestPlainContractGuard pins the plain-mode guarantees this feature must not
// break: plain output never consults the palette and never emits terminal bytes.
func TestPlainContractGuard(t *testing.T) {
	s := kindsSession()
	styled := Plain(s)
	if unstyled := withoutPalette(func() string { return Plain(s) }); styled != unstyled {
		t.Fatalf("plain output depends on the palette:\n%q\n%q", styled, unstyled)
	}
	if strings.ContainsAny(styled, "\x1b\a") {
		t.Fatalf("plain output carries terminal control bytes: %q", styled)
	}
	if !strings.Contains(styled, "UNAVAILABLE: blob exceeds per-blob limit") || !strings.Contains(styled, "WARNING: one blob unavailable") {
		t.Fatal("plain output lost a documented state label")
	}
}

// TestEscapeContractUnchanged pins that styling did not relax escaping: every
// hostile byte stays a reversible backslash sequence.
func TestEscapeContractUnchanged(t *testing.T) {
	for _, raw := range []string{"\x1b]52;c;attack\a", "\x1b[32m+fake", "tab\there", "back\\slash", string([]byte{0xff, 0xfe}), "\x00\r\n"} {
		e := Escape(raw)
		if strings.ContainsAny(e, "\x1b\a\x00\r\n") {
			t.Fatalf("escape leaked a control byte: %q", e)
		}
		got, err := strconv.Unquote(`"` + e + `"`)
		if err != nil || got != raw {
			t.Fatalf("escape is not reversible for %q: %q (%v)", raw, e, err)
		}
		if styled := styleLine(classContext, e); ansi.Strip(styled) != e {
			t.Fatalf("styling altered escaped text: %q", styled)
		}
	}
}
