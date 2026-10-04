package tui

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/source"
	"prui/internal/syntax"
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

// fileDivider makes file boundaries easy to scan without exposing Git object
// identifiers or extended-header bookkeeping in the interactive diff pane.
func fileDivider(f inventory.FileChange) string {
	return "── " + pathLabel(f)
}
func status(s *review.Session) string {
	completeness := "complete"
	if !s.Inventory.Complete {
		completeness = "INCOMPLETE - unavailable content"
	}
	m := s.Inventory.Comparison.Metadata
	line := fmt.Sprintf("PR #%d | head %.12s | inventory %s", m.Identity.Number, m.HeadSHA, completeness)
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
func body(c lineClass, text string) []diffLine {
	lines := []diffLine{}
	for _, l := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		lines = append(lines, diffLine{styledLine: styledLine{c, l}})
	}
	return lines
}

// unitLines is the classified form of unitText: the same escaped lines, each
// tagged with its semantic class. Classification never edits the text.
func unitLines(s *review.Session, i int) []diffLine {
	u := s.Inventory.Units[i]
	f := s.Inventory.Files[s.UnitFiles[i]]
	title := fmt.Sprintf("%s [%s]", pathLabel(f), u.Kind)
	lines := []diffLine{{styledLine: styledLine{classTitle, title}}}
	switch u.Kind {
	case inventory.TextHunk:
		metadata := s.Inventory.Comparison.Metadata
		return textHunkLines(f, u, s.Inventory.Patches[u.PatchReference], metadata.Identity, metadata.HeadSHA, s.Inventory.Syntax[u.ID])
	case inventory.FileMetadata:
		return []diffLine{{styledLine: styledLine{classFileHeader, fileDivider(f)}}}
	case inventory.Binary:
		lines = append(lines, body(cardClass(u.Kind), "Binary content changed; no text patch. Object IDs in file metadata.\n")...)
	case inventory.Gitlink:
		lines = append(lines, body(cardClass(u.Kind), fmt.Sprintf("Submodule pointer: %s -> %s\nSubmodule content not fetched or executed.\n", f.OldOID, f.NewOID))...)
	case inventory.Unavailable:
		lines = append(lines, body(cardClass(u.Kind), "UNAVAILABLE: "+Escape(u.UnavailableReason)+"\nNot fully reviewable; file remains in inventory.\n")...)
	}
	return lines
}

// hunkStarts extracts the two line counters from an unescaped unified-diff
// hunk header. Inventory creates one unit per hunk, but recognizing later
// headers keeps this pure renderer correct for all stored patch grammar.
func hunkStarts(raw []byte) (old, new int, ok bool) {
	if !bytes.HasPrefix(raw, []byte("@@ -")) {
		return 0, 0, false
	}
	old, rest, ok := diffRangeStart(raw[len("@@ -"):])
	if !ok || !bytes.HasPrefix(rest, []byte(" +")) {
		return 0, 0, false
	}
	new, rest, ok = diffRangeStart(rest[2:])
	if !ok || !bytes.HasPrefix(rest, []byte(" @@")) {
		return 0, 0, false
	}
	return old, new, true
}

func diffRangeStart(raw []byte) (start int, rest []byte, ok bool) {
	i := 0
	for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
		start = start*10 + int(raw[i]-'0')
		i++
	}
	if i == 0 {
		return 0, nil, false
	}
	if i < len(raw) && raw[i] == ',' {
		i++
		countStart := i
		for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
			i++
		}
		if i == countStart {
			return 0, nil, false
		}
	}
	return start, raw[i:], true
}

// diffCell is one side of an aligned diff row. line keeps the escaped source
// text and immutable review target separate from its display line number.
type diffCell struct {
	line   *diffLine
	number int
}

// diffRow is either one aligned old/new source row or one full-width
// structural row. It is presentation data only and never changes unitLines.
type diffRow struct {
	old, new *diffCell
	full     *diffLine
}

