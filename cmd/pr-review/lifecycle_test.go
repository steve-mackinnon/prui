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
)

type fixtureGH struct {
	value source.Metadata
	err   error
	prs   []source.PullRequest
}

type unavailableGuideAnalyzer struct{}

func (unavailableGuideAnalyzer) Analyze(context.Context, guide.Input) (guide.Bundle, error) {
	return guide.Bundle{}, errors.New("provider unavailable")
}

func (g *fixtureGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.value, g.err
}
func (g *fixtureGH) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	return g.prs, g.err
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
