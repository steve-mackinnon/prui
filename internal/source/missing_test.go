package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pr-review/internal/testutil"
)

func TestPinnedMissingBlobFetch(t *testing.T) {
	remote := testutil.NewRepo(t)
	remote.Write("f", "before\n")
	base := remote.Commit()
	remote.Write("f", "after\n")
	head := remote.Commit()
	local := testutil.NewRepo(t)
	local.Git("fetch", "file://"+remote.Dir, head)
	// Unpack into loose objects so removing one blob models a partial object store.
	blob := remote.Git("rev-parse", head+":f")
	local.GitInput(remote.GitInput("", "cat-file", "blob", blob)+"\n", "hash-object", "-w", "--stdin")
	// Build a loose-only checkout directly from the synthetic remote.
	if e := os.RemoveAll(filepath.Join(local.Dir, ".git/objects")); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(local.Dir, ".git/objects"), 0700); e != nil {
		t.Fatal(e)
	}
	runner := &fixtureRunner{t: t, remote: remote.Dir}
	_, e := runner.Run(context.Background(), Request{Dir: filepath.Join(local.Dir, ".git"), Args: []string{"fetch", "https://github.com/o/r.git", "--no-auto-maintenance", "--recurse-submodules=no"}, Env: gitEnvironment(t.TempDir(), "synthetic-token"), StorageLimit: 512 << 20})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(filepath.Join(local.Dir, ".git/objects", blob[:2], blob[2:])); e != nil {
		t.Fatal(e)
	}
	g := &fakeGH{values: []Metadata{metadata(base, head)}}
	v, _, e := Pin(context.Background(), local.Dir, g.values[0].Identity, g, runner, Defaults(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if g.tokens != 1 {
		t.Fatal("missing blob did not trigger explicit fetch")
	}
	if _, e = v.Blob(context.Background(), blob, 100); e != nil {
		t.Fatal(e)
	}
}

func TestPinnedSuccessfulRaceRetry(t *testing.T) {
	remote := testutil.NewRepo(t)
	base := remote.Commit()
	remote.Write("f", "one\n")
	head := remote.Commit()
	remote.Write("f", "two\n")
	newer := remote.Commit()
	local := testutil.NewRepo(t)
	g := &fakeGH{values: []Metadata{metadata(base, head), metadata(base, newer)}}
	runner := &fixtureRunner{t: t, remote: remote.Dir}
	v, p, e := Pin(context.Background(), local.Dir, g.values[0].Identity, g, runner, Defaults(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if p.Metadata.HeadSHA != newer || g.calls != 3 || g.tokens != 2 {
		t.Fatal("race did not refetch and repin", p, g.calls, g.tokens)
	}
}

func TestSafetyFastStorageOverrun(t *testing.T) {
	dir := t.TempDir()
	_, e := NewRunner().Run(context.Background(), Request{Program: "/bin/sh", Args: []string{"-c", "printf 1234567890 > data"}, Dir: dir, Limit: 100, Timeout: time.Second, StorageDir: dir, StorageLimit: 1})
	if !errors.Is(e, ErrLimit) {
		t.Fatal("fast storage overrun escaped monitor", e)
	}
}
