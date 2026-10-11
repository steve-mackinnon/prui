package tui

import (
	"errors"
	"fmt"
	"prui/internal/source"
	"reflect"
	"strings"
	"testing"
)

func BenchmarkLargeJSONScroll(b *testing.B) {
	for _, n := range []int{10000, 100000} {
		for _, comments := range []bool{false, true} {
			b.Run(fmt.Sprintf("lines=%d/comments=%t", n, comments), func(b *testing.B) {
				s := largeTextSession(1, 1)
				s.Inventory.Files[0].NewPath = []byte("graph.json")
				s.Inventory.Files[0].OldPath = []byte("graph.json")
				var patch strings.Builder
				fmt.Fprintf(&patch, "@@ -0,0 +1,%d @@\n", n)
				for i := 0; i < n; i++ {
					fmt.Fprintf(&patch, "+  {\"node\": %d, \"value\": 0.12345},\n", i)
				}
				s.Inventory.Patches["p"] = []byte(patch.String())
				m := largeModel(s, 120, 40)
				defer m.Close()
				m.Files, m.Focus, m.cursorActive = true, paneDiff, true
				rows := m.displayDetail()
				if comments {
					m.Comments = []source.ReviewComment{{ID: 1, Target: *rows[10].target, Body: "A comment"}}
				}
				m.setOffset(n / 2)
				m.setCursor(n/2 + 5)
				_ = m.View()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if i%2 == 0 {
						key(m, 'j')
					} else {
						key(m, 'k')
					}
					_ = m.View()
				}
			})
		}
	}
}

// Warm navigation must not allocate in proportion to the source diff size,
// even when a comment enables the overlay path.
func TestDiffOverlayScrollAllocationBudget(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := largeModel(largeTextSession(1, 500), width, 40)
			defer m.Close()
			m.Files, m.Focus, m.cursorActive = true, paneDiff, true
			if width >= sideBySideMinimumWidth {
				m.layout = diffLayoutSideBySide
			}
			rows := m.displayDetail()
			m.Comments = []source.ReviewComment{{ID: 1, Target: *rows[10].target, Body: "comment"}}
			m.setOffset(100)
			m.setCursor(105)
			_ = m.View()
			n := 0
			allocs := testing.AllocsPerRun(3, func() {
				if n%2 == 0 {
					key(m, 'j')
				} else {
					key(m, 'k')
				}
				n++
				_ = m.View()
			})
			if allocs > 10000 {
				t.Fatalf("scroll + render allocated %.0f times; budget 10000", allocs)
			}
		})
	}
}

// Reference presentation preserves the pre-cache attachment semantics.
func (m *Model) uncachedOverlayDetail() []diffLine {
	base := m.baseDetail()
	// Source rows are immutable; the renderer copies just the visible viewport.
	// Avoid rebuilding the whole review on every navigation call without overlays.
	if m.Composer != nil && m.Composer.Target.SubjectType == "file" {
		return append(m.inlineEditorLines(), m.wrapSource(base)...)
	}
	if len(m.Comments) == 0 && len(m.Pending) == 0 && m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply) {
		return m.wrapSource(base)
	}
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, m.wrapSource([]diffLine{line})...)
		lines = append(lines, m.fileDiscussionLines(line)...)
		for _, target := range sourceLineTargets(line) {
			for _, comment := range m.Comments {
				if targetEndsAt(comment.Target, target) && comment.ParentID == 0 {
					lines = append(lines, m.reviewCommentThread(comment, 0)...)
				}
			}
			lines = append(lines, m.recoveredReplyLines(target)...)
			lines = append(lines, m.pendingLines(target)...)
			if m.Composer != nil && targetEndsAt(m.Composer.Target, target) {
				lines = append(lines, m.inlineEditorLines()...)
			}
		}
	}
	return lines
}

