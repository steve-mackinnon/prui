package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

func fixture(t *testing.T) (*source.View, source.PinnedComparison, *testutil.Repo) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("old.txt", "same content\n")
	r.Write("mode", "mode\n")
	r.Write("text", "old\n")
	r.Write("binary", "a\x00b")
	r.Write("type", "regular\n")
	base := r.Commit()
	r.Git("mv", "old.txt", "new.txt")
	r.Git("update-index", "--chmod=+x", "mode")
	if e := os.Chmod(filepath.Join(r.Dir, "mode"), 0700); e != nil {
		t.Fatal(e)
	}
	r.Write("text", "new\n")
	r.Write("binary", "a\x00c")
	if e := os.Remove(filepath.Join(r.Dir, "type")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink("/outside/never-read", filepath.Join(r.Dir, "type")); e != nil {
		t.Fatal(e)
	}
	head := r.Commit()
	blob := r.Git("rev-parse", head+":text")
	tree := r.GitInput(r.Git("ls-tree", "-z", head)+"100644 blob "+blob+"\tbad\xff\x1b\nname\x00"+"160000 commit "+base+"\tsubmodule\x00", "mktree", "-z")
	head = r.Git("commit-tree", tree, "-p", head, "-m", "byte-path fixture")
	r.Write("text", "dirty source must not appear\n")
	v, e := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = v.Close() })
	return v, source.PinnedComparison{Metadata: source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: base, HeadSHA: head}, MergeBaseSHA: base}, r
}

func TestInventoryKindsAndLosslessPaths(t *testing.T) {
	v, p, r := fixture(t)
	before := r.Snapshot()
	inv, e := Build(context.Background(), v, p, source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	if !inv.Complete || len(inv.Files) != 7 {
		t.Fatalf("inventory: %+v", inv)
	}
	kinds := map[Kind]bool{}
	seen := map[string]bool{}
	renamed := false
	badPath := false
	for _, f := range inv.Files {
		if string(f.OldPath) == "old.txt" && string(f.NewPath) == "new.txt" {
			renamed = true
		}
		if bytes.Contains(f.NewPath, []byte{0xff}) {
			badPath = true
		}
	}
	for _, u := range inv.Units {
		kinds[u.Kind] = true
		if seen[u.ID] {
			t.Fatal("duplicate unit")
		}
		seen[u.ID] = true
		if bytes.Contains(inv.Patches[u.PatchReference], []byte("dirty source")) {
			t.Fatal("working tree leaked")
		}
	}
	if !renamed || !badPath || !kinds[TextHunk] || !kinds[Binary] || !kinds[Gitlink] || !kinds[FileMetadata] {
		t.Fatalf("missing kinds/path: %v %v %v", kinds, renamed, badPath)
	}
	encoded, e := json.Marshal(inv)
	if e != nil {
		t.Fatal(e)
	}
	var round Inventory
	if e = json.Unmarshal(encoded, &round); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(inv.Files, round.Files) {
		t.Fatal("path bytes lost")
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("checkout mutated")
	}
	again, e := Build(context.Background(), v, p, source.Defaults())
	if e != nil || !reflect.DeepEqual(inv, again) {
		t.Fatal("nondeterministic inventory", e)
	}
}

func TestInventoryLimits(t *testing.T) {
	v, p, _ := fixture(t)
	l := source.Defaults()
	l.BlobBytes = 1
	inv, e := Build(context.Background(), v, p, l)
	if e != nil {
		t.Fatal(e)
	}
	if inv.Complete || len(inv.Files) != 7 || len(inv.Problems) == 0 {
		t.Fatal("blob limit hid changes")
	}
	l = source.Defaults()
	l.Entries = 1
	if _, e = Build(context.Background(), v, p, l); e == nil {
		t.Fatal("entry limit accepted")
	}
	l = source.Defaults()
	l.DiffLines = 1
	inv, e = Build(context.Background(), v, p, l)
	if e != nil || inv.Complete {
		t.Fatal("line limit accepted", e)
	}
	l = source.Defaults()
	l.ContentBytes = 10
	inv, e = Build(context.Background(), v, p, l)
	if e != nil || inv.Complete {
		t.Fatal("content limit accepted", e)
	}
}

func TestInventoryRawParser(t *testing.T) {
	raw := []byte(":100644 100644 " + strings.Repeat("a", 40) + " " + strings.Repeat("b", 40) + " R100\x00old\xff\x00new\n\x00")
	f, e := parseRaw(raw, 100)
	if e != nil || len(f) != 1 || string(f[0].OldPath) != "old\xff" || string(f[0].NewPath) != "new\n" {
		t.Fatal(f, e)
	}
	for _, b := range [][]byte{raw[:len(raw)-1], []byte("garbage\x00"), append(raw, []byte("oops")...)} {
		if _, e := parseRaw(b, 100); e == nil {
			t.Fatal("malformed raw accepted")
		}
	}
}

func FuzzRaw(f *testing.F) {
	f.Add([]byte("invalid"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseRaw(b, 10000) })
}
