package commits

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"prui/internal/source"
)

type preparedGH struct {
	fakeGH
	list         func(context.Context, source.Identity) ([]source.PullRequestCommit, error)
	metadataRead func(context.Context, source.Identity) (source.Metadata, error)
}

func (g preparedGH) ListPullRequestCommits(ctx context.Context, id source.Identity) ([]source.PullRequestCommit, error) {
	return g.list(ctx, id)
}
func (g preparedGH) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	if g.metadataRead != nil {
		return g.metadataRead(ctx, id)
	}
	return g.fakeGH.Metadata(ctx, id)
}

func TestPreparedCaptureStartsMembershipOnceAndPreservesResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m := source.Metadata{HeadSHA: strings.Repeat("b", 40), BaseSHA: strings.Repeat("a", 40)}
	entries := []source.PullRequestCommit{{SHA: m.HeadSHA, Subject: "Selected PR", Author: "Fixture"}}
	started, release := make(chan struct{}), make(chan struct{})
	calls := 0
	g := preparedGH{fakeGH: fakeGH{metadata: m}, list: func(ctx context.Context, _ source.Identity) ([]source.PullRequestCommit, error) {
		calls++
		close(started)
		select {
		case <-release:
			return entries, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}}
	pending := PrepareCapture(ctx, source.PinnedComparison{Metadata: m}, g)
	defer pending.Close()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("membership did not start")
	}
	close(release)
	got, err := pending.Capture(nil, source.Defaults(), nil)
	want, wantErr := Capture(ctx, nil, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: entries}, source.Defaults(), nil)
	if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) || calls != 1 {
		t.Fatalf("capture changed: %#v, %v; want %#v, %v; calls %d", got, err, want, wantErr, calls)
	}
	if _, err := pending.Capture(nil, source.Defaults(), nil); !errors.Is(err, source.ErrCommand) {
		t.Fatalf("repeated capture: %v", err)
	}
}

func TestPreparedCaptureCloseCancelsAndJoinsWithoutFreshness(t *testing.T) {
	started, joined := make(chan struct{}), make(chan struct{})
	g := preparedGH{list: func(ctx context.Context, _ source.Identity) ([]source.PullRequestCommit, error) {
		close(started)
		<-ctx.Done()
		close(joined)
		return nil, ctx.Err()
	}, metadataRead: func(context.Context, source.Identity) (source.Metadata, error) {
		t.Fatal("freshness before capture")
		return source.Metadata{}, nil
	}}
	pending := PrepareCapture(context.Background(), source.PinnedComparison{}, g)
	<-started
	pending.Close()
	pending.Close()
	select {
	case <-joined:
	default:
		t.Fatal("membership not joined")
	}
	if _, err := pending.Capture(nil, source.Defaults(), nil); !errors.Is(err, source.ErrCommand) {
		t.Fatal(err)
	}
}

func TestPreparedCaptureSharesWorkDeadlineAndFreshnessReserve(t *testing.T) {
	m := source.Metadata{BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	g := &deadlineGH{fakeGH: fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: m.HeadSHA}}}}
	pending := prepareCapture(context.Background(), source.PinnedComparison{Metadata: m}, g, time.Nanosecond, time.Second)
	defer pending.Close()
	b, err := pending.Capture(nil, source.Defaults(), nil)
	if err != nil || !g.fresh || b.Status != Captured || len(b.Entries) != 1 || b.Entries[0].Reason != "Commit capture budget exhausted." {
		t.Fatal(b, err, g.fresh)
	}
}

func TestPreparedCaptureFailureNoRetryAndParentCancellation(t *testing.T) {
	for _, failure := range []error{source.ErrLimit, source.ErrCommand, context.DeadlineExceeded} {
		calls := 0
		m := source.Metadata{}
		g := preparedGH{fakeGH: fakeGH{metadata: m}, list: func(context.Context, source.Identity) ([]source.PullRequestCommit, error) {
			calls++
			return nil, failure
		}}
		pending := PrepareCapture(context.Background(), source.PinnedComparison{Metadata: m}, g)
		b, err := pending.Capture(nil, source.Defaults(), nil)
		if err != nil || b.Status != Unavailable || calls != 1 {
			t.Fatal(b, err, calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pending := PrepareCapture(ctx, source.PinnedComparison{}, preparedGH{list: func(context.Context, source.Identity) ([]source.PullRequestCommit, error) {
		t.Fatal("membership after parent cancellation")
		return nil, nil
	}})
	_, err := pending.Capture(nil, source.Defaults(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPreparedCaptureUnsupportedAndExpiredTotal(t *testing.T) {
	metadataOnly := struct{ source.GitHub }{fakeGH{}}
	if pending := PrepareCapture(context.Background(), source.PinnedComparison{}, metadataOnly); pending != nil {
		pending.Close()
		t.Fatal("unsupported client started preparation")
	}
	freshnessCalled := false
	g := preparedGH{list: func(ctx context.Context, _ source.Identity) ([]source.PullRequestCommit, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, metadataRead: func(ctx context.Context, _ source.Identity) (source.Metadata, error) {
		freshnessCalled = true
		if ctx.Err() == nil {
			t.Fatal("capture restarted the total deadline")
		}
		return source.Metadata{}, ctx.Err()
	}}
	pending := PrepareCapture(context.Background(), source.PinnedComparison{}, g)
	pending.stopTotal()
	b, err := pending.Capture(nil, source.Defaults(), nil)
	if err != nil || !freshnessCalled || b.Status != Unavailable || b.Reason != "Commit freshness check unavailable." {
		t.Fatal(b, err, freshnessCalled)
	}
}
