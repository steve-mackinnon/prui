package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"prui/internal/review"
	"strings"

	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/syntax"
)

type codeNavigation struct {
	mode        string // empty, expanded, OLD, NEW
	whitespace  bool
	source      *inventory.FullSource
	loading     bool
	sourceError string
}

func sourceRow(file inventory.FileChange, side string, n int, text string) diffLine {
	line := diffLine{styledLine: styledLine{classContext, " " + Escape(text)}, rawSource: text, searchID: searchSourceID{file.ID + ":" + side, n}}
	if side == "OLD" {
		line.oldLine = n
	} else {
		line.newLine = n
	}
	return line
}

// navigationDetail is a pure projection. Complete-source rows have coordinates
// but deliberately no comment targets; only canonical patch rows can compose.
func (m *Model) navigationDetail(file int) []diffLine {
	inv := m.navigationInventory()
	f := inv.Files[file]
	cache := m.sourceSyntaxCache()
	var result []diffLine
	if m.navigation.mode == "OLD" || m.navigation.mode == "NEW" {
		side := m.navigation.mode
		result = append(result, diffLine{styledLine: styledLine{classMetadata, "Full " + side + " · pinned source · read-only · Ctrl+D: diff"}})
		lines, ok := inv.SourceLines(file, side == "OLD")
		if !ok {
			return append(result, diffLine{styledLine: styledLine{classUnavailable, "UNAVAILABLE full source · not cached, binary, unsupported or capture limit · search coverage incomplete"}})
		}
		if len(lines) > 0 {
			oldStart, newStart := 0, 1
			if side == "OLD" {
				oldStart, newStart = 1, 0
			}
			result = append(result, diffLine{styledLine: styledLine{classHunk, fmt.Sprintf("@@ -%d,%d +%d,%d @@ full %s read-only", oldStart, len(lines), newStart, len(lines), side)}})
		}
		tokens := cache.tokens(m.ctx, inv, file, side == "OLD")
		for i, text := range lines {
			line := sourceRow(f, side, i+1, text)
			line.syntax = escapedSpans([]byte(text), tokens[i+1])
			line.Class = classAdded
			line.Text = "+" + Escape(text)
			if side == "OLD" {
				line.Class = classRemoved
				line.Text = "-" + Escape(text)
			}
			result = append(result, line)
		}
		if len(lines) == 0 {
			result = append(result, diffLine{styledLine: styledLine{classMetadata, "Empty or absent file · complete"}})
		}
		return result
	}
	old, oldOK := inv.SourceLines(file, true)
	newLines, newOK := inv.SourceLines(file, false)
	expanded := m.navigation.mode == "expanded" && oldOK && newOK
	if m.navigation.mode == "expanded" {
		label := "Expanded unchanged context · pinned source · added context read-only"
		if !expanded {
			label = "UNAVAILABLE expanded context · E: retry loading source · search coverage limited to patch"
			if m.navigation.loading {
				label = "Loading pinned full source…"
			} else if m.navigation.sourceError != "" {
				label += " · " + Escape(m.navigation.sourceError)
			}
		}
		result = append(result, diffLine{styledLine: styledLine{classMetadata, label}})
	}
	var oldTokens, newTokens syntax.Lines
	if expanded {
		oldTokens = cache.tokens(m.ctx, inv, file, true)
		newTokens = cache.tokens(m.ctx, inv, file, false)
	}
	o, n := 1, 1
	gap := func(endOld, endNew int) {
		if !expanded {
			return
		}
		// Only identical bounded regions are unchanged context. Never infer text.
		if endOld-o != endNew-n || endOld < o || endNew < n || endOld > len(old)+1 || endNew > len(newLines)+1 {
			result = append(result, diffLine{styledLine: styledLine{classUnavailable, "UNAVAILABLE context gap · captured coordinates do not match full source"}})
			return
		}
		for i := 0; i < endOld-o; i++ {
			if old[o+i-1] != newLines[n+i-1] {
				result = append(result, diffLine{styledLine: styledLine{classUnavailable, "UNAVAILABLE context gap · pinned source differs"}})
				return
			}
		}
		if endOld > o {
			result = append(result, diffLine{styledLine: styledLine{classHunk, fmt.Sprintf("@@ -%d,%d +%d,%d @@ expanded read-only", o, endOld-o, n, endNew-n)}})
		}
		for o < endOld {
			line := sourceRow(f, "NEW", n, newLines[n-1])
			line.oldLine = o
			line.oldSyntax = escapedSpans([]byte(old[o-1]), oldTokens[o])
			line.syntax = escapedSpans([]byte(newLines[n-1]), newTokens[n])
			result = append(result, line)
			o++
			n++
		}
	}
	for _, unit := range m.Session.Slices[file].Units {
		if inv.Units[unit].Kind == inventory.FileMetadata {
			continue
		}
		rows := unitLines(m.Session, unit)
		if expanded && inv.Units[unit].Kind == inventory.TextHunk {
			u := inv.Units[unit]
			oldStart, newStart := u.OldRange.Start, u.NewRange.Start
			if u.OldRange.Count == 0 {
				oldStart++
			}
			if u.NewRange.Count == 0 {
				newStart++
			}
			gap(max(1, oldStart), max(1, newStart))
			o = max(1, u.OldRange.Start) + u.OldRange.Count
			n = max(1, u.NewRange.Start) + u.NewRange.Count
			// Zero-count ranges are positioned after Start, unlike nonempty ranges.
			if u.OldRange.Count == 0 {
				o = u.OldRange.Start + 1
			}
			if u.NewRange.Count == 0 {
				n = u.NewRange.Start + 1
			}
		}
		result = append(result, rows...)
	}
	gap(len(old)+1, len(newLines)+1)
	if m.navigation.whitespace {
		result = hideWhitespacePairs(result)
	}
	return result
}

