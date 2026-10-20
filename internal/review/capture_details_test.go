package review

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"prui/internal/commits"
	reviewcontext "prui/internal/context"
	"prui/internal/source"
)

func TestCaptureDetailsParallelJoinsEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evidenceStarted, captureDone, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	want := &commits.Bundle{HeadSHA: "pinned"}
	go func() {
		bundle, evidence, err := captureDetails(ctx, true, func(context.Context) (*commits.Bundle, error) {
			select {
			case <-evidenceStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			close(captureDone)
			return want, nil
		}, func(ctx context.Context) reviewcontext.ContextBundle {
			close(evidenceStarted)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return reviewcontext.ContextBundle{ComparisonID: "pinned"}
		})
		if err == nil && (bundle != want || evidence.ComparisonID != "pinned") {
			err = errors.New("capture results changed")
		}
		done <- err
	}()
	select {
	case <-captureDone:
	case <-ctx.Done():
		t.Fatal("capture did not overlap evidence")
	}
	select {
	case <-done:
		t.Fatal("returned before evidence joined")
	default:
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCaptureDetailsErrorCancelsAndJoins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	want := errors.New("capture failed")
	bundle, _, err := captureDetails(ctx, true, func(context.Context) (*commits.Bundle, error) {
		select {
		case <-started:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, want
	}, func(ctx context.Context) reviewcontext.ContextBundle {
		close(started)
		<-ctx.Done()
		close(stopped)
		return reviewcontext.ContextBundle{}
	})
	if bundle != nil || err != want {
		t.Fatalf("unexpected capture error: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("evidence not joined")
	}
}

func TestCaptureDetailsSerialOrderingAndFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		var order []string
		want := errors.New("failed")
		_, _, err := captureDetails(context.Background(), false, func(context.Context) (*commits.Bundle, error) {
			order = append(order, "capture")
			if failed {
				return nil, want
			}
			return &commits.Bundle{}, nil
		}, func(context.Context) reviewcontext.ContextBundle {
			order = append(order, "evidence")
			return reviewcontext.ContextBundle{}
		})
		expected := []string{"capture", "evidence"}
		if failed {
			expected = expected[:1]
		}
		if !reflect.DeepEqual(order, expected) || failed != errors.Is(err, want) {
			t.Fatalf("order %v, error %v", order, err)
		}
	}
}

func TestCaptureDetailsParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	_, _, err := captureDetails(ctx, true, func(ctx context.Context) (*commits.Bundle, error) {
		cancel()
		return nil, ctx.Err()
	}, func(ctx context.Context) reviewcontext.ContextBundle {
		<-ctx.Done()
		close(stopped)
		return reviewcontext.ContextBundle{}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("cancelled evidence not joined")
	}
}

type serialDetailRunner struct{ source.Runner }

func TestParallelDetailsPreserveFrozenSnapshot(t *testing.T) {
	r, metadata, items := commitReviewFixture(t)
	r.Write("README.md", "Supporting documentation\n")
	metadata.HeadSHA = r.Commit()
	items = append(items, source.PullRequestCommit{SHA: metadata.HeadSHA, Subject: "Documentation", Author: "Fixture"})
	before := r.Snapshot()
	var snapshots []*Session
	for _, runner := range []source.Runner{serialDetailRunner{source.NewRunner()}, source.NewRunner()} {
		gh := &commitGitHub{FixtureGitHub: FixtureGitHub{metadata}, items: items}
		result, err := Open(context.Background(), r.Dir, metadata.Identity, gh, runner, source.Defaults(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Inventory.Complete || !result.Commits.Complete || result.Commits.Composition.Status != commits.Captured || len(result.Context.Evidence) == 0 {
			t.Fatal("incomplete fixture")
		}
		snapshots = append(snapshots, result)
	}
	if !reflect.DeepEqual(snapshots[0].Snapshot, snapshots[1].Snapshot) {
		t.Fatal("parallel capture changed frozen source")
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("capture changed checkout")
	}
}
