package tui

import (
	"prui/internal/commits"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
)

func commentType(c *commentComposer) string {
	if c.Suggestion {
		return "Suggestion"
	}
	if c.Target.SubjectType == "file" {
		return "File comment"
	}
	return "Comment"
}

func (m *Model) composerLineTarget() *source.ReviewCommentTarget {
	c := m.Composer
	if c.Modes != nil {
		return c.Modes.LineTarget
	}
	if c.Target.SubjectType == "file" {
		return nil
	}
	target := c.Target
	return &target
}

func (m *Model) commentTypes() []string {
	c := m.Composer
	if c.CommitSHA != "" {
		return []string{"Comment"}
	}
	var types []string
	if target := m.composerLineTarget(); target != nil {
		types = append(types, "Comment")
		if target.Side == "RIGHT" && !m.commitFilter.subset {
			if _, ok := m.targetSource(*target); ok {
				types = append(types, "Suggestion")
			}
		}
	}
	if m.Session != nil && !m.commitFilter.subset {
		target := c.Target
		target.Side, target.Line, target.StartLine, target.StartSide, target.SubjectType = "", 0, 0, "", "file"
		inv := m.Session.Inventory
		if source.ValidateReviewCommentTarget(target) == nil && commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, target) {
			types = append(types, "File comment")
		}
	}
	if len(types) == 0 {
		types = []string{commentType(c)}
	}
	return types
}

func (m *Model) commentTypeSelector() string {
	types := m.commentTypes()
	for i, mode := range types {
		if mode == commentType(m.Composer) {
			types[i] = "[" + mode + "]"
		}
	}
	label := strings.Join(types, " · ")
	if len(types) > 1 {
		label += " · tab: type"
	}
	return label
}

func (m *Model) cycleCommentType(reverse bool) {
	c := m.Composer
	types := m.commentTypes()
	if len(types) < 2 {
		return
	}
	current := commentType(c)
	next := 0
	for i, mode := range types {
		if mode == current {
			step := 1
			if reverse {
				step = -1
			}
			next = (i + step + len(types)) % len(types)
			break
		}
	}
	if c.Modes == nil {
		c.Modes = &session.CommentModes{LineTarget: m.composerLineTarget(), Drafts: map[string]string{}}
	}
	if c.Modes.Drafts == nil {
		c.Modes.Drafts = map[string]string{}
	}
	c.Modes.Drafts[current] = c.Draft
	mode := types[next]
	draft, exists := c.Modes.Drafts[mode]
	c.Suggestion = mode == "Suggestion"
	if mode == "File comment" {
		c.Target.Side, c.Target.Line, c.Target.StartLine, c.Target.StartSide, c.Target.SubjectType = "", 0, 0, "", "file"
	} else {
		c.Target = *c.Modes.LineTarget
	}
	if c.Suggestion {
		c.Before, _ = m.targetSource(c.Target)
		if !exists {
			draft = c.Before
		}
	}
	c.Draft, c.Cursor = draft, len([]rune(draft))
	m.ActionError = nil
	m.ensureInlineEditorVisible()
	if current == "File comment" && mode == "Comment" {
		// A restored draft may be taller than the viewport. Return to its
		// source line even when the entire editor cannot fit beside it.
		target := c.Target
		target.StartLine, target.StartSide = 0, ""
		m.restoreCursorAnchor(&target, 0)
		m.ensureCursorVisible()
	}
}
