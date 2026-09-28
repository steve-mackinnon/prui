package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/theme"
)

func TestDescriptionMarkdownNormalizesCarriageReturns(t *testing.T) {
	lines, err := renderDescriptionMarkdown("# Summary\r\n\r\nFirst paragraph.\rSecond paragraph.", 72)
	if err != nil {
		t.Fatalf("render description markdown: %v", err)
	}

	output := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Contains(output, `\r`) || strings.ContainsRune(output, '\r') {
		t.Fatalf("rendered carriage-return artifact:\n%q", output)
	}
	for _, want := range []string{"Summary", "First paragraph.", "Second paragraph."} {
		if !strings.Contains(output, want) {
			t.Fatalf("rendered description missing %q:\n%s", want, output)
		}
	}
}

func TestDescriptionMarkdownRendersGFMAndKeepsHTMLLiteral(t *testing.T) {
	body := "# Heading\n\n- [x] done\n- **bold** and [link](https://example.com)\n\n| Name | Value |\n| --- | --- |\n| one | two |\n\n<em>literal HTML</em>"
	lines, err := renderDescriptionMarkdown(body, 72)
	if err != nil {
		t.Fatalf("render description markdown: %v", err)
	}

	output := ansi.Strip(strings.Join(lines, "\n"))
	for _, want := range []string{"Heading", "done", "bold", "link", "Name", "Value", "one", "two", "<em>literal HTML</em>"} {
		if !strings.Contains(output, want) {
			t.Fatalf("rendered description missing %q:\n%s", want, output)
		}
	}
}

func TestDescriptionMarkdownNeutralizesTerminalControls(t *testing.T) {
	lines, err := renderDescriptionMarkdown("safe\x1b]52;c;hostile\a\nnext\x7f", 72)
	if err != nil {
		t.Fatalf("render description markdown: %v", err)
	}

	plain := ansi.Strip(strings.Join(lines, "\n"))
	if strings.ContainsRune(plain, '\x1b') || strings.ContainsRune(plain, '\a') || strings.ContainsRune(plain, '\x7f') {
		t.Fatalf("rendered output retained untrusted control bytes: %q", plain)
	}
	for _, want := range []string{`\x1b`, `\x07`, `\x7f`} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered output did not visibly neutralize %q:\n%s", want, plain)
		}
	}
}

func TestDescriptionMarkdownNeutralizesDecodedControls(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		forbidden        rune
	}{
		{"decimal entity", "safe&#7;hostile", `\x07`, '\a'},
		{"hex entity in code", "`safe&#x1b;hostile`", `\x1b`, '\x1b'},
		{"named entity", "safe&lrm;hostile", `\u200e`, '\u200e'},
		{"named newline", "safe&NewLine;hostile", `\x0a`, 0},
		{"HTML numeric C1 replacement", "safe&#x9b;2Jhostile", "safe›2Jhostile", '\u009b'},
		{"nested named ampersand", "safe&amp;#7;hostile", "&#7;", '\a'},
		{"nested decimal ampersand", "safe&#38;#7;hostile", "&#7;", '\a'},
		{"nested hex ampersand", "safe&#x26;#x1b;hostile", "&#x1b;", 0},
		{"nested ampersand in code", "`safe&#38;#7;hostile`", "&#7;", '\a'},
		{"nested ampersand in link label", "[safe&amp;#7;hostile](https://example.com)", "&#7;", '\a'},
		{"nested ampersand in link destination", "[safe](https://example.com/?x=&#x26;#x1b;)", "&#x26;#x1b;", 0},
		{"semicolonless decimal stays literal", "safe&#7hostile", "&#7hostile", '\a'},
		{"link label", "[safe&#7;hostile](https://example.com)", `\x07`, '\a'},
		{"link destination", "[safe](https://example.com/?x=&#7;hostile)", `\x07`, '\a'},
		{"literal C1", "safe\u009b2Jhostile", `\x9b`, '\u009b'},
		{"literal bidi", "safe\u202ehostile\u202c", `\u202e`, '\u202e'},
		{"deprecated bidi", "safe\u206ahostile", `\u206a`, '\u206a'},
		{"deprecated bidi entity", "safe&#x206a;hostile", `\u206a`, '\u206a'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lines, err := renderDescriptionMarkdown(tc.body, 100)
			if err != nil {
				t.Fatal(err)
			}
			out := ansi.Strip(strings.Join(lines, "\n"))
			if tc.forbidden != 0 && strings.ContainsRune(out, tc.forbidden) {
				t.Fatalf("control U+%04X survived: %.120q", tc.forbidden, out)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("control was not visible as %q: %.120q", tc.want, out)
			}
		})
	}
}

