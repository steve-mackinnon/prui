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
