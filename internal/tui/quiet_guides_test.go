package tui

import (
	"strings"
	"testing"

	"pr-review/internal/inventory"
)

func TestQuietGuidePortionsRetainExceptionalKindsAndCoverage(t *testing.T) {
	s := kindsSession()
	for i, unit := range s.Inventory.Units {
		file := s.UnitFiles[i]
		r := row{file: file, units: []int{i}}
		want := pathLabel(s.Inventory.Files[file])
		if unit.Kind != inventory.TextHunk {
			want += " [" + string(unit.Kind) + "]"
		}
		if got := portionLabel(s, file, r.units); got != want {
			t.Fatalf("%s portion label = %q, want %q", unit.Kind, got, want)
		}
		if got := guidePortionText(s, r, "› ", 100, false, 0); got != "› "+want {
			t.Fatalf("%s portion row = %q, want %q", unit.Kind, got, "› "+want)
		}
	}
	if got := portionLabel(s, 0, []int{0, 1}); got != "text [2 units]" {
		t.Fatalf("multi-unit coverage lost: %q", got)
	}
}

func TestQuietFileDividerPreservesEscapedRenamePaths(t *testing.T) {
	f := inventory.FileChange{OldPath: []byte("old\tname.go"), NewPath: []byte("new\x1bname.go")}
	got := fileDivider(f)
	if got != `── old\tname.go -> new\x1bname.go` || strings.ContainsAny(got, "\t\x1b") {
		t.Fatalf("file divider lost or unescaped a rename path: %q", got)
	}
}
