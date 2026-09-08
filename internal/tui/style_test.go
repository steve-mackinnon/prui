package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestClassifyPatchGrammar(t *testing.T) {
	for _, c := range []struct {
		line string
		want lineClass
	}{
		{"diff --git a/a b/a", classFileHeader},
		{"index e69de29..0cfbf08 100644", classFileHeader},
		{"--- a/a", classFileHeader},
		{"+++ b/a", classFileHeader},
		{"--- /dev/null", classFileHeader},
		{"new file mode 100644", classFileHeader},
		{"deleted file mode 100755", classFileHeader},
		{"old mode 100644", classFileHeader},
		{"new mode 100755", classFileHeader},
		{"similarity index 90%", classFileHeader},
		{"rename from a", classFileHeader},
		{"rename to b", classFileHeader},
		{"copy from a", classFileHeader},
		{"copy to b", classFileHeader},
		{"@@ -1,2 +1,3 @@ func main()", classHunk},
		{"+added", classAdded},
		{"+", classAdded},
		{"-removed", classRemoved},
		{"-", classRemoved},
		{" context", classContext},
		{"", classContext},
		{`\\ No newline at end of file`, classContext},
	} {
		if got := classifyPatch(c.line); got != c.want {
			t.Fatalf("classify %q = %d, want %d", c.line, got, c.want)
		}
	}
}

func TestClassifyEscapedHostileBytesStayContext(t *testing.T) {
	// Control bytes and invalid UTF-8 become backslash sequences, so a hostile
	// line can never claim a structural class it did not earn.
	for _, raw := range []string{"\x1b]52;c;attack\a", "\x1b[32m+fake addition", "\a-fake removal", "\x00@@ fake hunk", string([]byte{0xff, 0xfe})} {
		e := Escape(raw)
		if strings.ContainsAny(e, "\x1b\a\x00") {
			t.Fatalf("escape leaked control bytes: %q", e)
		}
		if got := classifyPatch(e); got != classContext {
			t.Fatalf("hostile line %q reclassified as %d", e, got)
		}
	}
}

func TestStyleLinePreservesTextAndWidth(t *testing.T) {
	for _, c := range []lineClass{classPlain, classTitle, classFileHeader, classHunk, classAdded, classRemoved, classContext} {
		const line = "+abc def"
		styled := styleLine(c, line)
		if ansi.Strip(styled) != line {
			t.Fatalf("class %d altered text: %q", c, styled)
		}
		if visibleWidth(styled) != visibleWidth(line) {
			t.Fatalf("class %d changed visible width: %q", c, styled)
		}
		if styleLine(c, "") != "" {
			t.Fatalf("class %d styled an empty line", c)
		}
	}
	if styleLine(classPlain, "x") != "x" || styleLine(classContext, "x") != "x" {
		t.Fatal("unstyled classes emitted escape sequences")
	}
	if !strings.Contains(styleLine(classAdded, "+x"), "\x1b[") {
		t.Fatal("added lines carry no style")
	}
}

func TestClipIsANSIAware(t *testing.T) {
	styled := styleLine(classAdded, "+abcdefgh")
	for _, w := range []int{0, 1, 3, 5, 9, 20} {
		got := clip(styled, w)
		if visibleWidth(got) > w {
			t.Fatalf("clip(%d) exceeded width: %q", w, got)
		}
		if plain := ansi.Strip(got); !strings.HasPrefix("+abcdefgh", plain) {
			t.Fatalf("clip(%d) corrupted text: %q", w, plain)
		}
		if strings.Count(got, "\x1b") == 1 {
			t.Fatalf("clip(%d) split an escape sequence: %q", w, got)
		}
	}
	if clip(styled, 20) != styled {
		t.Fatal("clip altered a line that already fits")
	}
}
