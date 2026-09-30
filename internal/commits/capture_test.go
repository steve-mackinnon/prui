package commits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"prui/internal/inventory"
	"prui/internal/source"
	"prui/internal/testutil"
	"strings"
	"testing"
	"time"
)

type fakeGH struct {
	metadata source.Metadata
	entries  []source.PullRequestCommit
	err      error
}

func (g fakeGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.metadata, nil
}
func (g fakeGH) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	return nil, nil
}
func (g fakeGH) Token(context.Context) (string, error) { return "", nil }
func (g fakeGH) ListPullRequestCommits(context.Context, source.Identity) ([]source.PullRequestCommit, error) {
	return g.entries, g.err
}

func TestCaptureOwnDiffRootEmptyAndMissing(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "first\n")
	root := r.Commit()
	r.Write("a", "second\n")
	head := r.Commit()
	empty := r.Git("commit-tree", r.Git("rev-parse", head+"^{tree}"), "-p", head, "-m", "empty")
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseSHA: root, HeadSHA: empty}
	g := fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: root}, {SHA: head}, {SHA: empty}, {SHA: "0123456789abcdef0123456789abcdef01234567"}}}
	bundle, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, g, source.Defaults(), nil)
	if err != nil || bundle.Status != Captured || len(bundle.Entries) != 4 || !bundle.Complete {
		t.Fatal(bundle, err)
	}
	if len(bundle.Entries[0].Parents) != 0 || len(bundle.Entries[2].Diff.Files) != 0 || bundle.Entries[3].Status != Unavailable {
		t.Fatal(bundle.Entries)
	}
	var material []byte
	for _, patch := range bundle.Entries[1].Diff.Patches {
		material = append(material, patch...)
	}
	if !bytes.Contains(material, []byte("-first\n+second")) {
		t.Fatal(string(material))
	}
}

func TestCaptureUnavailableCancellationAndRevisionChange(t *testing.T) {
	m := source.Metadata{BaseSHA: "a", HeadSHA: "b"}
	b, err := Capture(context.Background(), nil, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, err: errors.New("secret command output")}, source.Defaults(), nil)
	if err != nil || b.Status != Unavailable || b.Reason == "secret command output" {
		t.Fatal(b, err)
	}
	g := fakeGH{metadata: source.Metadata{BaseSHA: "changed"}}
	if _, err := Capture(context.Background(), nil, source.PinnedComparison{Metadata: m}, g, source.Defaults(), nil); !errors.Is(err, ErrRevisionChanged) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Capture(ctx, nil, source.PinnedComparison{}, g, source.Defaults(), nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestCaptureMergeFirstParentAndUnreachable(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	base := r.Commit()
	r.Write("a", "first parent\n")
	first := r.Commit()
	r.Write("a", "second parent\n")
	second := r.Commit()
	merge := r.Git("commit-tree", r.Git("rev-parse", second+"^{tree}"), "-p", first, "-p", second, "-m", "merge")
	unreachable := r.Git("commit-tree", r.Git("rev-parse", base+"^{tree}"), "-m", "unreachable")
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{HeadSHA: merge, BaseSHA: base}
	b, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: merge}, {SHA: unreachable}}}, source.Defaults(), nil)
	if err != nil || len(b.Entries[0].Parents) != 2 || b.Entries[0].Parents[0] != first || b.Entries[1].Status != Unavailable {
		t.Fatal(b, err)
	}
	var patch []byte
	for _, p := range b.Entries[0].Diff.Patches {
		patch = append(patch, p...)
	}
	if !bytes.Contains(patch, []byte("-first parent\n+second parent")) {
		t.Fatal(string(patch))
	}
}

