package commits

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"prui/internal/source"
)

// PendingCapture overlaps only the selected PR's membership read with other
// local work. Its original 55/60-second work/freshness deadlines start together;
// Capture never restarts them. Close cancels and joins an unused preparation.
type PendingCapture struct {
	parent, work, total context.Context
	stopWork, stopTotal context.CancelFunc
	comparison          source.PinnedComparison
	gh                  source.GitHub
	done                chan struct{}
	entries             []source.PullRequestCommit
	err                 error
	used, closed        atomic.Bool
}

// PrepareCapture starts membership loading, or returns nil for an unsupported
// client. The caller must consume it once with Capture or cancel/join with Close.
func PrepareCapture(parent context.Context, p source.PinnedComparison, gh source.GitHub) *PendingCapture {
	return prepareCapture(parent, p, gh, 55*time.Second, 60*time.Second)
}

func prepareCapture(parent context.Context, p source.PinnedComparison, gh source.GitHub, workDuration, totalDuration time.Duration) *PendingCapture {
	reader, ok := gh.(source.PullRequestCommitReader)
	if !ok {
		return nil
	}
	total, stopTotal := context.WithTimeout(parent, totalDuration)
	work, stopWork := context.WithTimeout(total, workDuration)
	pending := &PendingCapture{parent: parent, work: work, total: total, stopWork: stopWork, stopTotal: stopTotal, comparison: p, gh: gh, done: make(chan struct{})}
	go func() {
		defer close(pending.done)
		if err := parent.Err(); err != nil {
			pending.err = err
			return
		}
		entries, err := reader.ListPullRequestCommits(work, p.Metadata.Identity)
		pending.entries, pending.err = slices.Clone(entries), err
	}()
	return pending
}

func (p *PendingCapture) Close() {
	if p == nil {
		return
	}
	p.closed.Store(true)
	p.stopWork()
	p.stopTotal()
	<-p.done
}

// Capture consumes the preparation once. Local capture and the final metadata
// read retain their existing order and budgets; a membership failure is not
// retried. The caller continues to own the view.
func (p *PendingCapture) Capture(v *source.View, l source.Limits, notify func(string)) (*Bundle, error) {
	if p == nil || p.closed.Load() || !p.used.CompareAndSwap(false, true) {
		return nil, source.ErrCommand
	}
	defer p.Close()
	return captureWithContexts(p.parent, p.work, p.total, v, p.comparison, preparedMembership{p.gh, p}, l, notify)
}

type preparedMembership struct {
	source.GitHub
	pending *PendingCapture
}

func (g preparedMembership) ListPullRequestCommits(_ context.Context, id source.Identity) ([]source.PullRequestCommit, error) {
	if id != g.pending.comparison.Metadata.Identity {
		return nil, source.ErrCommand
	}
	// Join even on deadline/cancellation before the final metadata read can run.
	// A valid response arriving at the work deadline still retains membership,
	// exactly as in ordinary capture; exhausted per-commit work stays explicit.
	<-g.pending.done
	return g.pending.entries, g.pending.err
}
