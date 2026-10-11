package tui

import (
	"reflect"

	"prui/internal/source"
)

// Only small, freshly rendered overlay blocks are compared on navigation.
// Source and composed rows are immutable and retained for one stream per tab.
// Comparing presentation also catches in-place edits, reactions, resolution,
// errors and caret changes without scattering invalidation across handlers.
type diffOverlays struct {
	before  []diffLine
	files   map[string][]diffLine
	targets map[source.ReviewCommentTarget][]diffLine
}

type diffOverlayCache struct {
	source, wrapped, rows []diffLine
	overlays              diffOverlays
	anchors               diffOverlayAnchors
	split                 bool
}

func overlayEndpoint(target source.ReviewCommentTarget) source.ReviewCommentTarget {
	target.StartLine, target.StartSide = 0, ""
	return target
}

// Anchor membership is indexed once per immutable logical stream. Unattached
// remote comments and local drafts never enter its rendered overlay surface.
type diffOverlayAnchors struct {
	targets map[source.ReviewCommentTarget]bool
	files   map[string]bool
}

func indexDiffOverlayAnchors(base []diffLine, split bool) diffOverlayAnchors {
	a := diffOverlayAnchors{targets: make(map[source.ReviewCommentTarget]bool), files: make(map[string]bool)}
	for _, line := range base {
		if line.Class == classFileHeader {
			a.files[line.Text] = true
		}
		targets := sourceLineTargets(line)
		if split {
			targets = nil
			if line.sideBySide != nil {
				targets = rowTargets(*line.sideBySide)
			}
		}
		for _, target := range targets {
			a.targets[target] = true
		}
	}
	return a
}

func (m *Model) diffOverlays(anchors diffOverlayAnchors) diffOverlays {
	o := diffOverlays{targets: make(map[source.ReviewCommentTarget][]diffLine)}
	// A file composer owns the leading editor surface, as before.
	if m.Composer != nil && m.Composer.Target.SubjectType == "file" {
		o.before = m.inlineEditorLines()
		return o
	}
	for _, c := range m.Comments {
		if c.ParentID != 0 {
			continue
		}
		if c.Target.SubjectType == "file" {
			if o.files == nil {
				o.files = make(map[string][]diffLine)
			}
			var rows []diffLine
			for _, file := range m.Session.Inventory.Files {
				if c.Target.Path == string(file.NewPath) || c.Target.Path == string(file.OldPath) {
					header := fileDivider(file)
					if !anchors.files[header] {
						continue
					}
					if rows == nil {
						rows = m.reviewCommentThread(c, 0)
					}
					o.files[header] = append(o.files[header], rows...)
				}
			}
		} else {
			target := overlayEndpoint(c.Target)
			if !anchors.targets[target] {
				continue
			}
			o.targets[target] = append(o.targets[target], m.reviewCommentThread(c, 0)...)
		}
	}
	if menu := m.CommentMenu; menu != nil && menu.mode == commentActionReply {
		target := overlayEndpoint(menu.Target)
		if anchors.targets[target] {
			o.targets[target] = append(o.targets[target], m.recoveredReplyLines(target)...)
		}
	}
	seen := make(map[source.ReviewCommentTarget]bool)
	for _, pending := range m.Pending {
		target := overlayEndpoint(pending.Target)
		if anchors.targets[target] && !seen[target] {
			seen[target] = true
			o.targets[target] = append(o.targets[target], m.pendingLines(target)...)
		}
	}
	if c := m.Composer; c != nil {
		target := overlayEndpoint(c.Target)
		if anchors.targets[target] {
			o.targets[target] = append(o.targets[target], m.inlineEditorLines()...)
		}
	}
	return o
}

func sameDiffStream(a, b []diffLine) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}

func (m *Model) withDiffOverlays(base []diffLine, split bool) []diffLine {
	wrapped := m.wrapSource(base)
	if len(m.Comments) == 0 && len(m.Pending) == 0 && m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply) {
		m.diffOverlayCache = diffOverlayCache{}
		return wrapped
	}
	c := &m.diffOverlayCache
	if !sameDiffStream(c.source, base) || c.split != split {
		*c = diffOverlayCache{source: base, split: split, anchors: indexDiffOverlayAnchors(base, split)}
	}
	overlays := m.diffOverlays(c.anchors)
	if sameDiffStream(c.source, base) && sameDiffStream(c.wrapped, wrapped) && reflect.DeepEqual(c.overlays, overlays) {
		return c.rows
	}
	rows := make([]diffLine, 0, len(wrapped)+len(overlays.before))
	rows = append(rows, overlays.before...)
	start := 0
	for i, line := range base {
		end := m.diffWrapCache.ends[i]
		rows = append(rows, wrapped[start:end]...)
		start = end
		if line.Class == classFileHeader {
			rows = append(rows, overlays.files[line.Text]...)
		}
		targets := sourceLineTargets(line)
		if split {
			targets = nil
			if line.sideBySide != nil {
				targets = rowTargets(*line.sideBySide)
			}
		}
		for _, target := range targets {
			rows = append(rows, overlays.targets[target]...)
		}
	}
	c.wrapped, c.rows, c.overlays = wrapped, rows, overlays
	return rows
}
