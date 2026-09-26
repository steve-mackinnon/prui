package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
	"pr-review/internal/tui"
)

type fixtureGH struct {
	value      source.Metadata
	err        error
	prs        []source.PullRequest
	comments   []source.ReviewComment
	commentErr error
	reviews    []source.PullRequestReview
	reviewErr  error
}

type requestFunc func(context.Context, source.Request) ([]byte, error)

func (f requestFunc) Run(ctx context.Context, request source.Request) ([]byte, error) {
	return f(ctx, request)
}

type unavailableGuideAnalyzer struct{}

func (unavailableGuideAnalyzer) Analyze(context.Context, guide.Input) (guide.Bundle, error) {
	return guide.Bundle{}, errors.New("provider unavailable")
}

type generatedGuideAnalyzer struct{}

func (generatedGuideAnalyzer) Analyze(_ context.Context, in guide.Input) (guide.Bundle, error) {
	ids := make([]string, len(in.Units))
	for i, unit := range in.Units {
		ids[i] = unit.ID
	}
	return guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Review guide", Sections: []guide.Section{{Title: "Changed code", UnitIDs: ids}}}}}, nil
}

func (g *fixtureGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.value, g.err
}
func (g *fixtureGH) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	return g.prs, g.err
}

func (g *fixtureGH) CreateReviewComment(_ context.Context, comment source.ReviewComment) (source.ReviewComment, error) {
	g.comments = append(g.comments, comment)
	return comment, g.commentErr
}

func (g *fixtureGH) CreatePullRequestReview(_ context.Context, review source.PullRequestReview) error {
	g.reviews = append(g.reviews, review)
	return g.reviewErr
}

func TestSubmitPullRequestReviewRequiresFrozenComparison(t *testing.T) {
	app, saved := wiringFixture(t)
	gh := app.gh.(*fixtureGH)
	frozen := saved.Inventory.Comparison.Metadata
	review := source.PullRequestReview{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Event: "APPROVE"}
	submission := tui.ReviewSubmission{Metadata: frozen, Review: review}
	if err := app.submitPullRequestReview(context.Background(), submission); err != nil || len(gh.reviews) != 1 {
		t.Fatal(err, gh.reviews)
	}
	gh.value.HeadSHA = frozen.BaseSHA
	if err := app.submitPullRequestReview(context.Background(), submission); err == nil || !strings.Contains(err.Error(), "new comparison") || len(gh.reviews) != 1 {
		t.Fatal("stale review was submitted", err, gh.reviews)
	}
	gh.value = frozen
	app.offline = true
	if err := app.submitPullRequestReview(context.Background(), submission); err == nil || len(gh.reviews) != 1 {
		t.Fatal("offline review was submitted", err, gh.reviews)
	}
}

func TestSubmitReviewCommentRequiresExactFrozenMetadata(t *testing.T) {
	app, saved := wiringFixture(t)
	gh := app.gh.(*fixtureGH)
	frozen := saved.Inventory.Comparison.Metadata
	comment := source.ReviewComment{Target: source.ReviewCommentTarget{
		Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1,
	}, Body: "Please consider this."}

	submission := tui.CommentSubmission{Comment: comment, Metadata: frozen}
	if _, err := app.submitReviewComment(context.Background(), submission); err != nil {
		t.Fatal("matching metadata rejected", err)
	}
	if len(gh.comments) != 1 || gh.comments[0] != comment {
		t.Fatalf("comments = %#v, want one frozen request %#v", gh.comments, comment)
	}

	for name, current := range map[string]source.Metadata{
		"base repository": {Identity: frozen.Identity, BaseRepository: "other/repo", HeadRepository: frozen.HeadRepository, BaseSHA: frozen.BaseSHA, HeadSHA: frozen.HeadSHA},
		"head repository": {Identity: frozen.Identity, BaseRepository: frozen.BaseRepository, HeadRepository: "other/repo", BaseSHA: frozen.BaseSHA, HeadSHA: frozen.HeadSHA},
		"base SHA":        {Identity: frozen.Identity, BaseRepository: frozen.BaseRepository, HeadRepository: frozen.HeadRepository, BaseSHA: frozen.HeadSHA, HeadSHA: frozen.HeadSHA},
		"head SHA":        {Identity: frozen.Identity, BaseRepository: frozen.BaseRepository, HeadRepository: frozen.HeadRepository, BaseSHA: frozen.BaseSHA, HeadSHA: frozen.BaseSHA},
	} {
		t.Run(name, func(t *testing.T) {
			gh.value = current
			before := len(gh.comments)
			if _, err := app.submitReviewComment(context.Background(), submission); err == nil || !strings.Contains(err.Error(), "new comparison") {
				t.Fatal("mismatched metadata was allowed", err)
			}
			if len(gh.comments) != before {
				t.Fatal("mismatch called commenter")
			}
		})
	}
}