// projectSideBySideRows aligns a text hunk's already escaped diff lines. A
// deletion run pairs only with immediately following additions; all other
// structural lines stay full width.
func projectSideBySideRows(lines []diffLine) []diffRow {
	rows := make([]diffRow, 0, len(lines))
	oldNumber, newNumber := 0, 0
	deletions := make([]diffLine, 0)
	cell := func(sourceLine diffLine, number int) *diffCell {
		return &diffCell{line: &sourceLine, number: number}
	}
	flushDeletions := func() {
		for _, line := range deletions {
			rows = append(rows, diffRow{old: cell(line, oldNumber)})
			oldNumber++
		}
		deletions = deletions[:0]
	}

	for _, line := range lines {
		if old, new, ok := hunkStarts([]byte(line.Text)); ok {
			flushDeletions()
			oldNumber, newNumber = old, new
			line := line
			rows = append(rows, diffRow{full: &line})
			continue
		}
		if strings.HasPrefix(line.Text, `\ No newline at end of file`) {
			flushDeletions()
			line := line
			rows = append(rows, diffRow{full: &line})
			continue
		}

		switch {
		case line.Class == classRemoved && strings.HasPrefix(line.Text, "-"):
			deletions = append(deletions, line)
		case line.Class == classAdded && strings.HasPrefix(line.Text, "+"):
			for len(deletions) > 0 {
				old := deletions[0]
				deletions = deletions[1:]
				rows = append(rows, diffRow{old: cell(old, oldNumber), new: cell(line, newNumber)})
				oldNumber++
				newNumber++
				goto next
			}
			rows = append(rows, diffRow{new: cell(line, newNumber)})
			newNumber++
		case line.Class == classContext && strings.HasPrefix(line.Text, " "):
			flushDeletions()
			oldLine := line
			oldLine.target = nil
			oldLine.syntax = oldLine.oldSyntax
			rows = append(rows, diffRow{old: cell(oldLine, oldNumber), new: cell(line, newNumber)})
			oldNumber++
			newNumber++
		default:
			flushDeletions()
			line := line
			rows = append(rows, diffRow{full: &line})
		}
	next:
	}
	flushDeletions()
	return rows
}

const sideBySideMinimumWidth = 160

// projectSideBySideDetail turns source lines into logical split rows. Model
// uses these rows before it attaches comment overlays so guide anchors and
// focused-row navigation keep the same coordinate system as rendering.
func projectSideBySideDetail(lines []diffLine) []diffLine {
	rows := projectSideBySideRows(lines)
	projected := make([]diffLine, 0, len(rows))
	for _, row := range rows {
		line := diffLine{sideBySide: &row, target: rowTarget(row)}
		if row.full != nil {
			line.styledLine = row.full.styledLine
			line.target = row.full.target
		}
		projected = append(projected, line)
	}
	return projected
}

// renderProjectedSideBySideDetail renders logical rows assembled by Model so
// cursor and overlays share the same row indices as the visible split view.
func (m *Model) renderProjectedSideBySideDetail(lines []diffLine, width, horizontal int) []diffLine {
	var selected *source.ReviewCommentTarget
	selectedRow := -1
	if m.cursorActive {
		selected = m.selectedDiffTarget()
		selectedRow = m.cursor()
	}
	return m.renderSideBySideViewport(lines, width, horizontal, selectedRow, selected)
}

// selectedRow is relative to the supplied viewport, not the full diff.
func (m *Model) renderSideBySideViewport(lines []diffLine, width, horizontal, selectedRow int, selected *source.ReviewCommentTarget) []diffLine {
	rendered := make([]diffLine, 0, len(lines))
	oldBounds, _ := splitCellBounds(width)
	cellWidth := oldBounds.Dx()
	for index, line := range lines {
		if line.sideBySide == nil {
			rendered = append(rendered, diffLine{styledLine: styledLine{
				Class: line.Class,
				Text:  m.styleLine(line.Class, clip(line.Text, width)),
			}, target: line.target, commentID: line.commentID})
			continue
		}
		row := *line.sideBySide
		if row.full != nil {
			rendered = append(rendered, diffLine{styledLine: styledLine{
				Class: classPlain,
				Text:  m.styleLine(row.full.Class, clip(row.full.Text, width)),
			}, target: line.target, commentID: line.commentID})
			continue
		}
		renderCell := func(cell *diffCell) string {
			if !m.cursorActive {
				return m.renderSideBySideCell(cell, cellWidth, horizontal)
			}
			active := index == selectedRow && selected != nil && cell != nil && cell.line != nil && cell.line.target != nil && *cell.line.target == *selected
			return cursorMarker(active) + m.renderSideBySideCell(cell, max(0, cellWidth-2), horizontal)
		}
		old := renderCell(row.old)
		new := renderCell(row.new)
		rendered = append(rendered, diffLine{styledLine: styledLine{Class: classPlain, Text: old + " │ " + new}, target: line.target, commentID: line.commentID, sideBySide: line.sideBySide})
	}
	return rendered
}

func (m *Model) renderSideBySideCell(cell *diffCell, width, horizontal int) string {
	if width <= 0 {
		return ""
	}
	if cell == nil || cell.line == nil {
		return strings.Repeat(" ", width)
	}
	marker, text := splitPatchMarker(cell.line.Text)
	gutter := fmt.Sprintf("%5d %s ", cell.number, marker)
	source := *cell.line
	source.Text = text
	source.sourceOffset += len(marker)
	source.syntax = cropSpans(source.syntax, len(marker), len(cell.line.Text), 0)
	source.wordChanges = cropSpans(source.wordChanges, len(marker), len(cell.line.Text), 0)
	value := m.syntaxText(source, horizontal, width, gutter)
	value += strings.Repeat(" ", max(0, width-visibleWidth(value)))
	return m.styleLine(cell.line.Class, value)
}

