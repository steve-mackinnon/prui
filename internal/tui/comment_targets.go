package tui

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"prui/internal/commits"
	"prui/internal/source"
)

// Endpoint projection is display-only: stored and submitted targets keep all
// coordinates. Rendered source rows themselves are always single-line anchors.
func targetEndsAt(target, row source.ReviewCommentTarget) bool {
	target.StartLine, target.StartSide = 0, ""
	return target == row
}
func commentTargetLabel(t source.ReviewCommentTarget) string {
	if t.SubjectType == "file" {
		return Escape(t.Path) + " · file"
	}
	if t.StartLine > 0 {
		return fmt.Sprintf("%s start %d → end %d · %s", t.Side, t.StartLine, t.Line, Escape(t.Path))
	}
	return fmt.Sprintf("%s:%d %s", Escape(t.Path), t.Line, t.Side)
}
func (m *Model) commentSelectionTarget() *source.ReviewCommentTarget {
	if m.selectedReviewView() == viewCommits {
		if m.commit.focus != paneDiff {
			return nil
		}
		rows := m.commitRows()
		cursor := m.commitCursor()
		if cursor < 0 || cursor >= len(rows) {
			return nil
		}
		return rows[cursor].target
	}
	if m.Focus != paneDiff {
		return nil
	}
	return m.selectedDiffTarget()
}
func (m *Model) toggleCommentRange() {
	if m.rangeStart != nil {
		m.rangeStart = nil
		return
	}
	t := m.commentSelectionTarget()
	if t == nil {
		m.ActionError = errors.New("range start requires a captured diff line")
		return
	}
	copy := *t
	m.rangeStart = &copy
	m.ActionError = nil
}
func (m *Model) completeCommentRange(end source.ReviewCommentTarget) (source.ReviewCommentTarget, error) {
	if m.rangeStart == nil {
		return end, nil
	}
	start := *m.rangeStart
	if start.Identity != end.Identity || start.CommitID != end.CommitID || start.Path != end.Path || start.Side != end.Side {
		return end, errors.New("range endpoints must share the pinned file and diff side")
	}
	if start.Line == end.Line {
		return end, nil
	}
	end.StartLine = min(start.Line, end.Line)
	end.Line = max(start.Line, end.Line)
	end.StartSide = end.Side
	if m.Session == nil {
		return end, errors.New("range source unavailable")
	}
	inv := m.Session.Inventory
	valid := commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, end)
	if m.selectedReviewView() == viewCommits {
		valid = commits.ContainsTarget(m.Session.Commits, end)
	}
	if !valid {
		return end, errors.New("range must contain contiguous captured coordinates in one hunk")
	}
	return end, nil
}
func (m *Model) openFileComposer() tea.Cmd {
	if m.Session == nil || m.Composer != nil {
		return nil
	}
	if m.selectedReviewView() == viewCommits {
		m.ActionError = errors.New("historical file comments are unsupported; select the PR Files view")
		return nil
	}
	if !m.diffReviewView() {
		return nil
	}
	inv := m.Session.Inventory
	if m.Selected < 0 || m.Selected >= len(m.Session.UnitFiles) {
		m.ActionError = errors.New("file target unavailable")
		return nil
	}
	file := m.Session.UnitFiles[m.Selected]

	if file < 0 || file >= len(inv.Files) {
		m.ActionError = errors.New("file target unavailable")
		return nil
	}
	f := inv.Files[file]
	path := f.NewPath
	if f.Status == "D" {
		path = f.OldPath
	}
	meta := inv.Comparison.Metadata
	t := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: string(path), SubjectType: "file"}
	if source.ValidateReviewCommentTarget(t) != nil || !commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, t) {
		m.ActionError = errors.New("file comment target is unavailable or invalid")
		return nil
	}
	m.rangeStart = nil
	m.Focus = paneDiff
	m.Composer = &commentComposer{Target: t, PendingIndex: -1}
	m.editorCursorVisible = true
	m.editorCursorGeneration++
	m.setOffset(0)
	return nextEditorCursorTick(m.editorCursorGeneration)
}

// switchCommentSide provides a keyboard choice on a paired split row.
func (m *Model) switchCommentSide() {
	if !m.sideBySideEnabled() || m.Focus != paneDiff {
		return
	}
	current := m.selectedDiffTarget()
	cursor := m.cursor()
	if current == nil || cursor < 0 {
		return
	}
	line := m.displayDetail()[cursor]
	if line.sideBySide == nil {
		return
	}
	for _, t := range rowTargets(*line.sideBySide) {
		if t.Side != current.Side {
			m.rangeStart = nil
			m.setSelectedDiffTarget(&t)
			return
		}
	}
}
