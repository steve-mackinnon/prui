package tui

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

// Observe only on the event-loop goroutine. Tests never read a running Model:
// its pointer fields and maps would otherwise race asynchronous Update calls.
type programFrame struct {
	text, sessionID, failure string
	page                     page
	busy, loading            bool
	read                     int
}

type observedModel struct {
	*Model
	frames chan programFrame
	ctx    context.Context
}

func (m observedModel) observe() {
	f := programFrame{text: ansi.Strip(m.View().Content), page: m.top(), busy: m.Busy, loading: m.Loading}
	if m.Session != nil {
		f.sessionID, f.read = m.Session.ID, len(m.Session.ReviewedSliceIDs)
	}
	if m.ActionError != nil {
		f.failure = m.ActionError.Error()
	}
	select {
	case m.frames <- f:
	case <-m.ctx.Done():
	}
}

func (m observedModel) Init() tea.Cmd {
	cmd := m.Model.Init()
	m.observe()
	return cmd
}

func (m observedModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.Model.Update(msg)
	m.observe()
	return m, cmd
}

type programDriver struct {
	t      *testing.T
	p      *tea.Program
	frames chan programFrame
	done   chan struct{}
	err    error // read only after done closes
	last   programFrame
}

func runProgram(t *testing.T, m *Model) *programDriver {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	h := &programDriver{t: t, frames: make(chan programFrame, 64), done: make(chan struct{})}
	h.p = tea.NewProgram(observedModel{m, h.frames, ctx}, tea.WithInput(nil), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignalHandler(), tea.WithContext(ctx),
		tea.WithWindowSize(120, 24), tea.WithEnvironment([]string{"TERM=dumb"}))
	go func() {
		_, h.err = h.p.Run()
		close(h.done)
	}()
	t.Cleanup(func() {
		cancel()
		h.p.Kill()
		select {
		case <-h.done:
			m.Close()
		case <-time.After(5 * time.Second):
			t.Error("program did not stop during cleanup")
		}
	})
	return h
}

func (h *programDriver) expect(label string, matches func(programFrame) bool) programFrame {
	h.t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case f := <-h.frames:
			h.last = f
			if matches(f) {
				return f
			}
		case <-h.done:
			h.t.Fatalf("program exited while waiting for %s: %v\n%s", label, h.err, h.last.text)
		case <-timer.C:
			h.t.Fatalf("timed out waiting for %s\n%s", label, h.last.text)
		}
	}
}

func (h *programDriver) key(code rune) {
	h.t.Helper()
	msg := tea.KeyPressMsg{Code: code}
	if code >= ' ' && code <= '~' {
		msg.Text = string(code)
	}
	h.p.Send(msg)
}

func (h *programDriver) quit() {
	h.t.Helper()
	h.key('q')
	select {
	case <-h.done:
		if h.err != nil {
			h.t.Fatalf("quit failed: %v", h.err)
		}
	case <-time.After(5 * time.Second):
		h.t.Fatal("quit did not terminate the program")
	}
}

func programStore(t *testing.T) (*session.Store, *review.Session) {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	s := largeSession(2, 2)
	s.Inventory.Comparison.Metadata = source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	saved, err := store.Create(s.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return store, saved
}

func TestProgramReviewProgressSurvivesRestart(t *testing.T) {
	store, saved := programStore(t)
	open := func() *programDriver {
		m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) {
			return store.Load(saved.ID)
		})
		m.SetLifecycle(store, nil, nil)
		return runProgram(t, m)
	}
	h := open()
	h.expect("loaded review", func(f programFrame) bool { return f.sessionID == saved.ID && !f.busy })
	h.key('m')
	h.expect("durable reading progress", func(f programFrame) bool { return f.read == 1 && !f.busy })
	h.key('?')
	h.expect("Health & help", func(f programFrame) bool { return strings.Contains(f.text, "Health & help") })
	h.key(tea.KeyEscape)
	h.expect("review after back", func(f programFrame) bool { return f.page == pageReview })
	h.quit()
	resumed := open()
	resumed.expect("progress after restart", func(f programFrame) bool { return f.read == 1 && !f.busy })
	resumed.quit()
}

