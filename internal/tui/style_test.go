package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
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

func TestSelectionChromeKeepsMarkerAndFocusDistinctWithoutColor(t *testing.T) {
	if got := selectionMarker(false); got != "  " {
		t.Fatalf("unselected marker = %q, want blank gutter", got)
	}
	if got := selectionMarker(true); got != "› " {
		t.Fatalf("selected marker = %q, want persistent chevron", got)
	}
	if selectedClass(false) != classSelection {
		t.Fatal("unfocused selection did not use the shared selection style")
	}
	if selectedClass(true) != classSelectionFocused {
		t.Fatal("focused selection did not use the shared focused selection style")
	}
	for _, focused := range []bool{false, true} {
		styled := styleLine(selectedClass(focused), selectionMarker(true)+"main.go")
		if got := ansi.Strip(styled); got != "› main.go" {
			t.Fatalf("focused=%t selection changed visible text: %q", focused, got)
		}
	}
}

func TestCursorMarkerReservesVisibleDetailGutter(t *testing.T) {
	if got := cursorMarker(false); got != "  " {
		t.Fatalf("unselected cursor marker = %q, want blank gutter", got)
	}
	if got := cursorMarker(true); got != "› " {
		t.Fatalf("selected cursor marker = %q, want chevron", got)
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

// kindsSession builds every unit kind without Git, so card classification and
// wording can be asserted deterministically.
func kindsSession() *review.Session {
	patch := "diff --git a/text b/text\nindex 1111111..2222222 100644\n--- a/text\n+++ b/text\n@@ -1,2 +1,2 @@\n context\n-old\n+new\n"
	files := []inventory.FileChange{
		{ID: "f-text", OldPath: []byte("text"), NewPath: []byte("text"), OldOID: "1111111", NewOID: "2222222", OldMode: "100644", NewMode: "100644", Status: "M"},
		{ID: "f-binary", OldPath: []byte("image.png"), NewPath: []byte("image.png"), OldOID: "3333333", NewOID: "4444444", OldMode: "100644", NewMode: "100644", Status: "M"},
		{ID: "f-gitlink", OldPath: []byte("sub"), NewPath: []byte("sub"), OldOID: "5555555", NewOID: "6666666", OldMode: "160000", NewMode: "160000", Status: "M"},
		{ID: "f-unavailable", OldPath: []byte("huge"), NewPath: []byte("huge"), OldOID: "7777777", NewOID: "8888888", OldMode: "100644", NewMode: "100644", Status: "M"},
	}
	units := []inventory.ReviewUnit{
		{ID: "u-meta", FileChangeID: "f-text", Kind: inventory.FileMetadata},
		{ID: "u-text", FileChangeID: "f-text", Kind: inventory.TextHunk, PatchReference: "p"},
		{ID: "u-binary", FileChangeID: "f-binary", Kind: inventory.Binary},
		{ID: "u-gitlink", FileChangeID: "f-gitlink", Kind: inventory.Gitlink},
		{ID: "u-unavailable", FileChangeID: "f-unavailable", Kind: inventory.Unavailable, UnavailableReason: "blob exceeds per-blob limit"},
	}
	s := &review.Session{}
	s.Inventory = inventory.Inventory{
		Comparison: source.PinnedComparison{Metadata: source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 7}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: "aaaaaaaaaaaa", HeadSHA: "bbbbbbbbbbbb"}},
		Files:      files,
		Units:      units,
		Patches:    map[string][]byte{"p": []byte(patch)},
		Problems:   []string{"one blob unavailable"},
	}
	s.Slices = make([]review.Slice, len(files))
	s.UnitFiles = make([]int, len(units))
	index := map[string]int{}
	for i, f := range files {
		index[f.ID] = i
		s.Slices[i] = review.Slice{FileID: f.ID, Units: []int{}}
	}
	for i, u := range units {
		f := index[u.FileChangeID]
		s.Slices[f].Units = append(s.Slices[f].Units, i)
		s.UnitFiles[i] = f
	}
	return s
}

func TestUnitCardClassesPerKind(t *testing.T) {
	s := kindsSession()
	want := map[inventory.Kind]lineClass{
		inventory.FileMetadata: classMetadata,
		inventory.Binary:       classMetadata,
		inventory.Gitlink:      classMetadata,
		inventory.Unavailable:  classUnavailable,
	}
	for i, u := range s.Inventory.Units {
		lines := unitLines(s, i)
		if len(lines) < 2 || lines[0].Class != classTitle {
			t.Fatalf("unit %s missing styled title: %+v", u.Kind, lines)
		}
		if u.Kind == inventory.TextHunk {
			classes := map[lineClass]bool{}
			for _, l := range lines[1:] {
				classes[l.Class] = true
			}
			for _, c := range []lineClass{classFileHeader, classHunk, classAdded, classRemoved, classContext} {
				if !classes[c] {
					t.Fatalf("text hunk missing class %d", c)
				}
			}
			continue
		}
		for _, l := range lines[1:] {
			if l.Class != want[u.Kind] {
				t.Fatalf("%s body line %q classified %d, want %d", u.Kind, l.Text, l.Class, want[u.Kind])
			}
		}
	}
}

func TestUnitLinesWordingMatchesUnitText(t *testing.T) {
	s := kindsSession()
	for i, u := range s.Inventory.Units {
		var b strings.Builder
		for _, l := range unitLines(s, i) {
			if strings.Contains(l.Text, "\x1b") {
				t.Fatalf("%s classified line carries ANSI: %q", u.Kind, l.Text)
			}
			b.WriteString(styleLine(l.Class, l.Text) + "\n")
		}
		if got := ansi.Strip(b.String()); got != unitText(s, i) {
			t.Fatalf("%s styled wording changed:\n%q\n%q", u.Kind, got, unitText(s, i))
		}
	}
}

