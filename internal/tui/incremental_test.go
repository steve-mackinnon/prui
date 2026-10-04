package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"path/filepath"
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
