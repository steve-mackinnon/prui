package tui

import (
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestDiffMetricsCountHunksOnly(t *testing.T) {
	units := []inventory.ReviewUnit{{ID: "a", FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}, {ID: "b", FileChangeID: "f", Kind: inventory.TextHunk, PatchReference: "p"}}
	patches := map[string][]byte{"p": []byte("diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,2 +1,3 @@\n context\n---source\n+++source\n+new\n\\ No newline at end of file\n")}
	stats := diffMetrics(units, patches)
	if got := stats["f"].String(); got != "+4/-2" {
		t.Fatalf("counts = %s", got)
	}
	units = append(units, inventory.ReviewUnit{FileChangeID: "f", Kind: inventory.Unavailable})
	if got := diffMetrics(units, patches)["f"].String(); got != "+4/-2 (partial)" {
		t.Fatalf("partial = %s", got)
	}
}

func TestDiffMetricsFilesAndCommits(t *testing.T) {
	m := largeModel(largeTextSession(2, 4), 120, 20)
	list, _ := m.reviewListPresentation()
	if !strings.Contains(list[0].text, "+") || !strings.Contains(list[0].text, "/-") {
		t.Fatalf("file row: %s", list[0].text)
	}
	if !strings.Contains(m.cachedFileDetail(false)[0].Text, "/-") {
		t.Fatal("missing file header counts")
	}
	c := commitModel(t)
	if !strings.Contains(c.commitRail()[1].text, "+30/-1") {
		t.Fatal("missing commit totals")
	}
	if got := ansi.Strip(c.View().Content); !strings.Contains(got, "changed.go +30/-1") {
		t.Fatalf("commit file header missing: %s", got)
	}
}

func TestDiffMetricsGuides(t *testing.T) {
	s := largeTextSession(1, 2)
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Change", Sections: []guide.Section{{Title: "First", UnitIDs: []string{s.Inventory.Units[0].ID}}, {Title: "Second", UnitIDs: []string{s.Inventory.Units[1].ID}}}}}}
	rows := rowsFor(s, newExpansion())
	lines := guideList(s, rows, 2, 60, true, 0)
	for _, line := range lines {
		if !strings.Contains(line.text, "/-") {
			t.Fatalf("missing guide/section/portion metrics: %s", line.text)
		}
	}
	if got := portionDiffStat(s, []int{0, 0}).String(); got != portionDiffStat(s, []int{0}).String() {
		t.Fatal("repeated guide reference double-counted")
	}
}

func TestDiffMetricsPRAndColors(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 20)
	m.colorProfile = colorprofile.TrueColor
	pr := source.PullRequest{Identity: source.Identity{Number: 42}, Title: "Change", Additions: 122, Deletions: 100, DiffStatsKnown: true}
	if got := ansi.Strip(m.prPickerRow(pr)); !strings.Contains(got, "+122/-100") {
		t.Fatalf("PR row: %s", got)
	}
	text := "file.go +122/-100"
	for _, class := range []lineClass{classPlain, classFileHeader, classSelection, classSelectionFocused, classMetadata} {
		got := m.styleLine(class, text)
		if ansi.Strip(got) != text || !strings.Contains(got, "\x1b[") || got == text {
			t.Fatalf("metrics not colored for %v: %q", class, got)
		}
	}
	if got := m.styleLine(classContext, text); got != text {
		t.Fatal("styled context source as metrics")
	}
	m.colorProfile = colorprofile.Ascii
	if got := m.styleDiffMetrics(text); got != text {
		t.Fatal("colorless metrics emitted styling")
	}
}