func splitPatchMarker(text string) (marker, source string) {
	if text == "" {
		return " ", ""
	}
	runes := []rune(text)
	switch runes[0] {
	case '+', '-', ' ':
		return string(runes[0]), string(runes[1:])
	default:
		return " ", text
	}
}

// textHunkLines walks immutable patch bytes once. Coordinates and anchors stay
// separate from escaped source text so each view can choose its presentation.
func textHunkLines(f inventory.FileChange, u inventory.ReviewUnit, patch []byte, identity source.Identity, sha string, highlighting ...syntax.Patch) []diffLine {
	var tokens syntax.Patch
	if len(highlighting) > 0 {
		tokens = highlighting[0]
	}
	if tokens == nil {
		tokens = fragmentSyntax(f, patch)
	}
	var lines []diffLine
	old, new, inHunk := u.OldRange.Start, u.NewRange.Start, false
	for index, raw := range bytes.Split(patch, []byte{'\n'}) {
		if isGitFilePreamble(raw) {
			continue
		}
		text := Escape(string(raw))
		row := diffLine{styledLine: styledLine{classifyPatch(text), text}}
		if o, n, ok := hunkStarts(raw); ok {
			old, new, inHunk = o, n, true
		} else if inHunk && len(raw) > 0 {
			target := func(path []byte, side string, line int) *source.ReviewCommentTarget {
				return &source.ReviewCommentTarget{Identity: identity, CommitID: sha, Path: string(path), Side: side, Line: line}
			}
			switch raw[0] {
			case '+':
				row.newLine = new
				row.target = target(f.NewPath, "RIGHT", new)
				new++
			case '-':
				row.oldLine = old
				row.target = target(f.OldPath, "LEFT", old)
				old++
			case ' ':
				row.oldLine, row.newLine = old, new
				row.oldTarget = target(f.OldPath, "LEFT", old)
				row.target = target(f.NewPath, "RIGHT", new)
				old++
				new++
			}
		}
		if len(raw) > 0 && (row.oldLine > 0 || row.newLine > 0) {
			row.searchID = searchSourceID{u.ID, index}
			row.rawSource = string(raw[1:])
			row.oldSyntax = escapedSpans(raw[1:], tokens[index].Old)
			row.syntax = escapedSpans(raw[1:], tokens[index].New)
			if row.newLine == 0 {
				row.syntax = row.oldSyntax
			}
		}
		lines = append(lines, row)
	}
	attachWordChanges(lines, len(patch))
	return lines
}

// numberedPatchText adds commit-view columns without changing cached source.
func numberedPatchText(row diffLine) string {
	switch {
	case row.oldLine != 0 && row.newLine != 0:
		return fmt.Sprintf("%4d %4d %s", row.oldLine, row.newLine, row.Text)
	case row.oldLine != 0:
		return fmt.Sprintf("%4d      %s", row.oldLine, row.Text)
	case row.newLine != 0:
		return fmt.Sprintf("     %4d %s", row.newLine, row.Text)
	default:
		return row.Text
	}
}

