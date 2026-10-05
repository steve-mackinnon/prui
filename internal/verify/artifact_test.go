package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactWriterWritesBoundedTranscriptAndDeterministicScreenFiles(t *testing.T) {
	dir := t.TempDir()
	w, err := NewArtifactWriter(dir)
	if err != nil {
		t.Fatal(err)
	}

	transcript, err := w.WriteTranscript([]byte("open\r\nmarked\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "terminal.txt" {
		t.Fatalf("transcript path = %q", transcript)
	}

	screen, err := w.WriteScreen("marked", "title\n<item>&\"\x1b[31m\n")
	if err != nil {
		t.Fatal(err)
	}
	if screen.Text != "screens/marked.txt" || screen.SVG != "screens/marked.svg" {
		t.Fatalf("screen paths = %#v", screen)
	}

	text, err := os.ReadFile(filepath.Join(dir, screen.Text))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(text), "title\n<item>&\"\\x1b[31m\n"; got != want {
		t.Fatalf("screen text = %q, want %q", got, want)
	}

	svg, err := os.ReadFile(filepath.Join(dir, screen.SVG))
	if err != nil {
		t.Fatal(err)
	}
	got := string(svg)
	for _, want := range []string{
		`width="960" height="432" viewBox="0 0 960 432"`,
		`&lt;item&gt;&amp;&#34;\x1b[31m`,
		`font-family="monospace"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("SVG missing %q:\n%s", want, got)
		}
	}
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("SVG contains a terminal control byte: %q", got)
	}

	before := string(svg)
	second, err := w.WriteScreen("initial", "title\n<item>&\"\x1b[31m\n")
	if err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(dir, second.SVG))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != before {
		t.Fatal("identical screens produced different SVG output")
	}
}

func TestArtifactWriterRejectsUnsafeNamesAndOversizedContent(t *testing.T) {
	w, err := NewArtifactWriter(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside", "nested/name", "/absolute", ""} {
		t.Run(name, func(t *testing.T) {
			if _, err := w.WriteScreen(name, "screen"); err == nil {
				t.Fatalf("accepted unsafe screen name %q", name)
			}
		})
	}
	if _, err := w.WriteTranscript(make([]byte, MaxTranscriptBytes+1)); err == nil {
		t.Fatal("accepted oversized transcript")
	}
	if _, err := w.WriteScreen("initial", strings.Repeat("x", MaxScreenInputBytes+1)); err == nil {
		t.Fatal("accepted oversized screen")
	}
}
