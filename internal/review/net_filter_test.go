package review

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"prui/internal/commits"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

func TestNetCommitFilterSurvivesOfflineRestartWithoutCheckout(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("first.txt", "first before\n")
	r.Write("skipped.txt", "skipped before\n")
	r.Write("last.txt", "last before\n")
	base := r.Commit()
	r.Write("first.txt", "first selected\n")
	first := r.Commit()
	r.Write("skipped.txt", "skipped change\n")
	skipped := r.Commit()
	r.Write("last.txt", "last selected\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, items: []source.PullRequestCommit{{SHA: first}, {SHA: skipped}, {SHA: head}}}
	before := r.Snapshot()
	s, err := Open(context.Background(), r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("capturing filter material changed checkout")
	}
	storePath := filepath.Join(t.TempDir(), "sessions")
	store, err := session.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(s.Snapshot)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(r.Dir); err != nil {
		t.Fatal(err)
	}
	store, err = session.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	resumed, err := Resume(context.Background(), store, saved.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := json.Marshal(resumed)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := commits.Compose(context.Background(), resumed.Commits, []string{head, first}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Complete || len(inv.Files) != 2 {
		t.Fatalf("expected one complete net inventory with two files: %+v", inv)
	}
	paths := map[string]bool{}
	for _, file := range inv.Files {
		paths[string(file.NewPath)] = true
	}
	if !paths["first.txt"] || !paths["last.txt"] || paths["skipped.txt"] {
		t.Fatalf("selected changes are wrong: %v", paths)
	}
	var patches strings.Builder
	for _, patch := range inv.Patches {
		patches.Write(patch)
	}
	if !strings.Contains(patches.String(), "+first selected") || !strings.Contains(patches.String(), "+last selected") || strings.Contains(patches.String(), "skipped change") {
		t.Fatalf("unexpected combined content: %s", patches.String())
	}
	after, err := json.Marshal(resumed)
	if err != nil || string(after) != string(frozen) {
		t.Fatal("composition changed resumed source/progress", err)
	}
	if gh.listCalls != 1 {
		t.Fatal("offline filtering requested GitHub")
	}
}