func TestSubmitReviewCommentRejectsBeforeCommenter(t *testing.T) {
	app, saved := wiringFixture(t)
	gh := app.gh.(*fixtureGH)
	frozen := saved.Inventory.Comparison.Metadata
	valid := source.ReviewComment{Target: source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "body"}
	for name, mutate := range map[string]func(){
		"offline":            func() { app.offline = true },
		"invalid target":     func() { valid.Target.Line = 0 },
		"unavailable GitHub": func() { app.gh = nil },
	} {
		t.Run(name, func(t *testing.T) {
			app.offline, app.gh, valid = false, gh, source.ReviewComment{Target: source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "body"}
			mutate()
			before := len(gh.comments)
			if _, err := app.submitReviewComment(context.Background(), tui.CommentSubmission{Comment: valid, Metadata: frozen}); err == nil {
				t.Fatal("invalid delivery was allowed")
			}
			if len(gh.comments) != before {
				t.Fatal("rejected delivery called commenter")
			}
		})
	}
}

func TestSubmitReviewCommentDoesNotRetryUnknownDelivery(t *testing.T) {
	app, saved := wiringFixture(t)
	gh := app.gh.(*fixtureGH)
	frozen := saved.Inventory.Comparison.Metadata
	submission := tui.CommentSubmission{Metadata: frozen, Comment: source.ReviewComment{Target: source.ReviewCommentTarget{
		Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1,
	}, Body: "body"}}
	for name, deliveryErr := range map[string]error{
		"canceled": context.Canceled,
		"unknown":  errors.New("connection ended after request"),
	} {
		t.Run(name, func(t *testing.T) {
			gh.comments, gh.commentErr = nil, deliveryErr
			_, err := app.submitReviewComment(context.Background(), submission)
			if !errors.Is(err, deliveryErr) || len(gh.comments) != 1 {
				t.Fatalf("delivery error = %v, calls = %d; want propagated error and one call", err, len(gh.comments))
			}
		})
	}
}

func TestLifecycleListPullRequests(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	g := &fixtureGH{prs: []source.PullRequest{{Identity: source.Identity{Repository: "owner/repo", Number: 42}, Title: "Add\x1btitle"}}}
	app := application{store: store, gh: g}
	prs, err := app.listPullRequests(context.Background(), "owner/repo")
	if err != nil || len(prs) != 1 {
		t.Fatal(prs, err)
	}
	var out strings.Builder
	listPullRequests(&out, "owner/repo", prs)
	if got := out.String(); !strings.Contains(got, "owner/repo #42 Add\\x1btitle") || strings.ContainsAny(got, "\x1b\a") {
		t.Fatal("list output was unsafe", got)
	}
	if _, err := app.listPullRequests(context.Background(), "../repo"); err == nil {
		t.Fatal("invalid repository listed")
	}
}
func (g *fixtureGH) Token(context.Context) (string, error) { panic("credentials must not be used") }

// captureStdout records what a CLI run writes to stdout; plain output must stay
// free of terminal control bytes even while the interactive view is colored.
func captureStdout(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	code := f()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out, code
}