func TestProgramBrowserCancelFailureRetryAndBack(t *testing.T) {
	store, saved := programStore(t)
	if err := store.RememberRepository("owner/repo", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	m := NewPullRequestBrowser(context.Background(), store, func(ctx context.Context, _ string) ([]source.PullRequest, error) {
		switch calls.Add(1) {
		case 1:
			<-ctx.Done()
			return nil, ctx.Err()
		case 2:
			return nil, errors.New("synthetic list failure")
		default:
			return []source.PullRequest{
				{Identity: source.Identity{Repository: "owner/repo", Number: 1}, Title: "First PR"},
				{Identity: source.Identity{Repository: "owner/repo", Number: 2}, Title: "Second PR"},
			}, nil
		}
	}, func(context.Context, string, source.Identity, func(string)) (*review.Session, error) {
		return store.Load(saved.ID)
	})
	h := runProgram(t, m)
	h.expect("repository picker startup", func(f programFrame) bool { return !f.busy && strings.Contains(f.text, "owner/repo") })
	h.key(tea.KeyEnter)
	h.expect("pending list request", func(f programFrame) bool { return f.busy })
	h.key(tea.KeyEscape)
	h.expect("canceled request", func(f programFrame) bool { return !f.busy && strings.Contains(f.failure, "canceled") })
	h.key(tea.KeyEnter)
	h.expect("list error", func(f programFrame) bool { return !f.busy && f.failure == "synthetic list failure" })
	h.key(tea.KeyEnter)
	h.expect("successful retry", func(f programFrame) bool { return !f.busy && strings.Contains(f.text, "First PR") })
	h.key(tea.KeyDown)
	h.expect("second PR", func(f programFrame) bool {
		return strings.Contains(f.text, "› #2  Second PR")
	})
	h.key(tea.KeyEscape)
	h.expect("repository restored", func(f programFrame) bool { return f.page == pageRepositoryPicker })
	h.key(tea.KeyEnter)
	h.expect("reopened PR picker", func(f programFrame) bool { return f.page == pagePullRequestPicker && !f.busy })
	h.key(tea.KeyEnter)
	h.expect("opened review", func(f programFrame) bool { return f.sessionID == saved.ID && !f.busy })
	h.quit()
}

func TestProgramQuitCancelsInitialLoad(t *testing.T) {
	canceled := make(chan struct{})
	m := New(context.Background(), func(ctx context.Context, _ func(string)) (*review.Session, error) {
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	h := runProgram(t, m)
	h.expect("loading", func(f programFrame) bool { return f.loading && f.busy })
	h.quit()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("quit left initial loader running")
	}
}

func TestProgramCommentComposerSubmitErrorAndCancellation(t *testing.T) {
	newCommentProgram := func(t *testing.T, submit CommentSubmitter) *programDriver {
		t.Helper()
		m := New(context.Background(), nil)
		m.Loading = false
		m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
		m.SetCommentSubmitter(submit)
		return runProgram(t, m)
	}
	openAndType := func(h *programDriver) {
		h.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		h.expect("comment composer", func(f programFrame) bool {
			return f.page == pageReview && strings.Contains(f.text, "+---")
		})
		h.key('o')
		h.key('k')
	}
	submit := func(h *programDriver) {
		h.p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	}

	t.Run("success", func(t *testing.T) {
		h := newCommentProgram(t, func(_ context.Context, submission CommentSubmission) (source.ReviewComment, error) {
			if submission.Comment.Body != "ok" {
				t.Errorf("submitted draft = %q, want %q", submission.Comment.Body, "ok")
			}
			return source.ReviewComment{ID: 1, Target: submission.Comment.Target, Body: submission.Comment.Body}, nil
		})
		h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.busy })
		openAndType(h)
		submit(h)
		h.expect("successful comment submission", func(f programFrame) bool {
			return f.page == pageReview && !f.busy && f.failure == ""
		})
		h.quit()
	})

	t.Run("error retains draft", func(t *testing.T) {
		h := newCommentProgram(t, func(context.Context, CommentSubmission) (source.ReviewComment, error) {
			return source.ReviewComment{}, errors.New("synthetic comment rejection")
		})
		h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.busy })
		openAndType(h)
		submit(h)
		h.expect("failed comment submission", func(f programFrame) bool {
			return f.page == pageReview && !f.busy && f.failure == "synthetic comment rejection" && strings.Contains(f.text, "synthetic comment rejection")
		})
		h.quit()
	})

	t.Run("cancellation retains draft", func(t *testing.T) {
		started := make(chan struct{})
		h := newCommentProgram(t, func(ctx context.Context, _ CommentSubmission) (source.ReviewComment, error) {
			close(started)
			<-ctx.Done()
			return source.ReviewComment{}, ctx.Err()
		})
		h.expect("review", func(f programFrame) bool { return f.page == pageReview && !f.busy })
		openAndType(h)
		submit(h)
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("comment submitter did not start")
		}
		h.expect("pending comment submission", func(f programFrame) bool { return f.busy })
		h.p.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
		h.expect("canceled comment submission", func(f programFrame) bool {
			return f.page == pageReview && !f.busy && f.failure == context.Canceled.Error()
		})
		h.quit()
	})
}

func TestProgramBackgroundRefreshPreservesPendingProgress(t *testing.T) {
	store, saved := programStore(t)
	released := make(chan struct{})
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return store.Load(saved.ID) })
	m.SetLifecycle(store, nil, nil)
	m.SetPullRequestRefresh(func(ctx context.Context, _ PullRequestRefreshRequest, _ func(string)) (PullRequestFreshness, error) {
		select {
		case <-released:
		case <-ctx.Done():
			return PullRequestFreshness{}, ctx.Err()
		}
		return PullRequestFreshness{Status: session.Current}, nil
	})
	h := runProgram(t, m)
	h.expect("loaded", func(f programFrame) bool { return f.sessionID == saved.ID })
	h.key('m')
	h.expect("marked", func(f programFrame) bool { return f.read == 1 })
	close(released)
	h.expect("refreshed", func(f programFrame) bool { return strings.Contains(f.text, "current") })
	if h.last.read != 1 {
		t.Fatal("refresh lost reading progress")
	}
	h.quit()
	reopened, err := store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.ReviewedSliceIDs) != 1 || reopened.RevisionStatus != session.Current {
		t.Fatalf("reopened state = %#v", reopened.State)
	}
}
