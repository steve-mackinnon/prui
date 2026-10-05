package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"os"
	"path/filepath"
	"prui/internal/incremental"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
	"reflect"
	"strings"
	"testing"
)

type incrementalGH struct{ metadata source.Metadata }

func (g incrementalGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.metadata, nil
}
func (incrementalGH) Token(context.Context) (string, error) { panic("live token forbidden") }
func (incrementalGH) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	panic("live request forbidden")
}

func TestIncrementalViewNavigatesCapturedDiffAndPrivateDraftOutcomes(t *testing.T) {
	ctx := context.Background()
	r := testutil.NewRepo(t)
	r.Write("stable", "before\ncontext\n")
	r.Write("changed", "before\n")
	base := r.Commit()
	r.Write("stable", "after\ncontext\n")
	r.Write("changed", "after\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	raw, err := review.Open(ctx, r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: head, Path: "stable", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
	pending := source.ReviewComment{Target: target, Body: "PRIVATE PENDING PAYLOAD"}
	file := target
	file.SubjectType = "file"
	file.Side = ""
	file.StartSide = ""
	file.Line = 0
	file.StartLine = 0
	attempted := source.PullRequestReview{Identity: meta.Identity, CommitID: head, Event: "COMMENT", Body: "PRIVATE SUMMARY", Comments: []source.ReviewComment{pending}}
	d := session.Draft{Version: 2, Pending: []source.ReviewComment{pending}, Summary: attempted.Body, Composer: &session.DraftEditor{Target: file, Body: "PRIVATE FILE PAYLOAD", PendingIndex: -1}, Attempt: "review", Attempted: &session.DraftAttempt{Kind: "review", Review: &attempted}}
	savedDraft, err := store.SaveDraft(ctx, session.DraftKeyFor(meta), 0, d)
	if err != nil {
		t.Fatal(err)
	}
	r.Write("changed", "again\n")
	r.Write("z-added", strings.Repeat("scroll me\n", 50)+"END OF NEW FILE\n")
	next := meta
	next.HeadSHA = r.Commit()
	fresh, err := review.OpenWithConfig(ctx, r.Dir, meta.Identity, incrementalGH{next}, source.NewRunner(), source.Defaults(), nil, review.Config{Previous: old})
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.CreateWithProgress(fresh.Snapshot, fresh.ReviewedSliceIDs)
	if err != nil {
		t.Fatal(err)
	}
	m := New(ctx, nil)
	defer m.Close()
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(current)
	m.Width = 100
	m.Height = 12
	posts := 0
	m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		posts++
		return source.ReviewComment{}, nil
	})
	key(m, '5')
	view := ansi.Strip(m.View().Content)
	if m.top() != pageIncremental || !strings.Contains(view, "+again") || !strings.Contains(view, "-after") {
		t.Fatalf("actual diff missing\n%s", view)
	}
	key(m, 'n')
	namedKey(m, tea.KeyEnd)
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "END OF NEW FILE") {
		t.Fatalf("diff navigation missing\n%s", view)
	}
	key(m, 'd')
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "stable RIGHT 1-2") || !strings.Contains(view, "delivery uncertain") {
		t.Fatalf("range outcome missing\n%s", view)
	}
	namedKey(m, tea.KeyEnd)
	view += ansi.Strip(m.View().Content)
	if strings.Contains(view, "PRIVATE") || posts != 0 {
		t.Fatal("private payload leaked or navigation wrote")
	}
	afterDraft, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
	if err != nil || !reflect.DeepEqual(afterDraft, savedDraft) {
		t.Fatalf("original draft mutated %v", err)
	}
	if nextDraft, err := store.LoadDraft(ctx, session.DraftKeyFor(next)); err != nil || nextDraft.Generation != 0 {
		t.Fatalf("drafts copied %v", err)
	}
	cmd := m.incrementalKey("o")
	if cmd == nil {
		t.Fatal("old snapshot not accessible")
	}
	m.Update(cmd())
	if m.Session.ID != old.ID || m.top() != pageDraftRecovery || m.draft.attempt != "review" || m.Pending[0].Target != target {
		t.Fatal("original attempted payload not recovered")
	}
	if posts != 0 {
		t.Fatal("original snapshot access submitted")
	}
}

