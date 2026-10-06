package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/commits"
	"prui/internal/review"
	"prui/internal/source"
)

func TestProgramCommitDiscussionReadFallbackJumpReturn(t *testing.T) {
	m := commitModel(t)
	m.Loading = false
	m.selectReviewView(viewDescription)
	sha := m.commitEntries()[1].SHA
	d := testDiscussion("thread-captured", sha)
	lost := testDiscussion("thread-unavailable", strings.Repeat("f", 40))
	lost.Comments[0].ID = 2
	lost.Comments[0].Body = "Unavailable original concern"
	var reads atomic.Int32
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		reads.Add(1)
		return DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{d, lost}}}, nil
	})
	h := runProgram(t, m)
	h.expect("description", func(f programFrame) bool { return f.page == pageReview && !f.loading })
	h.key('c')
	h.expect("loaded discussions", func(f programFrame) bool {
		return reads.Load() == 1 && !strings.Contains(f.text, "Refreshing discussions")
	})
	h.key('D')
	h.expect("discussion list", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "Outdated on current PR")
	})
	h.key(tea.KeyEnter)
	h.expect("captured detail", func(f programFrame) bool {
		return strings.Contains(f.text, "Historical concern") && strings.Contains(f.text, "Historical snippet")
	})
	h.key('o')
	h.expect("jump to commit", func(f programFrame) bool { return f.page == pageReview && strings.Contains(f.text, "Second commit") })
	h.key(tea.KeyEscape)
	h.expect("return detail", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "Historical snippet")
	})
	h.key(tea.KeyEscape)
	h.expect("return list", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "enter: expand") && strings.Contains(f.text, "Unavailable original concern")
	})
	h.key(tea.KeyTab)
	h.key(tea.KeyEnter)
	h.expect("fallback detail", func(f programFrame) bool {
		return strings.Contains(f.text, "Original commit not captured") && strings.Contains(f.text, "Unavailable original concern")
	})
	h.key('o')
	h.expect("fallback remains readable", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "Original commit not captured")
	})
	if reads.Load() != 1 {
		t.Fatal("navigation fetched discussions")
	}
	h.key(tea.KeyEscape)
	h.key(tea.KeyEscape)
	h.expect("review restored", func(f programFrame) bool { return f.page == pageReview })
	h.quit()
}

func TestProgramCommitCommentNullCurrentAndPartialRefresh(t *testing.T) {
	for _, historical := range []bool{false, true} {
		t.Run(fmt.Sprintf("historical=%v", historical), func(t *testing.T) {
			m := commitModel(t)
			m.Loading = false
			s := m.Session
			sha := s.Inventory.Comparison.Metadata.HeadSHA
			if historical {
				sha = strings.Repeat("d", 40)
				s.Commits.Entries[0].SHA = sha
				s.Commits.Entries[0].Parents = []string{strings.Repeat("c", 40)}
				m.commit.selectedSHA = sha
				m.commit.cache = commitRenderCache{}
				for i := range s.Inventory.Files {
					s.Inventory.Files[i].Status = "M"
					s.Inventory.Files[i].OldPath = append([]byte(nil), s.Inventory.Files[i].NewPath...)
				}
			}
			s.Commits.Entries[0].Diff = &commits.Diff{Complete: true, Files: s.Inventory.Files, Units: s.Inventory.Units, Patches: s.Inventory.Patches}
			var target source.ReviewCommentTarget
			rowIndex := 0
			for i, row := range m.commitRows() {
				if row.target != nil && row.target.Side == "RIGHT" {
					target = *row.target
					rowIndex = i
					break
				}
			}
			if target.Line == 0 {
				t.Fatal("no supported head fixture")
			}
			m.commit.focus = paneDiff
			m.commit.cursors = map[string]int{sha: rowIndex}
			m.commit.offsets = map[string]int{sha: rowIndex}
			var posts atomic.Int32
			m.SetCommentSubmitter(func(_ context.Context, v CommentSubmission) (source.ReviewComment, error) {
				posts.Add(1)
				if v.CommitSHA != sha || v.Comment.Target != target {
					t.Errorf("retargeted submission: %+v", v)
				}
				return source.ReviewComment{ID: 991, Author: "alice", Body: v.Comment.Body, OriginalAnchor: &target, Target: source.ReviewCommentTarget{Identity: target.Identity, CommitID: sha, Path: target.Path, Side: target.Side}}, nil
			})
			m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
				return DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: false, Reason: "Reply limit reached"}}, nil
			})
			h := runProgram(t, m)
			h.expect("commit review", func(f programFrame) bool { return f.page == pageReview && !f.loading })
			h.key(tea.KeyEnter)
			h.expect("commit composer", func(f programFrame) bool { return strings.Contains(f.text, "Comment on ") })
			h.key('x')
			h.key(tea.KeyEnter)
			h.expect("confirmed creation", func(f programFrame) bool {
				return posts.Load() == 1 && !f.busy && strings.Contains(f.text, "Status unknown")
			})
			h.key('D')
			h.expect("created overlay", func(f programFrame) bool { return f.page == pageReview && strings.Contains(f.text, "@alice") })
			h.key('c')
			h.expect("partial retains created", func(f programFrame) bool {
				return f.page == pageReview && strings.Contains(f.text, "Loaded activity") && strings.Contains(f.text, "@alice")
			})
			h.key(tea.KeyEnter)
			h.expect("created detail", func(f programFrame) bool {
				return strings.Contains(f.text, "Status unknown") && strings.Contains(f.text, "@alice")
			})
			if posts.Load() != 1 {
				t.Fatal("refresh retried write")
			}
			h.key(tea.KeyEscape)
			h.key(tea.KeyEscape)
			h.expect("review", func(f programFrame) bool { return f.page == pageReview })
			h.quit()
		})
	}
}

func TestProgramDiscussionFreshnessMismatchSuppressesMainCards(t *testing.T) {
	m := commitModel(t)
	m.Loading = false
	m.selectReviewView(viewFiles)
	m.Focus = paneDiff
	var target *source.ReviewCommentTarget
	for _, row := range m.baseDetail() {
		if row.target != nil {
			copy := *row.target
			target = &copy
			break
		}
	}
	if target == nil {
		t.Fatal("no main fixture anchor")
	}
	no := false
	thread := source.Discussion{ID: "live-thread", CurrentAnchor: target, Outdated: &no, Resolved: &no, Comments: []source.ReviewComment{{ID: 701, Author: "alice", Body: "Live snapshot concern"}}}
	m.SetDiscussionReader(func(context.Context, *review.Session) (DiscussionSnapshot, error) {
		return DiscussionSnapshot{CurrentVerified: false, Reason: "Snapshot freshness unknown", Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{thread}}}, nil
	})
	h := runProgram(t, m)
	h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.loading })
	h.key('c')
	h.expect("refresh complete without misplaced card", func(f programFrame) bool {
		return f.page == pageReview && !strings.Contains(f.text, "Refreshing") && !strings.Contains(f.text, "Live snapshot concern")
	})
	h.key('D')
	h.expect("live discussion remains discoverable", func(f programFrame) bool {
		return f.page == pageReview && strings.Contains(f.text, "snapshot differs") && strings.Contains(f.text, "Live snapshot concern")
	})
	h.key('2')
	h.expect("main remains clean", func(f programFrame) bool {
		return f.page == pageReview && !strings.Contains(f.text, "Live snapshot concern")
	})
	h.quit()
}
