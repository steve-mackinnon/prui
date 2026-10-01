package tui

import (
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/inventory"
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

func TestGuideCopyStaysExpandedThroughoutActiveGuide(t *testing.T) {
	s := largeSession(3, 3)
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{
		{Title: "First", Description: "First overview", Sections: []guide.Section{
			{Title: "One", Description: "First context", UnitIDs: []string{s.Inventory.Units[0].ID}},
			{Title: "Two", Description: "Second context", UnitIDs: []string{s.Inventory.Units[1].ID}},
		}},
		{Title: "Next", Description: "Next overview", Sections: []guide.Section{
			{Title: "Three", Description: "Next context", UnitIDs: []string{s.Inventory.Units[2].ID}},
		}},
	}}
	rows := rowsFor(s, newExpansion())
	for selected, r := range rows {
		for _, focused := range []bool{true, false} {
			lines := guideList(s, rows, selected, 80, focused, 0)
			var copy []string
			for _, line := range lines {
				if line.row < 0 {
					copy = append(copy, strings.TrimSpace(line.text))
				}
			}
			want := "First overview|First context|Second context"
			if r.guide == 1 {
				want = "Next overview|Next context"
			}
			if got := strings.Join(copy, "|"); got != want {
				t.Fatalf("row %d, list focused %v: copy = %q, want %q", selected, focused, got, want)
			}
		}
	}
}
