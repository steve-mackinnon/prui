package tui

import (
	"charm.land/lipgloss/v2"
	"strings"
	"testing"
)

func TestCommitFileHeaderHighlight(t *testing.T) {
	m := commitModel(t)
	m.commitSelection()
	m.commitOffset()
	m.styles = map[lineClass]lipgloss.Style{classSelection: lipgloss.NewStyle().Transform(func(s string) string { return "ACTIVE:" + s })}
	for _, offset := range []int{0, 3} {
		m.commit.offsets[m.commit.selectedSHA] = offset
		if got := m.commitsView(); !strings.Contains(got, "ACTIVE:── changed.go") {
			t.Fatalf("offset %d: missing active filename: %s", offset, got)
		}
	}
}

func TestDiffViewportPreservesSourceAndSplitHeaders(t *testing.T) {
	header := diffLine{styledLine: styledLine{Class: classFileHeader, Text: "── second.go"}}
	header.sideBySide = &diffRow{full: &diffLine{styledLine: header.styledLine}}
	source := []diffLine{{styledLine: styledLine{Class: classFileHeader, Text: "── first.go"}}, {styledLine: styledLine{Class: classContext, Text: " first"}}, header, {styledLine: styledLine{Class: classAdded, Text: "+second"}}, {styledLine: styledLine{Class: classContext, Text: " end"}}}
	rows, sticky := diffViewport(source, 3, 2, 3, true)
	if sticky != 2 || rows[0].Class != classSelection || rows[0].sideBySide.full.Class != classSelection || rows[1].Text != " end" {
		t.Fatalf("unexpected viewport: %#v, sticky %d", rows, sticky)
	}
	if source[2].Class != classFileHeader || source[2].sideBySide.full.Class != classFileHeader {
		t.Fatal("viewport mutated source")
	}
	rows, sticky = diffViewport(source, 3, 1, 3, true)
	if sticky != -1 || rows[0].Text != "+second" {
		t.Fatal("sticky header occluded single-row viewport")
	}
	rows, sticky = diffViewport(source, 0, 5, 3, true)
	if sticky != -1 || rows[0].Class != classFileHeader || rows[2].Class != classSelection {
		t.Fatal("highlight did not follow active file")
	}
}

func TestDiffViewportWithoutPinningPreservesCursorAndEditorRows(t *testing.T) {
	source := []diffLine{{styledLine: styledLine{Class: classFileHeader, Text: "── file.go"}}, {styledLine: styledLine{Class: classPlain, Text: "draft"}, editor: true}}
	rows, sticky := diffViewport(source, 1, 2, 1, false)
	if sticky != -1 || len(rows) != 1 || !rows[0].editor {
		t.Fatal("viewport hid editor")
	}
}
