package review

import (
	"context"
	"errors"
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

type commitGitHub struct {
	FixtureGitHub
	items     []source.PullRequestCommit
	err       error
	listCalls int
	moves     []string
	cancel    context.CancelFunc
}

func (g *commitGitHub) ListPullRequestCommits(context.Context, source.Identity) ([]source.PullRequestCommit, error) {
	g.listCalls++
	items := g.items
	if len(g.moves) > 0 {
		items = []source.PullRequestCommit{{SHA: g.Value.HeadSHA, Subject: "Before movement", Author: "Fixture"}}
		g.Value.HeadSHA = g.moves[0]
		g.moves = g.moves[1:]
	}
	if g.cancel != nil {
		g.cancel()
	}
	return items, g.err
}

func commitReviewFixture(t *testing.T) (*testutil.Repo, source.Metadata, []source.PullRequestCommit) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("a", "before\n")
	base := r.Commit()
	r.Write("a", "middle\n")
	first := r.Commit()
	r.Write("a", "after\n")
	head := r.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	return r, m, []source.PullRequestCommit{{SHA: first, Subject: "First change", Author: "Fixture"}, {SHA: head, Subject: "Second change", Author: "Fixture"}}
}

func TestCommitCaptureSurvivesOfflineResumeWithoutCheckout(t *testing.T) {
	r, meta, items := commitReviewFixture(t)
	before := r.Snapshot()
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, items: items}
	s, err := Open(context.Background(), r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Commits == nil || s.Commits.Status != commits.Captured || len(s.Commits.Entries) != 2 || gh.listCalls != 1 {
		t.Fatalf("commit capture missing: %+v", s.Commits)
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("commit capture changed checkout")
	}
	if len(s.Commits.Entries[1].Parents) != 1 || s.Commits.Entries[1].Parents[0] != items[0].SHA {
		t.Fatal("second commit did not use its first parent")
	}
	patches := s.Commits.Entries[1].Diff.Patches
	var patch string
	for _, raw := range patches {
		patch += string(raw)
	}
	if !strings.Contains(patch, "-middle") || !strings.Contains(patch, "+after") || strings.Contains(patch, "-before") {
		t.Fatalf("wrong individual diff: %s", patch)
	}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.Create(s.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(r.Dir); err != nil {
		t.Fatal(err)
	}
	resumed, err := Resume(context.Background(), store, saved.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Commits, s.Commits) {
		t.Fatal("offline resume lost frozen commits")
	}
	derived := DeriveGuide(context.Background(), resumed, &fakeAnalyzer{}, Config{})
	if !reflect.DeepEqual(derived.Commits, resumed.Commits) {
		t.Fatal("guide derivation lost commit material")
	}
}

func TestCommitCaptureFailurePreservesMainDiff(t *testing.T) {
	r, meta, _ := commitReviewFixture(t)
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, err: errors.New("sensitive raw failure")}
	s, err := Open(context.Background(), r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Inventory.Units) == 0 || s.Commits == nil || s.Commits.Status != commits.Unavailable {
		t.Fatal("optional failure erased review or unavailable status")
	}
	if strings.Contains(s.Commits.Reason, "sensitive") {
		t.Fatal("persisted raw error diagnostic")
	}
}

func TestCommitCaptureRetriesMovedRevision(t *testing.T) {
	r, meta, items := commitReviewFixture(t)
	r.Write("a", "latest\n")
	latest := r.Commit()
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, items: []source.PullRequestCommit{{SHA: latest, Subject: "Latest", Author: "Fixture"}}, moves: []string{latest}}
	s, err := Open(context.Background(), r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if gh.listCalls != 2 || s.Inventory.Comparison.Metadata.HeadSHA != latest || s.Commits.HeadSHA != latest || s.Commits.Entries[0].SHA != latest {
		t.Fatal("retry attached wrong revision")
	}
	if len(s.Commits.Entries[0].Parents) != 1 || s.Commits.Entries[0].Parents[0] != items[1].SHA {
		t.Fatal("retry comparison parent wrong")
	}
}

func TestCommitCaptureRepeatedRevisionMovementFails(t *testing.T) {
	r, meta, _ := commitReviewFixture(t)
	r.Write("a", "latest\n")
	latest := r.Commit()
	r.Write("a", "newest\n")
	newest := r.Commit()
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, moves: []string{latest, newest}}
	s, err := Open(context.Background(), r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err == nil || s != nil || gh.listCalls != 2 {
		t.Fatal("repeated movement saved a mixed snapshot")
	}
}

func TestCommitCaptureCancellationDoesNotOpenReview(t *testing.T) {
	r, meta, items := commitReviewFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{meta}, items: items, cancel: cancel}
	s, err := Open(ctx, r.Dir, meta.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if !errors.Is(err, context.Canceled) || s != nil {
		t.Fatal("cancelled commit capture opened a review", err)
	}
}

func TestCommitCaptureExistsForEmptyNetComparison(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "before\n")
	base := r.Commit()
	r.Write("a", "temporary\n")
	first := r.Commit()
	r.Write("a", "before\n")
	head := r.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	gh := &commitGitHub{FixtureGitHub: FixtureGitHub{m}, items: []source.PullRequestCommit{{SHA: first, Subject: "Temporary", Author: "Fixture"}, {SHA: head, Subject: "Revert", Author: "Fixture"}}}
	s, err := Open(context.Background(), r.Dir, m.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Inventory.Units) != 0 || s.Commits == nil || len(s.Commits.Entries) != 2 || len(s.Commits.Entries[0].Diff.Units) == 0 {
		t.Fatal("empty net PR diff hid commit changes")
	}
}
