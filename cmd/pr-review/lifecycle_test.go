package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

type fixtureGH struct {
	value source.Metadata
	err   error
}

func (g *fixtureGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.value, g.err
}
func (g *fixtureGH) Token(context.Context) (string, error) { panic("credentials must not be used") }

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
		if code := run(args); code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
	}
	if _, err := os.Stat(filepath.Join(path, saved.ID)); !os.IsNotExist(err) {
		t.Fatal("CLI deletion incomplete")
	}
}