func hideWhitespacePairs(lines []diffLine) []diffLine {
	var out []diffLine
	for i := 0; i < len(lines); {
		start := i
		for i < len(lines) && lines[i].Class == classRemoved {
			i++
		}
		middle := i
		for i < len(lines) && lines[i].Class == classAdded {
			i++
		}
		end := i
		same := middle > start && middle-start == end-middle
		for j := 0; same && j < middle-start; j++ {
			same = strings.Join(strings.Fields(lines[start+j].rawSource), "") == strings.Join(strings.Fields(lines[middle+j].rawSource), "")
		}
		switch {
		case same:
			out = append(out, diffLine{styledLine: styledLine{classMetadata, fmt.Sprintf("Whitespace-only change hidden (%d canonical rows) · Ctrl+W: show", end-start)}})
			// Restore counters for subsequent split rows without inventing targets.
			last := lines[end-1]
			lastOld := lines[middle-1]
			out = append(out, diffLine{styledLine: styledLine{classHunk, fmt.Sprintf("@@ -%d,0 +%d,0 @@ canonical coordinates", lastOld.oldLine+1, last.newLine+1)}})
		case end > start:
			out = append(out, lines[start:end]...)
		default:
			out = append(out, lines[i])
			i++
		}
	}
	return out
}

func (m *Model) codeNavigationKey(key string) bool {
	if m.Session != nil && !m.Inventory && m.selectedReviewView() == viewGuide && !m.commitFilter.open && !m.commitFilter.subset && m.discussions.published == nil && m.discussions.editor == nil && (key == "E" || key == "ctrl+e" || key == "ctrl+d") {
		target, commentID := m.cursorAnchor()
		m.guideExpanded = (key == "E" || key == "ctrl+e") && (!m.guideExpanded || m.navigation.sourceError != "")
		m.guideCache = guideDetailCache{}
		m.fileCache = fileDetailCache{}
		m.closeSearchPopovers()
		m.restoreCursorAnchor(target, commentID)
		m.notice = "E: full file context · Ctrl+D: compact guide diff"
		return true
	}
	if m.Session == nil || !m.fileView() || m.selectedReviewView() != viewFiles || len(m.Session.Inventory.Files) == 0 || m.commitFilter.open || m.commitFilter.subset || m.discussions.published != nil || m.discussions.editor != nil {
		return false
	}
	switch key {
	case "E", "ctrl+e":
		if m.navigation.mode == "expanded" && m.navigation.sourceError == "" {
			m.navigation.mode = ""
		} else {
			m.navigation.mode = "expanded"
		}
	case "alt+o":
		m.navigation.mode = "OLD"
	case "alt+n":
		m.navigation.mode = "NEW"
	case "ctrl+d":
		m.navigation.mode = ""
	case "ctrl+w":
		m.navigation.whitespace = !m.navigation.whitespace
	default:
		return false
	}
	m.fileCache = fileDetailCache{}
	m.closeSearchPopovers()
	m.setOffset(m.fileOffset(m.Session.UnitFiles[m.Selected]))
	m.notice = "E: context · Alt+O/N: OLD/NEW · Ctrl+D: diff · Ctrl+W: whitespace · Alt+↑/↓: unresolved"
	return true
}