func TestCaptureAggregateBudgetAndEmptyMembership(t *testing.T) {
	m := source.Metadata{BaseSHA: "a", HeadSHA: "b"}
	empty, err := Capture(context.Background(), nil, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m}, source.Defaults(), nil)
	if err != nil || empty.Status != Captured || !empty.Complete || len(empty.Entries) != 0 {
		t.Fatal(empty, err)
	}
	entries := make([]source.PullRequestCommit, 100)
	for i := range entries {
		entries[i].SHA = fmt.Sprintf("%040x", i)
	}
	l := source.Defaults()
	l.ContentBytes = 1
	b, err := Capture(context.Background(), nil, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: entries}, l, nil)
	if err != nil || b.Complete || len(b.Entries) != 100 {
		t.Fatal(b, err)
	}
	for _, e := range b.Entries {
		if e.Status != Unavailable || e.Diff != nil || e.Reason != "Commit capture budget exhausted." {
			t.Fatal(e)
		}
	}
}

type deadlineGH struct {
	fakeGH
	fresh bool
}

func (g *deadlineGH) ListPullRequestCommits(ctx context.Context, _ source.Identity) ([]source.PullRequestCommit, error) {
	// Simulate the response arriving as the allotted work window expires.
	<-ctx.Done()
	return g.entries, nil
}
func (g *deadlineGH) Metadata(ctx context.Context, _ source.Identity) (source.Metadata, error) {
	g.fresh = ctx.Err() == nil
	return g.metadata, ctx.Err()
}
func TestCaptureWorkDeadlinePreservesMembershipWithFreshnessReserve(t *testing.T) {
	m := source.Metadata{BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	g := &deadlineGH{fakeGH: fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: m.HeadSHA}}}}
	b, err := capture(context.Background(), nil, source.PinnedComparison{Metadata: m}, g, source.Defaults(), nil, time.Nanosecond, time.Second)
	if err != nil || !g.fresh || b.Status != Captured || !b.Complete || len(b.Entries) != 1 || b.Entries[0].Status != Unavailable || b.Entries[0].Reason != "Commit capture budget exhausted." {
		t.Fatal(b, err, g.fresh)
	}
}

func TestCapturePreservesRenameBinaryModeAndSubmoduleKinds(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("old", "AAAAAAAAAAAAAAAAAAA\nBBBBBBBBBBBBBBBBBBB\nCCCCCCCCCCCCCCCCCCC\nDDDDDDDDDDDDDDDDDDD\n")
	r.Write("binary", "before\x00binary")
	r.Write("mode", "same\n")
	base := r.Commit()
	r.Git("mv", "old", "new")
	r.Write("new", "AAAAAAAAAAAAAAAAAAA\nBBBBBBBBBBBBBBBBBBB\nEEEEEEEEEEEEEEEEEEE\nFFFFFFFFFFFFFFFFFFF\n")
	r.Write("binary", "after\x00binary")
	changed := r.Commit()
	treeRecords := r.Git("ls-tree", "-z", changed)
	treeRecords = strings.Replace(treeRecords, "100644 blob "+r.Git("rev-parse", changed+":mode"), "100755 blob "+r.Git("rev-parse", changed+":mode"), 1)
	tree := r.GitInput(treeRecords+"160000 commit "+base+"\tsubmodule\x00", "mktree", "-z")
	head := r.Git("commit-tree", tree, "-p", base, "-m", "special kinds")
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{BaseSHA: base, HeadSHA: head}
	b, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: head}}}, source.Defaults(), nil)
	if err != nil || b.Entries[0].Status != Captured || !b.Entries[0].Diff.Complete {
		t.Fatal(b, err)
	}
	diff := b.Entries[0].Diff
	renamed, mode := false, false
	for _, f := range diff.Files {
		if f.Status == "R050" && string(f.OldPath) == "old" && string(f.NewPath) == "new" {
			renamed = true
		}
		if string(f.NewPath) == "mode" && f.OldMode == "100644" && f.NewMode == "100755" {
			mode = true
		}
	}
	kinds := map[inventory.Kind]bool{}
	for _, u := range diff.Units {
		kinds[u.Kind] = true
	}
	if !renamed || !mode || !kinds[inventory.Binary] || !kinds[inventory.Gitlink] || !kinds[inventory.TextHunk] {
		t.Fatal(diff, renamed, mode, kinds)
	}
}
