package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTabStripNumbersEscapesAndMarksTheActiveTab(t *testing.T) {
	got := tabStrip(80, []string{"PRs", "octo/repo#42\x1b[31m\nnext"}, 1)
	want := "1 PRs  [2 octo/repo#42\\x1b[31m\\nnext]"
	if got != want {
		t.Fatalf("tab strip = %q, want %q", got, want)
	}
	if strings.ContainsAny(got, "\x1b\n\r\a") {
		t.Fatalf("tab strip leaked a terminal control byte: %q", got)
	}
}

func TestTabStripClipsToWidthWithoutANSI(t *testing.T) {
	for _, width := range []int{-1, 0, 1, 7, 15} {
		got := tabStrip(width, []string{"PRs", "owner/repository#123"}, 0)
		if visibleWidth(got) > max(width, 0) {
			t.Fatalf("width %d exceeded by %q", width, got)
		}
		if strings.Contains(got, "\x1b") || ansi.Strip(got) != got {
			t.Fatalf("width %d emitted ANSI: %q", width, got)
		}
	}
}

func TestTabStripLimitsLabelsToNumericHotkeys(t *testing.T) {
	labels := make([]string, 10)
	for i := range labels {
		labels[i] = "tab"
	}
	got := tabStrip(100, labels, 9)
	if strings.Contains(got, "10 tab") {
		t.Fatalf("tab strip rendered an unavailable numeric hotkey: %q", got)
	}
	if !strings.Contains(got, "9 tab") {
		t.Fatalf("tab strip omitted its final numeric hotkey: %q", got)
	}
}