func TestDescriptionMarkdownKeepsLiteralEntitiesInFencedCode(t *testing.T) {
	for _, body := range []string{
		"```text\n&#7; &lrm;\n```",
		"<div>\n```text\n&#7; &lrm;\n```\n</div>",
	} {
		lines, err := renderDescriptionMarkdown(body, 80)
		if err != nil {
			t.Fatal(err)
		}
		plain := ansi.Strip(strings.Join(lines, "\n"))
		if !strings.Contains(plain, "&#7; &lrm;") {
			t.Fatalf("fenced code entity text changed: %.200q", plain)
		}
		for _, forbidden := range []rune{'\a', '\u200e'} {
			if strings.ContainsRune(plain, forbidden) {
				t.Fatalf("fenced code contains U+%04X: %.200q", forbidden, plain)
			}
		}
	}
}

func TestDescriptionMarkdownKeepsRendererHyperlinks(t *testing.T) {
	lines, err := renderDescriptionMarkdown("[normal link](https://example.com/%07?x=1) &amp; <em>literal</em>", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	if !strings.Contains(out, "\x1b]8;") || !strings.Contains(out, "https://example.com/%07?x=1") {
		t.Fatalf("normal link lost Glamour hyperlink rendering: %.200q", out)
	}
	if strings.Contains(out, "\x1b]52") {
		t.Fatalf("unexpected OSC command: %.200q", out)
	}
	plain := ansi.Strip(out)
	for _, want := range []string{"normal link", " & ", "<em>literal</em>"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("normal Markdown lost %q: %.200q", want, plain)
		}
	}
}

func TestDescriptionMarkdownEscapesControlsInsideOSC8Target(t *testing.T) {
	lines, err := renderDescriptionMarkdown("[safe](https://example.com/?x=&#7;hostile)", 100)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	start := strings.Index(out, "\x1b]8;")
	if start < 0 {
		t.Fatalf("missing OSC 8 hyperlink: %.200q", out)
	}
	end := strings.IndexByte(out[start:], '\a')
	if end < 0 {
		t.Fatalf("unterminated OSC 8 hyperlink: %.200q", out)
	}
	target := out[start : start+end]
	if !strings.Contains(target, `\x07`) || strings.ContainsRune(target, '\a') || strings.ContainsRune(target, '\u009d') {
		t.Fatalf("unsafe OSC 8 target: %.200q", target)
	}
}

func TestDescriptionMarkdownUsesLightStyleOnlyForLightTheme(t *testing.T) {
	light, err := renderDescriptionMarkdownForTheme("# Heading", 72, theme.Light)
	if err != nil {
		t.Fatalf("render light description markdown: %v", err)
	}
	dark, err := renderDescriptionMarkdownForTheme("# Heading", 72, theme.Dark)
	if err != nil {
		t.Fatalf("render dark description markdown: %v", err)
	}
	terminal, err := renderDescriptionMarkdownForTheme("# Heading", 72, theme.Terminal)
	if err != nil {
		t.Fatalf("render terminal description markdown: %v", err)
	}

	lightOutput := strings.Join(light, "\n")
	darkOutput := strings.Join(dark, "\n")
	if ansi.Strip(lightOutput) != ansi.Strip(darkOutput) {
		t.Fatalf("theme selection changed description text:\nlight=%q\ndark=%q", ansi.Strip(lightOutput), ansi.Strip(darkOutput))
	}
	if lightOutput == darkOutput {
		t.Fatal("light theme did not select Glamour's light style")
	}
	if strings.Join(terminal, "\n") != darkOutput {
		t.Fatal("non-light theme did not select Glamour's dark style")
	}
}