func TestLifecycleOpenResumeNewAndOfflineCLI(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	g := &fixtureGH{value: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}}
	path := filepath.Join(t.TempDir(), "sessions")
	store, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := application{store: store, gh: g, runner: source.NewRunner(), limits: source.Defaults()}
	saved, err := app.load(context.Background(), options{Command: "open", Checkout: r.Dir, Identity: g.value.Identity}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.RevisionStatus != session.Current {
		t.Fatal("open did not persist/check")
	}
	if err := review.Mark(store, saved, saved.Slices[0].FileID, true); err != nil {
		t.Fatal(err)
	}
	r.Write("a", "third\n")
	g.value.HeadSHA = r.Commit()
	resumed, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID}, func(string) {})
	if err != nil || resumed.RevisionStatus != session.Stale {
		t.Fatal("resume did not check", err)
	}
	fresh, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID, New: true}, func(string) {})
	if err != nil || fresh.ID == saved.ID || len(fresh.ReviewedSliceIDs) != 0 {
		t.Fatal("new comparison reused progress", err)
	}
	old, err := store.Load(saved.ID)
	if err != nil || len(old.ReviewedSliceIDs) != 1 {
		t.Fatal("old session destroyed", err)
	}
	g.err = errors.New("authentication unavailable")
	failed, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID}, func(string) {})
	if err != nil || failed.RevisionStatus != session.CheckFailed {
		t.Fatal("offline failed to resume", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(r.Dir); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"sessions", "--store", path},
		{"resume", saved.ID, "--store", path, "--offline", "--plain"},
		{"delete", saved.ID, "--store", path},
	} {
		out, code := captureStdout(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if strings.ContainsAny(out, "\x1b\a") {
			t.Fatalf("%v: terminal control bytes in plain output", args)
		}
	}
	if _, err := os.Stat(filepath.Join(path, saved.ID)); !os.IsNotExist(err) {
		t.Fatal("CLI deletion incomplete")
	}
}