// Reference presentation preserves the pre-cache attachment semantics.
func (m *Model) uncachedOverlaySplitDetail() []diffLine {
	var base []diffLine
	if guide, ok := m.activeGuide(); ok {
		base = m.cachedGuideDetail(guide).splitLines
	} else if m.fileView() {
		base = m.presentedFileDetail(true)
	} else {
		base = projectSideBySideDetail(m.baseDetail())
	}
	if m.Composer != nil && m.Composer.Target.SubjectType == "file" {
		return append(m.inlineEditorLines(), m.wrapSource(base)...)
	}
	if len(m.Comments) == 0 && len(m.Pending) == 0 && m.Composer == nil && (m.CommentMenu == nil || m.CommentMenu.mode != commentActionReply) {
		return m.wrapSource(base)
	}
	lines := make([]diffLine, 0, len(base)+len(m.Comments)+2)
	for _, line := range base {
		lines = append(lines, m.wrapSource([]diffLine{line})...)
		lines = append(lines, m.fileDiscussionLines(line)...)
		if line.sideBySide == nil {
			continue
		}
		row := *line.sideBySide
		// A paired row can have comments on both sides. Keep their overlay
		// order stable: old/LEFT before new/RIGHT.
		for _, target := range rowTargets(row) {
			for _, comment := range m.Comments {
				if targetEndsAt(comment.Target, target) && comment.ParentID == 0 {
					lines = append(lines, m.reviewCommentThread(comment, 0)...)
				}
			}
			lines = append(lines, m.recoveredReplyLines(target)...)
			lines = append(lines, m.pendingLines(target)...)
			if m.Composer != nil && targetEndsAt(m.Composer.Target, target) {
				lines = append(lines, m.inlineEditorLines()...)
			}
		}
	}
	return lines
}

func TestDiffOverlayCachePresentationAndFreshness(t *testing.T) {
	for _, width := range []int{120, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := fileScrollModel(width)
			defer m.Close()
			var target source.ReviewCommentTarget
			for _, row := range m.baseDetail() {
				if row.target != nil && row.target.Side == "RIGHT" {
					target = *row.target
					break
				}
			}
			target.StartLine, target.StartSide = target.Line, target.Side
			m.Comments = []source.ReviewComment{{ID: 1, Target: target, Body: "root " + strings.Repeat("long comment ", 20)}, {ID: 2, ParentID: 1, Target: target, Body: "reply"}}
			check := func(label string) {
				t.Helper()
				got := m.displayDetail()
				if !sameDiffStream(got, m.displayDetail()) {
					t.Fatalf("%s: unchanged stream was rebuilt", label)
				}
				var want []diffLine
				if m.sideBySideEnabled() {
					want = m.uncachedOverlaySplitDetail()
				} else {
					want = m.uncachedOverlayDetail()
				}
				if !reflect.DeepEqual(want, got) {
					t.Fatalf("%s: cached rows differ from original presentation", label)
				}
				// The reference renderer disturbs the one-entry wrap cache.
				// Rewarm it so the next edit exercises a genuine cache hit.
				_ = m.displayDetail()
			}
			check("range comment and reply")
			m.Comments[0].Body = "edited in place"
			check("edited comment")
			m.CommentReactions = map[int64][]source.ReviewCommentReaction{1: {{Content: "+1"}}}
			check("reaction")
			m.CommentMenu = &commentActionMenu{CommentID: 1, ReplyToID: 1, Target: target, mode: commentActionReply, Draft: "reply draft"}
			check("reply editor")
			m.CommentMenu.Draft = "changed reply"
			check("edited reply")
			m.CommentMenu = nil
			m.Pending = []source.ReviewComment{{Target: target, Body: "first pending"}, {Target: target, Body: "second pending"}}
			m.Composer = &commentComposer{Target: target, Draft: "draft", PendingIndex: -1}
			check("pending and composer")
			m.Composer.Draft = "edited\n" + strings.Repeat("wrapped draft ", 20)
			m.Composer.Cursor = 5
			check("edited composer")
			m.editorCursorVisible = !m.editorCursorVisible
			check("caret")
			m.ActionError = errors.New("synthetic error")
			check("editor error")
			m.Width -= 10
			check("resize")
			m.hideLineNumbers = !m.hideLineNumbers
			m.diffWrapCache = diffWrapCache{}
			check("line numbers")
			m.Composer.Target.SubjectType = "file"
			check("file composer")
			m.Composer, m.CommentMenu, m.Pending, m.Comments = nil, nil, nil, nil
			check("removed overlays")
			m.Comments = []source.ReviewComment{{ID: 3, Target: source.ReviewCommentTarget{Path: target.Path, SubjectType: "file"}, Body: "file comment"}}
			check("file comment")
			m.Session = largeTextSession(1, 50)
			m.Session.Inventory.Patches["p"] = []byte("@@ -1 +1 @@\n-old\n+new snapshot\n")
			check("new snapshot")
		})
	}
}

func TestDiffOverlayIgnoresUnanchoredComment(t *testing.T) {
	m := fileScrollModel(120)
	defer m.Close()
	before := m.displayDetail()
	// This empty submit-result fixture has no source anchor and must never be
	// expanded as a thread (its zero ID is also its zero parent ID).
	m.Comments = []source.ReviewComment{{}}
	if got := m.displayDetail(); !reflect.DeepEqual(got, before) {
		t.Fatal("unanchored comment entered the source stream")
	}
}
