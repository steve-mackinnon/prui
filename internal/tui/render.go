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
	analysis := "file fallback"
	if s.AnalysisPlan != nil {
		analysis = s.AnalysisPlan.AnalysisStatus
	}
	return fmt.Sprintf("PR #%d | head %.12s | inventory %s | analysis: %s", m.Identity.Number, m.HeadSHA, completeness, analysis)
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
	if s.ID != "" {
		b.WriteString("Session: " + s.ID + " | plan: " + Escape(s.PlanVersion) + "\n" + progress(s) + "\n")
		b.WriteString("Optional full source context not retained; frozen patches and metadata available.\n")
	}
	b.WriteString(fmt.Sprintf("Evidence: %d retained, %d omitted; examined paths: %d; budgets files=%d excerpts=%d bytes=%d\n", len(s.Context.Evidence), len(s.Context.OmittedPaths), len(s.Context.ExaminedPaths), s.Context.FileBudget, s.Context.ExcerptBudget, s.Context.ByteBudget))
	for _, budget := range s.Context.ExhaustedBudgets {
		b.WriteString("SCOPE WARNING: " + Escape(budget) + " budget exhausted\n")
	}
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

func (m *Model) evidenceView() string {
	c := m.Session.Context
	lines := []string{status(m.Session), fmt.Sprintf("Evidence scope | retained %d | omitted %d | examined paths %d", len(c.Evidence), len(c.OmittedPaths), len(c.ExaminedPaths)), fmt.Sprintf("budgets: files %d | excerpt %d bytes | retained %d bytes", c.FileBudget, c.ExcerptBudget, c.ByteBudget)}
	for _, b := range c.ExhaustedBudgets {
		lines = append(lines, "WARNING budget exhausted: "+Escape(b))
	}
	for _, e := range c.Evidence {
		lines = append(lines, fmt.Sprintf("[observed] %s @ %.12s %s:%d-%d (%s)", e.Kind, e.CommitSHA, Escape(string(e.Path)), e.LineStart, e.LineEnd, Escape(e.RetrievalReason)))
	}
	for _, o := range c.OmittedPaths {
		lines = append(lines, fmt.Sprintf("[omitted] %s: %s", Escape(string(o.Path)), Escape(o.Reason)))
	}
	lines = append(lines, "e: return | raw inventory remains available | omissions are not missing diff entries")
	return strings.Join(lines, "\n")
}

func (m *Model) analysisView() string {
	s := m.Session
	if s.AnalysisPlan == nil {
		return status(s) + "\nNo accepted provider plan. Raw file slices remain the fallback.\na: return | analysis requires separate disclosure and consent"
	}
	p := s.AnalysisPlan
	lines := []string{status(s), fmt.Sprintf("Plan %s | %d slices | %d unassigned", Escape(p.Version), len(p.Slices), len(p.UnassignedUnitIDs))}
	for _, w := range p.Warnings {
		lines = append(lines, "WARNING: "+Escape(w))
	}
	for _, sl := range p.Slices {
		lines = append(lines, "[ ] "+Escape(sl.Title)+" ("+fmt.Sprintf("%d units", len(sl.UnitIDs))+")")
		if sl.OrderingRationale != "" {
			lines = append(lines, "  order: "+Escape(sl.OrderingRationale))
		}
		for _, c := range sl.Claims {
			label := "cited"
			if c.Hypothesis {
				label = "HYPOTHESIS"
			}
			lines = append(lines, "  "+label+": "+Escape(c.Text)+" ["+strings.Join(c.EvidenceID, ", ")+"]")
		}
	}
	if len(p.UnassignedUnitIDs) > 0 {
		lines = append(lines, "Unassigned units: "+strings.Join(p.UnassignedUnitIDs, ", "))
	}
	lines = append(lines, "a: return | i: full inventory | e: evidence scope | claims are advisory")
	return strings.Join(lines, "\n")
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