func TestLifecycleRememberedCheckout(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	g := &fixtureGH{value: source.Metadata{Identity: source.Identity{Repository: "Owner/Repo", Number: 1}, BaseRepository: "Owner/Repo", HeadRepository: "Owner/Repo", BaseSHA: base, HeadSHA: head}}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := application{store: store, gh: g, runner: source.NewRunner(), limits: source.Defaults()}
	if _, err := app.load(context.Background(), options{Command: "open", Checkout: r.Dir, Identity: g.value.Identity}, nil); err != nil {
		t.Fatal("explicit open", err)
	}
	canonical, err := canonicalPath(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.LookupRepository("owner/repo"); err != nil || got != canonical {
		t.Fatal("verified checkout was not remembered", got, err)
	}
	if _, err := app.load(context.Background(), options{Command: "open", Identity: g.value.Identity}, nil); err != nil {
		t.Fatal("cached open", err)
	}
	if err := store.RememberRepository("owner/repo", filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatal(err)
	}
	if _, err := app.load(context.Background(), options{Command: "open", Identity: g.value.Identity}, nil); err == nil || !strings.Contains(err.Error(), "pass --repo <checkout>") {
		t.Fatal("stale cached checkout lacked remediation", err)
	}
	if _, err := app.load(context.Background(), options{Command: "open", Checkout: r.Dir, Identity: g.value.Identity}, nil); err != nil {
		t.Fatal("explicit checkout did not override stale cache", err)
	}
}

func TestLifecycleCreatesDerivedGuideSession(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	app := application{store: store, gh: &fixtureGH{value: meta}, runner: source.NewRunner(), limits: source.Defaults()}
	original, err := app.open(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Mark(store, original, original.Slices[0].FileID, true); err != nil {
		t.Fatal(err)
	}
	derived, err := app.generateGuide(context.Background(), original, unavailableGuideAnalyzer{})
	if err != nil {
		t.Fatal(err)
	}
	if derived.ID == original.ID || derived.DerivedFrom != original.ID || len(derived.ReviewedSliceIDs) != 0 || derived.Guides == nil || derived.Guides.Status != guide.Unavailable {
		t.Fatal("guide action did not create an unread derived session", derived)
	}
	loaded, err := store.Load(original.ID)
	if err != nil || len(loaded.ReviewedSliceIDs) != 1 || loaded.DerivedFrom != "" {
		t.Fatal("guide action changed source session", loaded, err)
	}
}

func TestLifecyclePRListOpenCachesOnlyGeneratedGuides(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls := 0
	app := application{store: store, gh: &fixtureGH{value: meta}, runner: source.NewRunner(), limits: source.Defaults(), newAnalyzer: func() (guide.Analyzer, error) {
		calls++
		return generatedGuideAnalyzer{}, nil
	}}
	first, err := app.openFromPullRequestList(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil || calls != 0 {
		t.Fatalf("PR-list open unexpectedly generated a guide: calls=%d, err=%v", calls, err)
	}
	guided, err := app.generateGuide(context.Background(), first, generatedGuideAnalyzer{})
	if err != nil {
		t.Fatal(err)
	}
	guided.ReviewedSliceIDs = []string{guided.Slices[0].FileID}
	if err := store.Save(guided); err != nil {
		t.Fatal(err)
	}
	app.newAnalyzer = func() (guide.Analyzer, error) {
		calls++
		return nil, errors.New("cache hit must not create analyzer")
	}
	app.runner = requestFunc(func(context.Context, source.Request) ([]byte, error) {
		return nil, errors.New("source cache hit must not run git")
	})
	second, err := app.openFromPullRequestList(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil || second.Guides == nil || second.Guides.Status != guide.Generated || calls != 0 || len(second.ReviewedSliceIDs) != 1 {
		t.Fatalf("cached PR-list guide = %#v, calls=%d, err=%v", second, calls, err)
	}
}

func TestLifecyclePRListOpenShowsCachedSourceBeforeMetadataCheck(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gh := &fixtureGH{value: meta}
	app := application{store: store, gh: gh, runner: source.NewRunner(), limits: source.Defaults()}
	first, err := app.open(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}

	gh.err = errors.New("metadata check is still running")
	got, err := app.openFromPullRequestList(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil {
		t.Fatalf("cached source waited for metadata: %v", err)
	}
	if got.Inventory.Comparison.InventoryID != first.Inventory.Comparison.InventoryID || got.RevisionStatus != session.Unchecked {
		t.Fatalf("cached session = %#v, want frozen source with unchecked freshness", got)
	}
}

func TestLifecycleDirectOpenShowsCachedSourceBeforeMetadataCheck(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	gh := &fixtureGH{value: meta}
	app := application{store: store, gh: gh, runner: source.NewRunner(), limits: source.Defaults()}
	first, err := app.open(context.Background(), r.Dir, meta.Identity, nil)
	if err != nil {
		t.Fatal(err)
	}

	gh.err = errors.New("metadata check is still running")
	got, err := app.load(context.Background(), options{Command: "open", Checkout: r.Dir, Identity: meta.Identity}, nil)
	if err != nil {
		t.Fatalf("direct open waited for metadata: %v", err)
	}
	if got.Inventory.Comparison.InventoryID != first.Inventory.Comparison.InventoryID || got.RevisionStatus != session.Unchecked {
		t.Fatalf("cached session = %#v, want frozen source with unchecked freshness", got)
	}
}

func TestLifecyclePRListOpenRetriesUnavailableGuides(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	calls := 0
	app := application{store: store, gh: &fixtureGH{value: meta}, runner: source.NewRunner(), limits: source.Defaults(), newAnalyzer: func() (guide.Analyzer, error) {
		calls++
		return unavailableGuideAnalyzer{}, nil
	}}
	for attempt := 1; attempt <= 2; attempt++ {
		got, err := app.openFromPullRequestList(context.Background(), r.Dir, meta.Identity, nil)
		if err != nil || calls != 0 {
			t.Fatalf("attempt %d generated without consent: calls=%d, %v", attempt, calls, err)
		}
		_, err = app.generateGuide(context.Background(), got, unavailableGuideAnalyzer{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatalf("PR-list analyzer calls = %d, want none", calls)
	}
}

func TestBackgroundRefreshDoesNotMutateOrPersistOpenedState(t *testing.T) {
	app, saved := wiringFixture(t)
	before := saved.State
	if _, err := app.refreshOpenedPullRequest(context.Background(), tui.PullRequestRefreshRequest{Metadata: saved.Inventory.Comparison.Metadata, Checkout: string(saved.Checkout)}, nil); err != nil {
		t.Fatal(err)
	}
	if saved.RevisionStatus != before.RevisionStatus || saved.Generation != before.Generation {
		t.Fatal("background refresh mutated UI-owned state")
	}
	reopened, err := app.store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Generation != before.Generation {
		t.Fatal("background refresh persisted stale progress")
	}
}

func TestSubmitCommentAllowsDescriptionOnlyChange(t *testing.T) {
	app, saved := wiringFixture(t)
	frozen := saved.Inventory.Comparison.Metadata
	app.gh.(*fixtureGH).value.Description = "edited description"
	comment := source.ReviewComment{Target: source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "body"}
	if _, err := app.submitReviewComment(context.Background(), tui.CommentSubmission{Metadata: frozen, Comment: comment}); err != nil {
		t.Fatal(err)
	}
}

type actionFixtureGH struct {
	*fixtureGH
	comment source.ReviewComment
	writes  int
}

func (g *actionFixtureGH) Viewer(context.Context) (source.Viewer, error) {
	return source.Viewer{Login: g.comment.Author}, nil
}
func (g *actionFixtureGH) ReplyToReviewComment(context.Context, source.Identity, int64, string) (source.ReviewComment, error) {
	g.writes++
	return g.comment, nil
}
func (g *actionFixtureGH) DeleteReviewComment(context.Context, source.Identity, int64) error {
	g.writes++
	return nil
}
func (g *actionFixtureGH) AddReviewCommentReaction(context.Context, source.Identity, int64, string) (source.ReviewCommentReaction, error) {
	g.writes++
	return source.ReviewCommentReaction{}, nil
}

func TestCommentActionPreflightUsesPinnedRevision(t *testing.T) {
	app, saved := wiringFixture(t)
	frozen := saved.Inventory.Comparison.Metadata
	gh := &actionFixtureGH{fixtureGH: app.gh.(*fixtureGH), comment: source.ReviewComment{ID: 42, Author: "reviewer", Target: source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "existing"}}
	app.gh = gh
	for _, kind := range []string{"reply", "reaction", "delete"} {
		for _, change := range []string{"description", "identity", "base repository", "head repository", "base SHA", "head SHA"} {
			t.Run(kind+"/"+change, func(t *testing.T) {
				gh.value = frozen
				gh.writes = 0
				switch change {
				case "description":
					gh.value.Description = "edited"
				case "identity":
					gh.value.Identity.Number++
				case "base repository":
					gh.value.BaseRepository = "other/repo"
				case "head repository":
					gh.value.HeadRepository = "other/repo"
				case "base SHA":
					gh.value.BaseSHA = strings.Repeat("a", 40)
				case "head SHA":
					gh.value.HeadSHA = strings.Repeat("b", 40)
				}
				action := tui.CommentAction{Metadata: frozen, Comment: gh.comment}
				switch kind {
				case "reply":
					action.Body = "reply"
				case "reaction":
					action.Reaction = "+1"
				case "delete":
					action.Delete = true
				}
				_, _, err := app.submitReviewCommentAction(context.Background(), action)
				if change == "description" {
					if err != nil || gh.writes != 1 {
						t.Fatalf("description edit blocked action: %v (%d writes)", err, gh.writes)
					}
				} else if err == nil || gh.writes != 0 {
					t.Fatalf("revision mismatch allowed action: %v (%d writes)", err, gh.writes)
				}
			})
		}
	}
}

func TestBackgroundRefreshFailureReturnsFreshnessWithoutWriting(t *testing.T) {
	app, saved := wiringFixture(t)
	app.gh.(*fixtureGH).err = errors.New("metadata unavailable")
	request := tui.PullRequestRefreshRequest{Metadata: saved.Inventory.Comparison.Metadata, Checkout: string(saved.Checkout)}
	before := saved.Generation
	result, err := app.refreshOpenedPullRequest(context.Background(), request, nil)
	if err != nil || result.Status != session.CheckFailed || result.Session != nil {
		t.Fatalf("failure freshness = %#v, %v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := app.refreshOpenedPullRequest(ctx, request, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled refresh returned usable result", err)
	}
	reopened, err := app.store.Load(saved.ID)
	if err != nil || reopened.Generation != before {
		t.Fatal("worker wrote existing state", err)
	}
}
