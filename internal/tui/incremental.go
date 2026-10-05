package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"prui/internal/commits"
	"prui/internal/incremental"
	"prui/internal/inventory"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
)

type incrementalViewState struct {
	file, offset, horizontal int
	drafts                   bool
	outcomes                 []string
}

func (m *Model) openIncremental() {
	m.incremental.outcomes = m.incrementalDraftOutcomes()
	m.push(pageIncremental)
}

// Only outcomes and anchor metadata are rendered here. Bodies and immutable
// attempts stay in their original private draft record, accessible with o.
func (m *Model) incrementalDraftOutcomes() []string {
	if m.Session.Incremental == nil {
		return []string{"No predecessor captured."}
	}
	b := m.Session.Incremental
	if m.store == nil {
		return []string{"Private draft storage unavailable; original work retained."}
	}
	old, err := m.store.Load(b.PreviousID)
	if err != nil || old.SnapshotReference != b.PreviousReference || !source.SamePinnedRevision(old.Inventory.Comparison.Metadata, b.Previous) {
		return []string{"Original snapshot unavailable; captured changes remain readable."}
	}
	d, err := m.store.LoadDraft(context.Background(), session.DraftKeyFor(b.Previous))
	if err != nil {
		return []string{"Private draft outcomes unavailable; original retained."}
	}
	lines := []string{"Original drafts remain private and unchanged. o: inspect original snapshot.", "Recreate a target explicitly in the new PR diff to submit; this view never writes."}
	add := func(label string, t source.ReviewCommentTarget, uncertain bool) {
		o := incremental.Assess(old.Inventory, m.Session.Inventory, t, uncertain)
		anchor := fmt.Sprintf("%s %s %d", t.Path, t.Side, t.Line)
		if t.SubjectType == "file" {
			anchor = t.Path + " (file)"
		} else if t.StartLine > 0 {
			anchor = fmt.Sprintf("%s %s %d-%d", t.Path, t.Side, t.StartLine, t.Line)
		}
		lines = append(lines, Escape(label+": "+anchor+" — "+string(o.Kind)))
	}
	for i, c := range d.Pending {
		add(fmt.Sprintf("Pending %d", i+1), c.Target, d.Attempt != "")
	}
	if d.Composer != nil {
		add("Editor", d.Composer.Target, d.Attempt != "")
	}
	if d.General != nil {
		lines = append(lines, "General PR draft: inspect original comparison; never copied or remapped.")
		if d.General.Uncertain || d.General.Matched {
			lines = append(lines, "General attempted request: check its immutable delivery evidence in the original comparison before any retry.")
		}
	}
	if d.Reply != nil {
		lines = append(lines, "Reply: inspect original thread; never remapped.")
	}
	if d.Summary != "" || d.Event != 0 {
		lines = append(lines, "Review decision/summary: original comparison only; compose explicitly in new review.")
	}
	if a := d.Attempted; a != nil {
		lines = append(lines, "Immutable attempted request: outcome must be checked in original snapshot before any retry.")
		if a.Comment != nil {
			add("Attempted "+a.Kind, a.Comment.Target, true)
		}
		if a.Review != nil {
			for i, c := range a.Review.Comments {
				add(fmt.Sprintf("Attempted review target %d", i+1), c.Target, true)
			}
		}
	}
	if d.Generation == 0 {
		lines = append(lines, "No saved drafts for the predecessor.")
	}
	return lines
}

