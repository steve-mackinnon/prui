package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
)

func Escape(s string) string { q := strconv.Quote(s); return q[1 : len(q)-1] }
func pathLabel(f inventory.FileChange) string {
	if len(f.OldPath) == 0 {
		return Escape(string(f.NewPath))
	}
	if len(f.NewPath) == 0 {
		return Escape(string(f.OldPath))
	}
	if string(f.OldPath) != string(f.NewPath) {
		return Escape(string(f.OldPath)) + " -> " + Escape(string(f.NewPath))
	}
	return Escape(string(f.NewPath))
}
func status(s *review.Session) string {
	completeness := "complete"
	if !s.Inventory.Complete {
		completeness = "INCOMPLETE - unavailable content"
	}
	m := s.Inventory.Comparison.Metadata
	return fmt.Sprintf("PR #%d | head %.12s | inventory %s | analysis: file fallback", m.Identity.Number, m.HeadSHA, completeness)
}
func unitText(s *review.Session, i int) string {
	u := s.Inventory.Units[i]
	f := s.Inventory.Files[s.UnitFiles[i]]
	title := fmt.Sprintf("%s [%s]\n", pathLabel(f), u.Kind)
	switch u.Kind {
	case inventory.TextHunk:
		var b strings.Builder
		b.WriteString(title)
		for _, line := range strings.Split(string(s.Inventory.Patches[u.PatchReference]), "\n") {
			b.WriteString(Escape(line))
			b.WriteByte('\n')
		}
		return b.String()
	case inventory.FileMetadata:
		return title + fmt.Sprintf("status %s | mode %s -> %s\nold path: %s\nnew path: %s\nold object: %s\nnew object: %s\n", f.Status, f.OldMode, f.NewMode, Escape(string(f.OldPath)), Escape(string(f.NewPath)), f.OldOID, f.NewOID)
	case inventory.Binary:
		return title + "Binary content changed; no text patch. Object IDs in file metadata.\n"
	case inventory.Gitlink:
		return title + fmt.Sprintf("Submodule pointer: %s -> %s\nSubmodule content not fetched or executed.\n", f.OldOID, f.NewOID)
	case inventory.Unavailable:
		return title + "UNAVAILABLE: " + Escape(u.UnavailableReason) + "\nNot fully reviewable; file remains in inventory.\n"
	}
	return title
}
func Plain(s *review.Session) string {
	var b strings.Builder
	b.WriteString(status(s) + "\n" + s.Inventory.Comparison.Metadata.Identity.URL() + "\n")
	b.WriteString(fmt.Sprintf("%d files; %d units. Reading is not GitHub approval.\n", len(s.Inventory.Files), len(s.Inventory.Units)))
	if len(s.Inventory.Files) == 0 {
		b.WriteString("Empty comparison: no net tree changes.\n")
	}
	for _, p := range s.Inventory.Problems {
		b.WriteString("WARNING: " + Escape(p) + "\n")
	}
	for i := range s.Inventory.Units {
		b.WriteString("\n" + unitText(s, i))
	}
	return b.String()
}
func visibleWidth(s string) int { return lipgloss.Width(s) }
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if visibleWidth(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := visibleWidth(string(r))
		if used+rw > w {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}