func unitText(s *review.Session, i int) string {
	u := s.Inventory.Units[i]
	f := s.Inventory.Files[s.UnitFiles[i]]
	// Plain output retains its diagnostic metadata contract. The interactive
	// renderer intentionally condenses it into fileDivider instead.
	if u.Kind == inventory.FileMetadata {
		return fmt.Sprintf("%s [%s]\nstatus %s | mode %s -> %s\nold path: %s\nnew path: %s\nold object: %s\nnew object: %s\n", pathLabel(f), u.Kind, f.Status, f.OldMode, f.NewMode, Escape(string(f.OldPath)), Escape(string(f.NewPath)), f.OldOID, f.NewOID)
	}
	var b strings.Builder
	if u.Kind == inventory.TextHunk {
		b.WriteString("── " + pathLabel(f) + " ──\n")
	}
	for _, l := range unitLines(s, i) {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

// guidesText renders the interpretation layer above the unchanged raw units.
// Every model-authored string is escaped like a patch line, and the scope line
// states what never reached the analyzer so guides cannot imply full coverage.
func guidesText(s *review.Session) string {
	g := s.Guides
	if g == nil || g.Status != guide.Generated || len(g.Items) == 0 {
		return ""
	}
	index := make(map[string]int, len(s.Inventory.Units))
	for i, u := range s.Inventory.Units {
		index[u.ID] = i
	}
	var b strings.Builder
	b.WriteString("\nGuides interpret the diff; file slices remain the unit of reading progress.\n")
	for i, item := range g.Items {
		title := fmt.Sprintf("%d. %s", i+1, Escape(item.Title))
		if item.Ungrouped {
			title += " (not grouped by analysis)"
		}
		b.WriteString(title + "\n")
		if strings.TrimSpace(item.Description) != "" && !item.Ungrouped {
			b.WriteString("   " + Escape(item.Description) + "\n")
		}
		for j, section := range item.Sections {
			if !item.Ungrouped {
				fmt.Fprintf(&b, "   %d.%d %s\n", i+1, j+1, Escape(section.Title))
				if strings.TrimSpace(section.Description) != "" {
					b.WriteString("       " + Escape(section.Description) + "\n")
				}
			}
			for _, portion := range portions(s, index, section.UnitIDs) {
				b.WriteString("       " + portion + "\n")
			}
		}
	}
	b.WriteString(scope(s, g) + "\n")
	return b.String()
}

// portions collapses a section's units into one line per file, in section
// order; the same file may appear under several sections.
func portions(s *review.Session, index map[string]int, ids []string) []string {
	var order []int
	kinds := map[int][]string{}
	for _, id := range ids {
		i, ok := index[id]
		if !ok {
			continue
		}
		f := s.UnitFiles[i]
		if len(kinds[f]) == 0 {
			order = append(order, f)
		}
		kinds[f] = append(kinds[f], string(s.Inventory.Units[i].Kind))
	}
	out := make([]string, 0, len(order))
	for _, f := range order {
		out = append(out, pathLabel(s.Inventory.Files[f])+" ["+strings.Join(kinds[f], ", ")+"]")
	}
	return out
}

func scope(s *review.Session, g *guide.Bundle) string {
	total := len(s.Inventory.Units)
	line := fmt.Sprintf("Analysis scope: %d/%d units sent", total-len(g.WithheldPaths), total)
	seen := map[string]bool{}
	var withheld []string
	for _, w := range g.WithheldPaths {
		entry := Escape(string(w.Path)) + " (" + Escape(w.Reason) + ")"
		if seen[entry] {
			continue
		}
		seen[entry] = true
		withheld = append(withheld, entry)
	}
	if len(withheld) > 0 {
		line += "; withheld: " + strings.Join(withheld, ", ")
	}
	return line
}

func Plain(s *review.Session) string {
	var b strings.Builder
	b.WriteString(status(s) + "\n" + s.Inventory.Comparison.Metadata.Identity.URL() + "\n")
	fmt.Fprintf(&b, "%d files; %d units. Reading is not GitHub approval.\n", len(s.Inventory.Files), len(s.Inventory.Units))
	if s.ID != "" {
		b.WriteString("Session: " + s.ID + "\n" + progress(s) + "\n")
		b.WriteString("Optional full source context not retained; frozen patches and metadata available.\n")
	}
	b.WriteString(guidesText(s))
	fmt.Fprintf(&b, "Evidence: %d retained, %d omitted; examined paths: %d; budgets files=%d excerpts=%d bytes=%d\n", len(s.Context.Evidence), len(s.Context.OmittedPaths), len(s.Context.ExaminedPaths), s.Context.FileBudget, s.Context.ExcerptBudget, s.Context.ByteBudget)
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
	lines := []string{m.styleLine(statusClass(m.Session), status(m.Session)), m.styleLine(classTitle, fmt.Sprintf("Evidence scope | retained %d | omitted %d | examined paths %d", len(c.Evidence), len(c.OmittedPaths), len(c.ExaminedPaths))), fmt.Sprintf("budgets: files %d | excerpt %d bytes | retained %d bytes", c.FileBudget, c.ExcerptBudget, c.ByteBudget)}
	for _, b := range c.ExhaustedBudgets {
		lines = append(lines, m.styleLine(classWarning, "WARNING budget exhausted: "+Escape(b)))
	}
	for _, e := range c.Evidence {
		lines = append(lines, fmt.Sprintf("[observed] %s @ %.12s %s:%d-%d (%s)", e.Kind, e.CommitSHA, Escape(string(e.Path)), e.LineStart, e.LineEnd, Escape(e.RetrievalReason)))
	}
	for _, o := range c.OmittedPaths {
		lines = append(lines, fmt.Sprintf("[omitted] %s: %s", Escape(string(o.Path)), Escape(o.Reason)))
	}
	if m.Selected >= 0 && m.Selected < len(m.Session.UnitFiles) {
		f := m.Session.Inventory.Files[m.Session.UnitFiles[m.Selected]]
		if classification, ok := m.Session.Inventory.Classifications[f.ID]; ok {
			state := "captured committed attributes"
			if classification.Partial {
				state = "classification unavailable; safe support default"
			}
			lines = append(lines, fmt.Sprintf("File category: %s · %s · tree %.12s", Escape(string(classification.Category)), state, Escape(classification.SourceSHA)))
		}
	}
	lines = append(lines, "esc: back | raw inventory remains available | omissions are not missing diff entries")
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