func (m *Model) incrementalRows() []diffLine {
	b := m.Session.Incremental
	if b == nil {
		return body(classWarning, "No prior-head comparison captured. N: capture a new comparison from this session.")
	}
	if m.incremental.drafts {
		var out []diffLine
		for _, line := range m.incremental.outcomes {
			out = append(out, body(classContext, line)...)
		}
		return out
	}
	if b.Status != "captured" || b.Diff == nil {
		return body(classWarning, Escape(b.Reason))
	}
	if len(b.Diff.Files) == 0 {
		return body(classContext, "No tree changes between the captured heads. Base changes can still require PR review.")
	}
	m.incremental.file = max(0, min(m.incremental.file, len(b.Diff.Files)-1))
	f := b.Diff.Files[m.incremental.file]
	d := &commits.Diff{Files: []inventory.FileChange{f}, Patches: b.Diff.Patches, Syntax: b.Diff.Syntax, Complete: b.Diff.Complete}
	for _, u := range b.Diff.Units {
		if u.FileChangeID == f.ID {
			d.Units = append(d.Units, u)
		}
	}
	return commitDiffRows(d)
}

func (m *Model) incrementalView() string {
	header := "Changes since previous review (read-only)"
	meta := "5: available after capturing a new comparison"
	if b := m.Session.Incremental; b != nil {
		meta = shortCommitSHA(b.Previous.HeadSHA) + " -> " + shortCommitSHA(b.Current.HeadSHA) + " | " + Escape(b.Relation)
		if b.Status != "captured" {
			meta += " | UNAVAILABLE"
		}
		if b.Previous.BaseSHA != b.Current.BaseSHA {
			meta += " | base changed"
		}
		if b.Diff != nil {
			meta += fmt.Sprintf(" | file %d/%d", min(m.incremental.file+1, len(b.Diff.Files)), len(b.Diff.Files))
			if !b.Diff.Complete {
				meta += " | PARTIAL"
			}
		}
	}
	if m.incremental.drafts {
		header = "Draft anchor outcomes (private, read-only)"
	}
	footer := "n/p: file · ↑↓/PgUp/PgDn: scroll · ←→: pan · d: drafts/diff · o: old · esc: back"
	if m.ActionError != nil {
		footer = "! " + Escape(m.ActionError.Error()) + " · esc: back"
	}
	if m.Height <= 2 {
		if m.Height == 1 {
			return clip(footer, m.Width)
		}
		return clip(header, m.Width) + "\n" + clip(footer, m.Width)
	}
	rows := m.incrementalRows()
	height := max(1, m.Height-4)
	m.incremental.offset = max(0, min(m.incremental.offset, max(0, len(rows)-height)))
	lines := []string{clip(header, m.Width), clip(meta, m.Width)}
	for _, r := range rows[m.incremental.offset:min(len(rows), m.incremental.offset+height)] {
		lines = append(lines, m.syntaxText(r, m.incremental.horizontal, m.Width, ""))
	}
	for len(lines) < m.Height-1 {
		lines = append(lines, "")
	}
	return strings.Join(append(lines, clip(footer, m.Width)), "\n")
}

func (m *Model) incrementalKey(k string) tea.Cmd {
	s := &m.incremental
	switch k {
	case "esc":
		m.pop()
	case "d":
		s.drafts = !s.drafts
		s.offset = 0
		s.horizontal = 0
	case "n":
		s.file++
		s.offset = 0
	case "p":
		s.file = max(0, s.file-1)
		s.offset = 0
	case "j", "down":
		s.offset++
	case "k", "up":
		s.offset = max(0, s.offset-1)
	case "pgdown":
		s.offset += max(1, m.Height-4)
	case "pgup":
		s.offset = max(0, s.offset-max(1, m.Height-4))
	case "home":
		s.offset = 0
	case "end":
		s.offset = len(m.incrementalRows())
	case "right":
		s.horizontal += 5
	case "left":
		s.horizontal = max(0, s.horizontal-5)
	case "o":
		if m.store != nil && m.Session.Incremental != nil {
			b := m.Session.Incremental
			old, err := m.store.Load(b.PreviousID)
			if err != nil {
				m.ActionError = err
				return nil
			}
			if old.SnapshotReference != b.PreviousReference {
				return nil
			}
			return func() tea.Msg { return ActionResult{Session: old, Reset: true} }
		}
	}
	return nil
}