func TestIncrementalRestackCombinesProgressExtendedDraftsAndOfflineRecovery(t *testing.T) {
	for _, change := range []string{"push", "base change", "rename", "delete"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			r := testutil.NewRepo(t)
			r.Write("stable", "before\ncontext\n")
			r.Write("changed", "before\ncontext\n")
			base := r.Commit()
			r.Write("stable", "after\ncontext\n")
			r.Write("changed", "after\ncontext\n")
			head := r.Commit()
			meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 9}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
			store, err := session.Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			raw, err := review.OpenWithConfig(ctx, r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{CacheFullSource: true})
			if err != nil {
				t.Fatal(err)
			}
			old, err := store.Create(raw.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, slice := range old.Slices {
				if err := review.Mark(ctx, store, old, slice.FileID, true); err != nil {
					t.Fatal(err)
				}
			}
			rangeTarget := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: head, Path: "stable", Side: "RIGHT", StartSide: "RIGHT", StartLine: 1, Line: 2}
			fileTarget := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: head, Path: "stable", SubjectType: "file"}
			changedTarget := rangeTarget
			changedTarget.Path = "changed"
			changedTarget.Side = "LEFT"
			changedTarget.StartSide = "LEFT"
			root := rangeTarget
			d := session.Draft{Version: 2, Summary: "private summary", Event: 2, Pending: []source.ReviewComment{{Target: rangeTarget, Body: "private range"}, {Target: changedTarget, Body: "private old-side range"}}, Composer: &session.DraftEditor{Target: fileTarget, Body: "private file", PendingIndex: -1}, Reply: &session.DraftEditor{Target: rangeTarget, RootAnchor: &root, Body: "private reply", CommentID: 17, ReplyToID: 17}}
			originalDraft, err := store.SaveDraft(ctx, session.DraftKeyFor(meta), 0, d)
			if err != nil {
				t.Fatal(err)
			}
			next := meta
			switch change {
			case "base change":
				r.Git("checkout", "--detach", base)
				r.Write("stable", "different base\ncontext\n")
				next.BaseSHA = r.Commit()
				r.Git("checkout", "--detach", head)
				r.Git("merge", "--no-commit", "--strategy=ours", next.BaseSHA)
			case "rename":
				r.Git("mv", "stable", "renamed")
			case "delete":
				if err := os.Remove(filepath.Join(r.Dir, "stable")); err != nil {
					t.Fatal(err)
				}
			}
			r.Write("changed", "again\ncontext\n")
			next.HeadSHA = r.Commit()
			fresh, err := review.OpenWithConfig(ctx, r.Dir, meta.Identity, incrementalGH{next}, source.NewRunner(), source.Defaults(), nil, review.Config{Previous: old, CacheFullSource: true})
			if err != nil {
				t.Fatal(err)
			}
			current, err := store.CreateWithProgress(fresh.Snapshot, fresh.ReviewedSliceIDs)
			if err != nil {
				t.Fatal(err)
			}
			if change == "push" {
				if len(current.ReviewedSliceIDs) != 1 {
					t.Fatal("unchanged range/file content lost progress")
				}
			} else if len(current.ReviewedSliceIDs) != 0 {
				t.Fatal("base/rename/delete incorrectly retained reading progress")
			}
			if err := os.RemoveAll(r.Dir); err != nil {
				t.Fatal(err)
			}
			current, err = store.Load(current.ID)
			if err != nil {
				t.Fatal(err)
			}
			oldBefore, err := store.Load(old.ID)
			if err != nil {
				t.Fatal(err)
			}
			m := New(ctx, nil)
			defer m.Close()
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(oldBefore)
			filterCtx, filterCancel := context.WithCancel(ctx)
			defer filterCancel()
			discussionCtx, discussionCancel := context.WithCancel(ctx)
			defer discussionCancel()
			m.commitFilter = commitFilterState{subset: true, generation: 4, cancel: filterCancel, inventory: oldBefore.Inventory}
			event := source.ConversationEvent{ID: "live", Kind: "PR comment", Body: "ephemeral conversation"}
			stale := DiscussionSnapshot{Snapshot: source.DiscussionSnapshot{Complete: true, Events: []source.ConversationEvent{event}}}
			m.discussions = discussionState{loaded: true, generation: 3, cancel: discussionCancel, snapshot: stale}
			searchCtx, searchCancel := context.WithCancel(ctx)
			defer searchCancel()
			oldSearch := &diffSearchState{session: oldBefore, generation: 9, open: true, pending: true, cancel: searchCancel}
			m.search[0] = oldSearch
			m.navigation = codeNavigation{mode: "NEW", whitespace: true}
			m.incremental = incrementalViewState{file: 10, offset: 100, drafts: true}
			m.Update(ActionResult{Session: current, Reset: true})
			m.Update(diffSearchResult{state: oldSearch, generation: 9})
			m.Update(DiscussionResult{Target: 0, Session: oldBefore, Generation: 3, Snapshot: stale})
			if filterCtx.Err() != context.Canceled || discussionCtx.Err() != context.Canceled || m.commitFilter.subset || m.incremental.drafts || m.incremental.offset != 0 || m.discussions.loaded || len(m.discussions.snapshot.Snapshot.Events) != 0 {
				t.Fatal("comparison reset retained derived filter/conversation/changes state")
			}
			if searchCtx.Err() != context.Canceled || m.search[0] != nil || m.navigation != (codeNavigation{}) {
				t.Fatal("comparison reset retained previous pinned-source navigation/search")
			}
			if current.Inventory.FullSource == nil || oldBefore.Inventory.FullSource == nil {
				t.Fatal("consented pinned source lost during incremental capture or offline reload")
			}
			if len(m.Pending) != 0 || m.Composer != nil || m.CommentMenu != nil || m.ReviewForm != nil {
				t.Fatal("old extended drafts retargeted into new comparison")
			}
			m.openIncremental()
			outcomes := strings.Join(m.incremental.outcomes, "\n")
			want := string(incremental.AnchorUnchanged)
			switch change {
			case "base change":
				want = string(incremental.AnchorChanged)
			case "rename":
				want = string(incremental.AnchorRenamed)
			case "delete":
				want = string(incremental.AnchorRemoved)
			}
			if !strings.Contains(outcomes, "stable RIGHT 1-2 — "+want) || !strings.Contains(outcomes, "stable (file) — "+want) || !strings.Contains(outcomes, "changed LEFT 1-2 — "+string(incremental.AnchorChanged)) {
				t.Fatalf("complete target outcomes missing:\n%s", outcomes)
			}
			if strings.Contains(outcomes, "private range") || strings.Contains(outcomes, "ephemeral conversation") {
				t.Fatal("payload leaked into anchor report")
			}
			unchangedOld, err := store.Load(old.ID)
			if err != nil || !reflect.DeepEqual(unchangedOld, oldBefore) {
				t.Fatal("offline outcome changed original snapshot", err)
			}
			recovered, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(recovered, originalDraft) {
				t.Fatal("offline outcomes changed private range/file/reply data", err)
			}
			if newDraft, err := store.LoadDraft(ctx, session.DraftKeyFor(next)); err != nil || newDraft.Generation != 0 {
				t.Fatal("old drafts silently copied", err)
			}
			cmd := m.incrementalKey("o")
			if cmd == nil {
				t.Fatal("offline predecessor unavailable")
			}
			m.Update(cmd())
			if m.Session.ID != old.ID || m.top() != pageDraftRecovery || m.Pending[0].Target != rangeTarget || m.Pending[1].Target != changedTarget || m.Composer.Target != fileTarget || m.CommentMenu.RootAnchor == nil || *m.CommentMenu.RootAnchor != root || m.ReviewForm.Body != d.Summary || m.ReviewForm.Event != d.Event {
				t.Fatal("offline recovery flattened or retargeted original extended drafts")
			}
		})
	}
}