func (m *Model) nextUnresolved(delta int) {
	d := &m.discussions
	if !d.loaded {
		m.notice = "Unresolved threads unavailable · c: explicit online refresh"
		return
	}
	entries := m.discussionEntries()
	parentByEntry := map[string]string{}
	canonical := map[string]source.Discussion{}
	for _, thread := range d.snapshot.Snapshot.Threads {
		canonical[thread.ID] = thread
		parentByEntry[thread.ID] = thread.ID
		for _, comment := range thread.Comments {
			parentByEntry[fmt.Sprintf("inline:%d", comment.ID)] = thread.ID
		}
	}
	currentParent := ""
	if d.selected >= 0 && d.selected < len(entries) {
		currentParent = parentByEntry[entries[d.selected].ID]
	}
	candidates := []int{}
	seen := map[string]bool{}
	current := -1
	for i, entry := range entries {
		parent := parentByEntry[entry.ID]
		thread, ok := canonical[parent]
		if !ok || seen[parent] || thread.Retained || entry.Retained || thread.Resolved == nil || *thread.Resolved {
			continue
		}
		seen[parent] = true
		if parent == currentParent {
			current = len(candidates)
		}
		candidates = append(candidates, i)
	}
	if len(candidates) == 0 {
		m.notice = "No loaded authoritative unresolved threads · unknown/stale resolution excluded"
		if !d.snapshot.Snapshot.Complete {
			m.notice += " · partial coverage"
		}
		d.notice = m.notice
		return
	}
	chosen := 0
	switch {
	case current >= 0:
		chosen = (current + delta + len(candidates)) % len(candidates)
	case delta > 0:
		for j, i := range candidates {
			if i > d.selected {
				chosen = j
				break
			}
		}
	default:
		chosen = len(candidates) - 1
		for j := len(candidates) - 1; j >= 0; j-- {
			if candidates[j] < d.selected {
				chosen = j
				break
			}
		}
	}
	i := candidates[chosen]
	if m.top() != pageDiscussions {
		m.openDiscussions()
	}
	d.selected = i
	d.selectedID = entries[i].ID
	d.detail = true
	d.scroll = 0
	if !d.snapshot.Snapshot.Complete {
		d.notice = "Partial coverage · navigating loaded authoritative unresolved threads only"
	}
}

func (m *Model) navigableLine(line diffLine) bool {
	if line.target != nil || line.commentID > 0 {
		return true
	}
	if !m.guideExpanded && (!m.fileView() || m.navigation.mode == "") {
		return false
	}
	if line.searchID.Unit != "" {
		return true
	}
	if line.sideBySide != nil {
		for _, cell := range []*diffCell{line.sideBySide.old, line.sideBySide.new} {
			if cell != nil && cell.line != nil && cell.line.searchID.Unit != "" {
				return true
			}
		}
	}
	return false
}

// FullSourceLoader reads only the requested file's pinned blobs.
type FullSourceLoader func(context.Context, inventory.FileChange, source.PinnedComparison) (*inventory.FullSource, error)

func (m *Model) SetFullSourceLoader(load FullSourceLoader) { m.fullSourceLoader = load }

type fullSourceResult struct {
	target  int
	session *review.Session
	source  *inventory.FullSource
	err     error
}

func (m *Model) navigationInventory() inventory.Inventory {
	inv := m.Session.Inventory
	if m.navigation.source != nil {
		inv.FullSource = m.navigation.source
	}
	return inv
}
func (m *Model) loadNavigationSource() tea.Cmd {
	activeSource := m.selectedReviewView() == viewFiles && m.navigation.mode != "" || m.selectedReviewView() == viewGuide && m.guideExpanded
	if !activeSource || m.navigation.loading || m.Selected < 0 || m.Selected >= len(m.Session.UnitFiles) {
		return nil
	}
	file := m.Session.UnitFiles[m.Selected]
	inv := m.navigationInventory()
	_, oldOK := inv.SourceLines(file, true)
	_, newOK := inv.SourceLines(file, false)
	if oldOK && newOK {
		return nil
	}
	if m.fullSourceLoader == nil {
		m.navigation.sourceError = "source loading unavailable offline"
		return nil
	}
	m.guideCache = guideDetailCache{}
	m.navigation.loading = true
	m.navigation.sourceError = ""
	target := m.activeTab
	session, f, comparison, load, ctx := m.Session, inv.Files[file], inv.Comparison, m.fullSourceLoader, m.ctx
	return func() tea.Msg {
		data, err := load(ctx, f, comparison)
		return fullSourceResult{session: session, source: data, err: err, target: target}
	}
}
func (m *Model) applyFullSource(v fullSourceResult) {
	// A completion for a closed/replaced tab must never affect another comparison.
	state := m.reviewStateForTarget(v.target)
	if state == nil || state.Session != v.session {
		return
	}
	nav := &state.navigation
	nav.loading = false
	if v.err != nil {
		nav.sourceError = v.err.Error()
	} else if v.source != nil {
		inv := state.Session.Inventory
		if nav.source != nil {
			inv.FullSource = nav.source
		}
		merged := &inventory.FullSource{Blobs: map[string][]byte{}}
		if inv.FullSource != nil {
			for oid, b := range inv.FullSource.Blobs {
				merged.Blobs[oid] = b
			}
		}
		for oid, b := range v.source.Blobs {
			merged.Blobs[oid] = b
		}
		if merged.Valid(inv.Files) {
			nav.source = merged
		} else {
			nav.sourceError = "source cache limit reached"
		}
	}
	for _, search := range state.search {
		if search != nil {
			if search.cancel != nil {
				search.cancel()
			}
			search.generation++
			search.documents = nil
			search.matches = nil
			search.pending = false
		}
	}
	if v.target == m.activeTab {
		m.fileCache = fileDetailCache{}
		m.guideCache = guideDetailCache{}
		m.closeSearchPopovers()
	}
}
