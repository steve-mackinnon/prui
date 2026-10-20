package review

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"prui/internal/commits"
	"prui/internal/source"
)

type membershipGH struct {
	commitGitHub
	started, inventory, joined chan struct{}
	fail                       bool
}

func (g *membershipGH) ListPullRequestCommits(ctx context.Context, id source.Identity) ([]source.PullRequestCommit, error) {
	close(g.started)
	defer close(g.joined)
	if g.fail {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-g.inventory:
		return g.commitGitHub.ListPullRequestCommits(ctx, id)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type membershipRunner struct {
	source.Runner
	gh      *membershipGH
	once    sync.Once
	failure error
	view    string
}

func (r *membershipRunner) MaxParallelism() int { return 8 }
func (r *membershipRunner) Run(ctx context.Context, q source.Request) ([]byte, error) {
	if len(q.Args) > 2 && q.Args[2] == "diff-tree" {
		r.once.Do(func() {
			select {
			case <-r.gh.started:
				close(r.gh.inventory)
			case <-ctx.Done():
			}
			r.view = q.Dir
		})
		if r.failure != nil {
			return nil, r.failure
		}
	}
	return r.Runner.Run(ctx, q)
}

func TestMembershipReadOverlapsMainInventory(t *testing.T) {
	r, m, items := commitReviewFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gh := &membershipGH{commitGitHub: commitGitHub{FixtureGitHub: FixtureGitHub{m}, items: items}, started: make(chan struct{}), inventory: make(chan struct{}), joined: make(chan struct{})}
	runner := &membershipRunner{Runner: source.NewRunner(), gh: gh}
	got, err := Open(ctx, r.Dir, m.Identity, gh, runner, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Inventory.Complete || !got.Commits.Complete || got.Commits.Composition.Status != commits.Captured || gh.listCalls != 1 {
		t.Fatal("incomplete capture")
	}
	select {
	case <-gh.joined:
	default:
		t.Fatal("membership not joined")
	}
}

func TestInventoryFailureCancelsMembershipBeforeClosingView(t *testing.T) {
	r, m, items := commitReviewFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	gh := &membershipGH{commitGitHub: commitGitHub{FixtureGitHub: FixtureGitHub{m}, items: items}, started: make(chan struct{}), inventory: make(chan struct{}), joined: make(chan struct{}), fail: true}
	runner := &membershipRunner{Runner: source.NewRunner(), gh: gh, failure: errors.New("inventory failure")}
	got, err := Open(ctx, r.Dir, m.Identity, gh, runner, source.Defaults(), nil)
	if err == nil || got != nil {
		t.Fatal("inventory failure opened review")
	}
	select {
	case <-gh.joined:
	default:
		t.Fatal("membership not joined before return")
	}
	if runner.view == "" {
		t.Fatal("fixture did not reach main inventory")
	}
	if _, err := os.Stat(runner.view); !os.IsNotExist(err) {
		t.Fatalf("view retained: %v", err)
	}
}
