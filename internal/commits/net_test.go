package commits

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/testutil"
	"strings"
	"testing"
)

func netBundle(t *testing.T, r *testutil.Repo, base string, shas ...string) *Bundle {
	t.Helper()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseSHA: base, HeadSHA: shas[len(shas)-1]}
	entries := make([]source.PullRequestCommit, len(shas))
	for i, s := range shas {
		entries[i].SHA = s
	}
	b, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: entries}, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestComposeCombinesSelectedAndExcludesSkipped(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("a", "one\n")
	a := r.Commit()
	r.Write("skip", "excluded\n")
	b := r.Commit()
	r.Write("a", "final\n")
	c := r.Commit()
	bundle := netBundle(t, r, base, a, b, c)
	inv, err := Compose(context.Background(), bundle, []string{c, a}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Files) != 1 || string(inv.Files[0].NewPath) != "a" {
		t.Fatal(inv.Files)
	}
	var patch strings.Builder
	for _, p := range inv.Patches {
		patch.Write(p)
	}
	if !strings.Contains(patch.String(), "-base\n+final\n") {
		t.Fatal(patch.String())
	}
}

func TestComposeCancellationConflictAndLimits(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("x", "one\n")
	a := r.Commit()
	r.Write("a", "dependency\n")
	b := r.Commit()
	r.Write("a", "final\n")
	c := r.Commit()
	bundle := netBundle(t, r, base, a, b, c)
	if _, err := Compose(context.Background(), bundle, []string{a, c}, source.Defaults()); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatal(err)
	}
	inv, err := Compose(context.Background(), bundle, []string{b, c}, source.Defaults())
	if err != nil || len(inv.Files) != 1 {
		t.Fatal(inv, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compose(ctx, bundle, []string{a}, source.Defaults()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	l := source.Defaults()
	l.BlobBytes = 1
	if _, err := Compose(context.Background(), bundle, []string{a}, l); !errors.Is(err, source.ErrLimit) {
		t.Fatal(err)
	}
}
func TestComposeRevertDisappears(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("a", "changed\n")
	a := r.Commit()
	r.Write("a", "base\n")
	b := r.Commit()
	bundle := netBundle(t, r, base, a, b)
	inv, err := Compose(context.Background(), bundle, []string{a, b}, source.Defaults())
	if err != nil || len(inv.Files) != 0 || !inv.Complete {
		t.Fatal(inv, err)
	}
}
func TestComposeRootRenameModeBinaryAndDelete(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("dir/a", "root\n")
	root := r.Commit()
	r.Git("mv", "dir/a", "renamed")
	rename := r.Commit()
	if err := os.Chmod(filepath.Join(r.Dir, "renamed"), 0755); err != nil {
		t.Fatal(err)
	}
	r.Git("update-index", "--chmod=+x", "renamed")
	r.Git("commit", "-m", "mode")
	mode := r.Git("rev-parse", "HEAD")
	r.Write("binary", "a\x00b")
	binary := r.Commit()
	r.Git("rm", "renamed")
	deleted := r.Commit()
	bundle := netBundle(t, r, root, root, rename, mode, binary, deleted)
	inv, err := Compose(context.Background(), bundle, []string{root, rename, mode, binary}, source.Defaults())
	if err != nil || len(inv.Files) != 2 {
		t.Fatal(inv, err)
	}
	foundMode, foundBinary := false, false
	for _, f := range inv.Files {
		if string(f.NewPath) == "renamed" && f.NewMode == "100755" {
			foundMode = true
		}
	}
	for _, u := range inv.Units {
		if u.Kind == inventory.Binary {
			foundBinary = true
		}
	}
	if !foundMode || !foundBinary {
		t.Fatal(inv)
	}
	if err = os.RemoveAll(filepath.Join(r.Dir, ".git", "objects")); err != nil {
		t.Fatal(err)
	}
	inv, err = Compose(context.Background(), bundle, []string{root, rename, mode, binary, deleted}, source.Defaults())
	if err != nil || len(inv.Files) != 1 || string(inv.Files[0].NewPath) != "binary" {
		t.Fatal(inv, err)
	}
}

func TestComposeMergeUsesFirstParentAndRejectsBranches(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("a", "first\n")
	first := r.Commit()
	r.Write("a", "second\n")
	second := r.Commit()
	merge := r.Git("commit-tree", r.Git("rev-parse", second+"^{tree}"), "-p", first, "-p", second, "-m", "merge")
	bundle := netBundle(t, r, base, first, second, merge)
	inv, err := Compose(context.Background(), bundle, []string{merge}, source.Defaults())
	if err != nil || len(inv.Files) != 1 {
		t.Fatal(inv, err)
	}
	var patch strings.Builder
	for _, p := range inv.Patches {
		patch.Write(p)
	}
	if !strings.Contains(patch.String(), "-first\n+second\n") {
		t.Fatal(patch.String())
	}
	if _, err = Compose(context.Background(), bundle, []string{second, merge}, source.Defaults()); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatal(err)
	}
}
func TestComposeEmptySelectionLegacyAndDiffLineLimit(t *testing.T) {
	inv, err := Compose(context.Background(), nil, nil, source.Defaults())
	if err != nil || len(inv.Files) != 0 || !inv.Complete {
		t.Fatal(inv, err)
	}
	if _, err = Compose(context.Background(), &Bundle{}, []string{"missing"}, source.Defaults()); err == nil {
		t.Fatal("legacy source accepted")
	}
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("a", "final\n")
	head := r.Commit()
	bundle := netBundle(t, r, base, head)
	l := source.Defaults()
	l.DiffLines = 1
	if _, err = Compose(context.Background(), bundle, []string{head}, l); !errors.Is(err, source.ErrLimit) {
		t.Fatal(err)
	}
}

func TestComposeGitlinkMetadataAndEmptyCommit(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Git("update-index", "--add", "--cacheinfo", "160000,"+base+",sub")
	r.Git("commit", "-m", "gitlink")
	head := r.Git("rev-parse", "HEAD")
	empty := r.Git("commit-tree", r.Git("rev-parse", head+"^{tree}"), "-p", head, "-m", "empty")
	bundle := netBundle(t, r, base, head, empty)
	inv, err := Compose(context.Background(), bundle, []string{head, empty}, source.Defaults())
	if err != nil || len(inv.Files) != 1 || inv.Files[0].NewMode != "160000" {
		t.Fatal(inv, err)
	}
	found := false
	for _, u := range inv.Units {
		if u.Kind == inventory.Gitlink {
			found = true
		}
	}
	if !found {
		t.Fatal(inv.Units)
	}
	inv, err = Compose(context.Background(), bundle, []string{empty}, source.Defaults())
	if err != nil || len(inv.Files) != 0 {
		t.Fatal(inv, err)
	}
}

func TestComposeSkippedModeChangeIsExcluded(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("x", "one\n")
	a := r.Commit()
	if err := os.Chmod(filepath.Join(r.Dir, "a"), 0755); err != nil {
		t.Fatal(err)
	}
	b := r.Commit()
	r.Write("a", "final\n")
	c := r.Commit()
	bundle := netBundle(t, r, base, a, b, c)
	inv, err := Compose(context.Background(), bundle, []string{a, c}, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range inv.Files {
		if string(f.NewPath) == "a" && f.NewMode != "100644" {
			t.Fatalf("excluded executable mode leaked: %+v", f)
		}
	}
}

func TestComposeSharesMaterializationBudgetAcrossPhases(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", strings.Repeat("old content\n", 30))
	base := r.Commit()
	r.Write("a", strings.Repeat("new content\n", 30))
	head := r.Commit()
	bundle := netBundle(t, r, base, head)
	// Every blob, and the generated patch, individually fit. Their cumulative
	// source reads and resulting inventory must share one operation budget.
	limits := source.Defaults()
	limits.ContentBytes = 2400
	if _, err := Compose(context.Background(), bundle, []string{head}, limits); !errors.Is(err, source.ErrLimit) {
		t.Fatalf("combined materialization must exceed budget: %v", err)
	}
}