func TestUnitLinesPreserveReviewCommentTargets(t *testing.T) {
	sha := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	s := kindsSession()
	s.Inventory.Comparison.Metadata.HeadSHA = sha
	s.Inventory.Files[0] = inventory.FileChange{ID: "rename", OldPath: []byte("old-name.go"), NewPath: []byte("new-name.go")}
	s.Inventory.Units[1].OldRange = inventory.Range{Start: 4, Count: 3}
	s.Inventory.Units[1].NewRange = inventory.Range{Start: 10, Count: 4}
	s.Inventory.Patches["p"] = []byte("diff --git a/old-name.go b/new-name.go\n--- a/old-name.go\n+++ b/new-name.go\n@@ -4,3 +10,4 @@\n context\n-old\n+new\n+added\n context two\n\\ No newline at end of file\n")

	lines := unitLines(s, 1)
	want := map[string]struct {
		path string
		side string
		line int
	}{
		" context":     {"new-name.go", "RIGHT", 10},
		"-old":         {"old-name.go", "LEFT", 5},
		"+new":         {"new-name.go", "RIGHT", 11},
		"+added":       {"new-name.go", "RIGHT", 12},
		" context two": {"new-name.go", "RIGHT", 13},
	}
	for _, got := range lines {
		if expected, ok := want[got.Text]; ok {
			if got.target == nil {
				t.Fatalf("%q has no target", got.Text)
			}
			if got.target.Identity != s.Inventory.Comparison.Metadata.Identity || got.target.CommitID != sha || got.target.Path != expected.path || got.target.Side != expected.side || got.target.Line != expected.line {
				t.Fatalf("%q target = %#v, want path=%q side=%s line=%d", got.Text, got.target, expected.path, expected.side, expected.line)
			}
			delete(want, got.Text)
			continue
		}
		if got.target != nil {
			t.Fatalf("structural line %q unexpectedly commentable: %#v", got.Text, got.target)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing targets: %#v", want)
	}
}

func TestUnitLinesPreserveDeletionTargetAndInvalidPath(t *testing.T) {
	s := kindsSession()
	s.Inventory.Files[0] = inventory.FileChange{ID: "deleted", OldPath: []byte("deleted.go")}
	s.Inventory.Units[1].OldRange = inventory.Range{Start: 7, Count: 1}
	s.Inventory.Units[1].NewRange = inventory.Range{Start: 0, Count: 0}
	s.Inventory.Patches["p"] = []byte("@@ -7 +0,0 @@\n-gone\n")
	lines := unitLines(s, 1)
	if got := lines[2].target; got == nil || got.Path != "deleted.go" || got.Side != "LEFT" || got.Line != 7 {
		t.Fatalf("deleted line target = %#v", got)
	}

	s.Inventory.Files[0] = inventory.FileChange{ID: "invalid", NewPath: []byte("bad\xff")}
	s.Inventory.Units[1].OldRange = inventory.Range{Start: 0, Count: 0}
	s.Inventory.Units[1].NewRange = inventory.Range{Start: 2, Count: 1}
	s.Inventory.Patches["p"] = []byte("@@ -0,0 +2 @@\n+added\n")
	lines = unitLines(s, 1)
	if got := lines[2].target; got == nil || got.Path != string([]byte("bad\xff")) || got.Side != "RIGHT" || got.Line != 2 {
		t.Fatalf("invalid-path addition target = %#v", got)
	}
}

func TestUnitLinesResetTargetsAtLaterHunkHeaders(t *testing.T) {
	s := kindsSession()
	s.Inventory.Files[0] = inventory.FileChange{ID: "text", OldPath: []byte("text.go"), NewPath: []byte("text.go")}
	s.Inventory.Units[1].OldRange = inventory.Range{Start: 1, Count: 1}
	s.Inventory.Units[1].NewRange = inventory.Range{Start: 1, Count: 1}
	s.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+new\n@@ -9,2 +20,2 @@\n context\n-oldtwo\n+newtwo\n")

	want := map[string]struct {
		side string
		line int
	}{
		"-old":     {"LEFT", 1},
		"+new":     {"RIGHT", 1},
		" context": {"RIGHT", 20},
		"-oldtwo":  {"LEFT", 10},
		"+newtwo":  {"RIGHT", 21},
	}
	for _, line := range unitLines(s, 1) {
		expected, ok := want[line.Text]
		if !ok {
			continue
		}
		if line.target == nil || line.target.Side != expected.side || line.target.Line != expected.line {
			t.Fatalf("%q target = %#v, want side=%s line=%d", line.Text, line.target, expected.side, expected.line)
		}
	}
}

func TestChromeStatesStyledWithoutRewording(t *testing.T) {
	s := kindsSession()
	if statusClass(s) != classWarning {
		t.Fatal("incomplete inventory is not a warning state")
	}
	s.Inventory.Complete = true
	if statusClass(s) != classTitle {
		t.Fatal("complete inventory is not chrome")
	}
	if progressClass(s) != classTitle {
		t.Fatal("unchecked freshness is not chrome")
	}
	for _, state := range []session.RevisionStatus{session.Stale, session.CheckFailed} {
		s.RevisionStatus = state
		if progressClass(s) != classWarning {
			t.Fatalf("freshness %s is not a warning state", state)
		}
	}
	if cardClass(inventory.Unavailable) != classUnavailable || cardClass(inventory.Binary) != classMetadata {
		t.Fatal("card state classes lost")
	}
}
