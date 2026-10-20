package review

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"prui/internal/commits"
	"prui/internal/source"
)

type pinMembershipGH struct {
	commitGitHub
	started, joined  chan struct{}
	metadataReads    atomic.Int32
	waitCancellation bool
	listError        error
}

func (g *pinMembershipGH) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	if g.metadataReads.Add(1) > 1 {
		select {
		case <-g.joined:
		default:
			return source.Metadata{}, errors.New("freshness before membership joined")
		}
	}
	return g.FixtureGitHub.Metadata(ctx, id)
}
func (g *pinMembershipGH) ListPullRequestCommits(ctx context.Context, id source.Identity) ([]source.PullRequestCommit, error) {
	if g.metadataReads.Load() == 0 {
		return nil, errors.New("membership before metadata")
	}
	close(g.started)
	defer close(g.joined)
	if g.waitCancellation {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if g.listError != nil {
		g.listCalls++
		return nil, g.listError
	}
	return g.commitGitHub.ListPullRequestCommits(ctx, id)
}

type pinMembershipRunner struct {
	source.Runner
	started <-chan struct{}
	once    sync.Once
	failure error
	view    string
}

func (r *pinMembershipRunner) MaxParallelism() int { return 8 }
func (r *pinMembershipRunner) Run(ctx context.Context, q source.Request) ([]byte, error) {
	r.once.Do(func() {
		r.view = q.Dir
		select {
		case <-r.started:
		case <-ctx.Done():
		}
	})
	if r.failure != nil {
		return nil, r.failure
	}
	return r.Runner.Run(ctx, q)
}

func TestMembershipStartsBeforeAncestryValidation(t *testing.T) {
	repo, metadata, items := commitReviewFixture(t)
	gh := &pinMembershipGH{commitGitHub: commitGitHub{FixtureGitHub: FixtureGitHub{metadata}, items: items}, started: make(chan struct{}), joined: make(chan struct{})}
	runner := &pinMembershipRunner{Runner: source.NewRunner(), started: gh.started}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Open(ctx, repo.Dir, metadata.Identity, gh, runner, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Inventory.Complete || got.Commits == nil || !got.Commits.Complete || got.Commits.Composition.Status != commits.Captured || gh.listCalls != 1 || gh.metadataReads.Load() != 2 {
		t.Fatal("capture/checks omitted or membership retried")
	}
	select {
	case <-gh.joined:
	default:
		t.Fatal("membership not joined")
	}
}

func TestPinFailureCancelsAndJoinsMembership(t *testing.T) {
	repo, metadata, items := commitReviewFixture(t)
	gh := &pinMembershipGH{commitGitHub: commitGitHub{FixtureGitHub: FixtureGitHub{metadata}, items: items}, started: make(chan struct{}), joined: make(chan struct{}), waitCancellation: true}
	runner := &pinMembershipRunner{Runner: source.NewRunner(), started: gh.started, failure: source.ErrLimit}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := Open(ctx, repo.Dir, metadata.Identity, gh, runner, source.Defaults(), nil); !errors.Is(err, source.ErrLimit) {
		t.Fatal(err)
	}
	select {
	case <-gh.joined:
	default:
		t.Fatal("failed pin left membership running")
	}
	if gh.metadataReads.Load() != 1 {
		t.Fatal("failure performed extra freshness check")
	}
	if _, err := os.Stat(runner.view); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed view retained")
	}
}

func TestEarlyMembershipFailureIsNotRetried(t *testing.T) {
	repo, metadata, items := commitReviewFixture(t)
	gh := &pinMembershipGH{commitGitHub: commitGitHub{FixtureGitHub: FixtureGitHub{metadata}, items: items}, started: make(chan struct{}), joined: make(chan struct{}), listError: source.ErrLimit}
	runner := &pinMembershipRunner{Runner: source.NewRunner(), started: gh.started}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Open(ctx, repo.Dir, metadata.Identity, gh, runner, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Inventory.Complete || got.Commits == nil || got.Commits.Status != commits.Unavailable || gh.listCalls != 1 || gh.metadataReads.Load() != 2 {
		t.Fatal("failed membership retried, freshness omitted, or main inventory lost")
	}
}

type movingPinMembershipGH struct {
	FixtureGitHub
	items                    []source.PullRequestCommit
	started, joined          chan struct{}
	metadataReads, listCalls atomic.Int32
}

func (g *movingPinMembershipGH) Metadata(ctx context.Context, _ source.Identity) (source.Metadata, error) {
	m := g.Value
	if g.metadataReads.Add(1) == 1 {
		m.HeadSHA = "ffffffffffffffffffffffffffffffffffffffff"
	} else {
		select {
		case <-g.started:
		case <-ctx.Done():
			return source.Metadata{}, ctx.Err()
		}
	}
	return m, nil
}
func (g *movingPinMembershipGH) Token(context.Context) (string, error) {
	// The missing first head triggers pin's fetch barrier, without real network.
	return "", source.ErrCommand
}
func (g *movingPinMembershipGH) ListPullRequestCommits(ctx context.Context, _ source.Identity) ([]source.PullRequestCommit, error) {
	if g.listCalls.Add(1) == 1 {
		close(g.started)
		<-ctx.Done()
		close(g.joined)
		return nil, ctx.Err()
	}
	select {
	case <-g.joined:
		return g.items, nil
	default:
		return nil, errors.New("replacement started before old membership joined")
	}
}

func TestPinRevisionChangeDiscardsAndJoinsOldMembership(t *testing.T) {
	repo, metadata, items := commitReviewFixture(t)
	gh := &movingPinMembershipGH{FixtureGitHub: FixtureGitHub{metadata}, items: items, started: make(chan struct{}), joined: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Open(ctx, repo.Dir, metadata.Identity, gh, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Inventory.Comparison.Metadata.HeadSHA != metadata.HeadSHA || !got.Inventory.Complete || got.Commits == nil || !got.Commits.Complete || got.Commits.Composition.Status != commits.Captured || gh.listCalls.Load() != 2 || gh.metadataReads.Load() != 3 {
		t.Fatal("changed pins reused old membership or omitted checks")
	}
}
