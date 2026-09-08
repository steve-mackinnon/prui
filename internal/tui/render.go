package tui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
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
	if s.CurrentPlan() != nil {
		analysis = s.CurrentPlan().AnalysisStatus
	}
	line := fmt.Sprintf("PR #%d | head %.12s | inventory %s | analysis: %s", m.Identity.Number, m.HeadSHA, completeness, analysis)
	// The guide bundle is a separate claim from the plan's analysis status, so
	// it is reported separately and only when this session actually stored one.
	if s.Guides != nil {
		line += " | guides: " + guideStatus(s.Guides)
	}
	return line
}

// guideStatus states what the stored bundle claims; a session stored before
// guide analysis existed reports absence rather than a fallback decision it
// never made.
func guideStatus(b *guide.Bundle) string {
	if b == nil {
		return "none recorded for this session"
	}
	switch b.Status {
	case guide.Generated:
		origin := ""
		if b.Provider != "" || b.Model != "" {
			origin = Escape(b.Provider) + "/" + Escape(b.Model) + ", "
		}
		return fmt.Sprintf("generated (%s%d guides)", origin, len(b.Items))
	case guide.Unavailable:
		return "unavailable (" + Escape(b.Reason) + ")"
	}
	return "unrecognized status " + Escape(string(b.Status))
}

// statusClass reads the status line as a warning state when the inventory is
// incomplete. The wording is identical either way.
func statusClass(s *review.Session) lineClass {
	if !s.Inventory.Complete {
		return classWarning
	}
	return classTitle
}

// body splits an already-assembled card body into classified display lines.
func body(c lineClass, text string) []styledLine {
	lines := []styledLine{}
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		lines = append(lines, styledLine{c, l})
	}
	return lines
}

// unitLines is the classified form of unitText: the same escaped lines, each
// tagged with its semantic class. Classification never edits the text.
func unitLines(s *review.Session, i int) []styledLine {
	u := s.Inventory.Units[i]
	f := s.Inventory.Files[s.UnitFiles[i]]
	lines := []styledLine{{classTitle, fmt.Sprintf("%s [%s]", pathLabel(f), u.Kind)}}
	switch u.Kind {
	case inventory.TextHunk:
		// The patch keeps its trailing newline, so the final empty element is a real display line.
		for _, line := range strings.Split(string(s.Inventory.Patches[u.PatchReference]), "\n") {
			e := Escape(line)
			lines = append(lines, styledLine{classifyPatch(e), e})
		}
	case inventory.FileMetadata:
		lines = append(lines, body(cardClass(u.Kind), fmt.Sprintf("status %s | mode %s -> %s\nold path: %s\nnew path: %s\nold object: %s\nnew object: %s\n", f.Status, f.OldMode, f.NewMode, Escape(string(f.OldPath)), Escape(string(f.NewPath)), f.OldOID, f.NewOID))...)
	case inventory.Binary:
		lines = append(lines, body(cardClass(u.Kind), "Binary content changed; no text patch. Object IDs in file metadata.\n")...)
	case inventory.Gitlink:
		lines = append(lines, body(cardClass(u.Kind), fmt.Sprintf("Submodule pointer: %s -> %s\nSubmodule content not fetched or executed.\n", f.OldOID, f.NewOID))...)
	case inventory.Unavailable:
		lines = append(lines, body(cardClass(u.Kind), "UNAVAILABLE: "+Escape(u.UnavailableReason)+"\nNot fully reviewable; file remains in inventory.\n")...)
	}
	return lines
}
func unitText(s *review.Session, i int) string {
	var b strings.Builder
	for _, l := range unitLines(s, i) {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
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
	lines := []string{styleLine(statusClass(m.Session), status(m.Session)), styleLine(classTitle, fmt.Sprintf("Evidence scope | retained %d | omitted %d | examined paths %d", len(c.Evidence), len(c.OmittedPaths), len(c.ExaminedPaths))), fmt.Sprintf("budgets: files %d | excerpt %d bytes | retained %d bytes", c.FileBudget, c.ExcerptBudget, c.ByteBudget)}
	for _, b := range c.ExhaustedBudgets {
		lines = append(lines, styleLine(classWarning, "WARNING budget exhausted: "+Escape(b)))
	}
	for _, e := range c.Evidence {
		lines = append(lines, fmt.Sprintf("[observed] %s @ %.12s %s:%d-%d (%s)", e.Kind, e.CommitSHA, Escape(string(e.Path)), e.LineStart, e.LineEnd, Escape(e.RetrievalReason)))
	}
	for _, o := range c.OmittedPaths {
		lines = append(lines, fmt.Sprintf("[omitted] %s: %s", Escape(string(o.Path)), Escape(o.Reason)))
	}
	lines = append(lines, "esc: back | raw inventory remains available | omissions are not missing diff entries")
	return strings.Join(lines, "\n")
}

func (m *Model) analysisView() string {
	s := m.Session
	if s.CurrentPlan() == nil {
		return styleLine(statusClass(s), status(s)) + "\nNo accepted provider plan. Raw file slices remain the fallback.\nesc: back | analysis requires separate disclosure and consent"
	}
	p := s.CurrentPlan()
	if p == nil {
		return styleLine(statusClass(s), status(s)) + "\nNo accepted provider plan. Raw file slices remain the fallback.\nesc: back | analysis requires separate disclosure and consent"
	}
	lines := []string{styleLine(statusClass(s), status(s)), styleLine(classTitle, fmt.Sprintf("Plan %s | %d slices | %d unassigned", Escape(p.Version), len(p.Slices), len(p.UnassignedUnitIDs)))}
	for _, w := range p.Warnings {
		lines = append(lines, styleLine(classWarning, "WARNING: "+Escape(w)))
	}
	for i, sl := range p.Slices {
		marker := "[ ] "
		if slices.Contains(s.ReviewedSliceIDs, sl.SliceID) {
			marker = "[x] "
		}
		lines = append(lines, marker+fmt.Sprintf("%d. ", i+1)+Escape(sl.Title)+" ("+fmt.Sprintf("%d units", len(sl.UnitIDs))+")")
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
	lines = append(lines, "esc: back | claims are advisory")
	return strings.Join(lines, "\n")
}

func (m *Model) editView() string {
	s := m.Session
	p := s.CurrentPlan()
	if p == nil {
		return "No editable plan available"
	}
	if m.top() == pageReorder {
		lines := []string{"Reorder slices (preview; enter confirms, esc cancels)"}
		for i, sl := range p.Slices {
			marker := "  "
			if i == m.EditIndex {
				marker = "> "
			}
			lines = append(lines, marker+fmt.Sprintf("%d. %s", i+1, Escape(sl.Title)))
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{"Move selected unit (preview; enter confirms, esc cancels)", "unit: " + Escape(s.Inventory.Units[m.Selected].ID)}
	for i, sl := range p.Slices {
		marker := "  "
		if i == m.EditIndex {
			marker = "> "
		}
		lines = append(lines, marker+"slice: "+Escape(sl.Title))
	}
	marker := "  "
	if m.EditIndex == len(p.Slices) {
		marker = "> "
	}
	lines = append(lines, marker+"unassigned")
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
	// ANSI-aware: escape sequences cost no width and are never cut in half.
	return ansi.Truncate(s, w, "")
}
