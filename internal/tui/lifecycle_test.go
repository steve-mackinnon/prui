package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

func action(t *testing.T, m *Model, k rune) {
	t.Helper()
	_, cmd := m.Update(tea.KeyPressMsg{Code: k, Text: string(k)})
	completeAction(t, m, cmd)
}

func TestLifecycleProgressRefreshPickerAndFreshFailure(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	raw, err := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return saved, nil })
	defer m.Close()
	changed := meta
	changed.HeadSHA = base
	m.SetLifecycle(store, fakeGitHub{changed}, func(context.Context, *review.Session, func(string)) (*review.Session, error) {
		return nil, errors.New("new comparison unavailable")
	})
	completeAction(t, m, m.Init())
	action(t, m, 'm')
	if len(m.Session.ReviewedSliceIDs) != 1 || !strings.Contains(m.View().Content, "1/1 read") {
		t.Fatal("progress missing")
	}
	action(t, m, 'r')
	if m.Session.RevisionStatus != session.Stale || !strings.Contains(m.View().Content, "stale") {
		t.Fatal("stale warning missing")
	}
	action(t, m, 'N')
	if !strings.Contains(m.View().Content, "new comparison unavailable") || m.Session.ID != saved.ID || len(m.Session.ReviewedSliceIDs) != 1 {
		t.Fatal("failed replacement destroyed old session")
	}
	action(t, m, 's')
	if !strings.Contains(m.View().Content, "Sessions") || !strings.Contains(m.View().Content, saved.ID) {
		t.Fatal("picker missing")
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, cmd)
	if m.top() != pageReview || m.Session.ID != saved.ID {
		t.Fatal("picker did not resume")
	}
	action(t, m, 'm')
	got, err := store.Load(saved.ID)
	if err != nil || len(got.ReviewedSliceIDs) != 0 {
		t.Fatal("unmark not saved", err)
	}
	if !strings.Contains(Plain(m.Session), "freshness: stale") || !strings.Contains(Plain(m.Session), saved.ID) {
		t.Fatal("plain lifecycle state missing")
	}
	action(t, m, 'm')
	m.SetLifecycle(store, fakeGitHub{meta}, func(_ context.Context, old *review.Session, _ func(string)) (*review.Session, error) {
		return store.Create(old.Snapshot)
	})
	action(t, m, 'n')
	action(t, m, 'N')
	if m.Session.ID == saved.ID || len(m.Session.ReviewedSliceIDs) != 0 || m.Selected != 0 {
		t.Fatal("new session retained progress or navigation")
	}
	got, err = store.Load(saved.ID)
	if err != nil || len(got.ReviewedSliceIDs) != 1 {
		t.Fatal("new session erased old progress", err)
	}
	for _, width := range []int{40, 99, 100, 140} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		view := ansi.Strip(m.View().Content)
		if width >= 100 && !strings.Contains(view, "Freshness unknown") {
			t.Fatalf("wide layout hid freshness: %q", view)
		}
		progress := "0/1 read"
		if width < 50 {
			progress = "0/1"
		}
		freshness := "Freshness unknown"
		if width < 50 {
			freshness = "? unknown"
		}
		if width < 100 && (!strings.Contains(view, progress) || !strings.Contains(view, freshness)) {
			t.Fatalf("narrow layout lost compact health status: %q", view)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	action(t, m, 'm')
	if m.ActionError == nil || len(m.Session.ReviewedSliceIDs) != 0 {
		t.Fatal("failed save claimed completion")
	}
}

type canceledReader struct{}

func (canceledReader) Metadata(ctx context.Context, _ source.Identity) (source.Metadata, error) {
	<-ctx.Done()
	return source.Metadata{}, ctx.Err()
}

func TestLifecycleCancelRefreshKeepsSnapshot(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	raw, err := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return saved, nil })
	m.SetLifecycle(store, canceledReader{}, nil)
	m.Update(m.Init()())
	action(t, m, 'm')
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil || !m.Busy {
		t.Fatal("refresh not running")
	}
	key(m, 'q')
	m.Close()
	m.Update(cmd())
	got, err := store.Load(saved.ID)
	if err != nil || len(got.ReviewedSliceIDs) != 1 || got.RevisionStatus != session.CheckFailed {
		t.Fatal("cancellation lost progress or claimed current", err)
	}
}

func TestCommentSubmissionSuccessClearsAndFailureRetainsTheTabDraft(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	namedKey(m, tea.KeyEnter)
	key(m, 'o')
	key(m, 'k')

	var submitted CommentSubmission
	m.SetCommentSubmitter(func(_ context.Context, submission CommentSubmission) (source.ReviewComment, error) {
		submitted = submission
		return source.ReviewComment{ID: 1, Author: "reviewer", Target: submission.Comment.Target, Body: submission.Comment.Body}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, cmd)
	if submitted.Comment.Body != "ok" || submitted.Comment.Target.Side != "RIGHT" || m.Composer != nil || m.top() != pageReview {
		t.Fatalf("successful comment was not submitted and cleared: submitted=%#v page=%v composer=%#v", submitted, m.top(), m.Composer)
	}

	namedKey(m, tea.KeyEnter)
	key(m, 'x')
	failure := errors.New("comment rejected")
	m.SetCommentSubmitter(func(context.Context, CommentSubmission) (source.ReviewComment, error) {
		return source.ReviewComment{}, failure
	})
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, cmd)
	if m.Composer == nil || m.Composer.Draft != "x" || !errors.Is(m.ActionError, failure) || m.top() != pageReview {
		t.Fatalf("failed comment did not retain draft and target: composer=%#v error=%v page=%v", m.Composer, m.ActionError, m.top())
	}
}

func TestCommentSubmissionCopiesFrozenMetadataBeforeAsyncWork(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session, m.Selected, m.Focus = kindsSession(), 1, paneDiff
	frozen := m.Session.Inventory.Comparison.Metadata
	namedKey(m, tea.KeyEnter)
	key(m, 'o')
	key(m, 'k')

	var submitted CommentSubmission
	m.SetCommentSubmitter(func(_ context.Context, submission CommentSubmission) (source.ReviewComment, error) {
		submitted = submission
		return source.ReviewComment{ID: 1, Target: submission.Comment.Target, Body: submission.Comment.Body}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// The active session may change after submission starts. The request must
	// already carry the comparison from the originating tab.
	m.Session.Inventory.Comparison.Metadata.HeadSHA = "0000000000000000000000000000000000000000"
	completeAction(t, m, cmd)
	if submitted.Metadata != frozen {
		t.Fatalf("submitted metadata = %#v, want frozen %#v", submitted.Metadata, frozen)
	}
}

func TestLateCommentResultOnlyChangesItsOriginatingTab(t *testing.T) {
	m := New(context.Background(), nil)
	first, second := kindsSession(), kindsSession()
	first.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 1}
	second.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: 2}
	m.openReviewTab(first)
	m.Selected, m.Focus = 1, paneDiff
	namedKey(m, tea.KeyEnter)
	m.Composer.Draft, m.Composer.generation = "first draft", 1
	m.saveActiveReview()

	m.openReviewTab(second)
	m.Selected, m.Focus = 1, paneDiff
	namedKey(m, tea.KeyEnter)
	m.Composer.Draft, m.Composer.generation = "second draft", 9
	secondComposer := m.Composer

	m.Update(CommentResult{Target: 0, Generation: 1})
	if m.Composer != secondComposer || m.Composer.Draft != "second draft" || m.top() != pageReview {
		t.Fatalf("late first-tab result changed active second tab: composer=%#v page=%v", m.Composer, m.top())
	}
	if got := m.tabs[0].review.Composer; got != nil {
		t.Fatalf("successful first-tab result did not clear its composer: %#v", got)
	}
}

func TestPullRequestPickerListsAndOpens(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	raw, err := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RememberRepository("owner/repo", r.Dir); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := NewPullRequestBrowser(context.Background(), store, func(context.Context, string) ([]source.PullRequest, error) {
		return []source.PullRequest{{Identity: meta.Identity, Title: "Open me"}}, nil
	}, func(_ context.Context, checkout string, id source.Identity, _ func(string)) (*review.Session, error) {
		if checkout != r.Dir || id != meta.Identity {
			t.Fatal("picker did not preserve cached checkout and identity", checkout, id)
		}
		return saved, nil
	})
	defer m.Close()
	m.Update(m.Init()())
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	if m.top() != pagePullRequestPicker || !strings.Contains(m.View().Content, "Open me") {
		t.Fatal("pull request list missing")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.Busy {
		t.Fatal("local snapshot open did not start")
	}
	m.Update(cmd())
	if m.Busy || m.top() != pageReview || m.Session == nil || m.Session.ID != saved.ID || !m.Files {
		t.Fatal("selected pull request did not open")
	}

	failing := NewPullRequestBrowser(context.Background(), store, func(context.Context, string) ([]source.PullRequest, error) {
		return nil, errors.New("list unavailable")
	}, nil)
	defer failing.Close()
	failing.Update(failing.Init()())
	_, cmd = failing.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	failing.Update(cmd())
	if failing.top() != pageRepositoryPicker || failing.ActionError == nil {
		t.Fatal("list failure did not retain repository picker")
	}

	calls := 0
	canceling := NewPullRequestBrowser(context.Background(), store, func(ctx context.Context, _ string) ([]source.PullRequest, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []source.PullRequest{{Identity: meta.Identity, Title: "Retry"}}, nil
	}, nil)
	defer canceling.Close()
	canceling.Update(canceling.Init()())
	_, cmd = canceling.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	namedKey(canceling, tea.KeyEscape)
	canceling.Update(<-done)
	if canceling.Busy || canceling.top() != pageRepositoryPicker || !errors.Is(canceling.ActionError, context.Canceled) {
		t.Fatal("canceled list did not retain repository picker")
	}
	_, cmd = canceling.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	canceling.Update(cmd())
	if canceling.top() != pagePullRequestPicker || !strings.Contains(canceling.View().Content, "Retry") {
		t.Fatal("canceled list could not retry")
	}
}

func TestCurrentRepositoryBrowserListsAndOpensWithoutRepositoryPicker(t *testing.T) {
	store := pickerStore(t)
	checkout := t.TempDir()
	identity := source.Identity{Repository: "owner/repo", Number: 42}
	saved := largeSession(1, 1)
	listed := ""
	m := NewCurrentRepositoryBrowser(context.Background(), store, "owner/repo", checkout, func(_ context.Context, repository string) ([]source.PullRequest, error) {
		listed = repository
		return []source.PullRequest{{Identity: identity, Title: "Current checkout change"}}, nil
	}, func(_ context.Context, gotCheckout string, gotIdentity source.Identity, _ func(string)) (*review.Session, error) {
		if gotCheckout != checkout || gotIdentity != identity {
			t.Fatalf("opened %q %v", gotCheckout, gotIdentity)
		}
		return saved, nil
	})
	defer m.Close()

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("current repository browser did not start listing")
	}
	m.Update(cmd())
	if listed != "owner/repo" || m.top() != pagePullRequestPicker || !strings.Contains(m.View().Content, "Current checkout change") {
		t.Fatalf("current repository browser did not show PRs: %s", m.View().Content)
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.Busy || !strings.Contains(m.View().Content, "Opening selected pull request") {
		t.Fatal("PR selection did not show loading state")
	}
	m.Update(cmd())
	if m.top() != pageReview || m.Session != saved {
		t.Fatal("current repository browser did not open selected PR")
	}
}

func TestPullRequestOpeningUsesWorkspaceTabsWithoutReplacingOtherReviews(t *testing.T) {
	store := pickerStore(t)
	checkout := t.TempDir()
	firstID := source.Identity{Repository: "owner/repo", Number: 1}
	secondID := source.Identity{Repository: "owner/repo", Number: 2}
	first, second := largeSession(1, 1), largeSession(1, 1)
	first.Inventory.Comparison.Metadata.Identity = firstID
	second.Inventory.Comparison.Metadata.Identity = secondID

	started := make(chan struct{})
	release := make(chan struct{})
	calls := map[source.Identity]int{}
	m := NewCurrentRepositoryBrowser(context.Background(), store, "owner/repo", checkout,
		func(context.Context, string) ([]source.PullRequest, error) {
			return []source.PullRequest{{Identity: firstID, Title: "first"}, {Identity: secondID, Title: "second"}}, nil
		},
		func(ctx context.Context, gotCheckout string, id source.Identity, _ func(string)) (*review.Session, error) {
			if gotCheckout != checkout {
				t.Fatalf("checkout = %q, want %q", gotCheckout, checkout)
			}
			calls[id]++
			if id == secondID {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return second, nil
			}
			return first, nil
		})
	defer m.Close()
	m.Update(m.Init()())

	// Open the first PR, then request the second from the overlay. Its delayed
	// result must not replace the first review after selecting it again.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	if len(m.tabs) != 1 || m.activeTab != 0 || m.Session != first {
		t.Fatalf("first open tabs=%d active=%d session=%p", len(m.tabs), m.activeTab, m.Session)
	}
	ctrlKey(m, 'p')
	m.PullRequestPicker.Index = 1
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("second open did not start")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-started
	key(m, 'k')
	namedKey(m, tea.KeyEnter)
	close(release)
	m.Update(<-done)
	if m.activeTab != 0 || m.Session != first {
		t.Fatal("late second open replaced the active first review")
	}
	if len(m.tabs) != 1 {
		t.Fatalf("cancelled second open created a review: %#v", m.tabs)
	}

	// Selecting an already open identity activates it without opening again.
	ctrlKey(m, 'p')
	m.PullRequestPicker.Index = 0
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.activeTab != 0 || calls[firstID] != 1 || len(m.tabs) != 1 {
		t.Fatalf("duplicate open calls=%d active=%d tabs=%d", calls[firstID], m.activeTab, len(m.tabs))
	}
}

func TestPullRequestRefreshKeepsCachedReviewInteractive(t *testing.T) {
	cached := largeSession(1, 1)
	cached.ID = "cached-session"
	cached.RevisionStatus = session.Unchecked
	current := *cached
	current.RevisionStatus = session.Current

	started := make(chan struct{})
	release := make(chan struct{})
	m := newModel(context.Background())
	m.openReviewTab(cached)
	m.SetPullRequestRefresh(func(ctx context.Context, opened *review.Session, _ func(string)) (*review.Session, error) {
		if opened != cached {
			t.Fatalf("opened session = %p, want cached %p", opened, cached)
		}
		close(started)
		select {
		case <-release:
			return &current, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	defer m.Close()

	cmd := m.refreshOpenedPullRequest(0, cached)
	if cmd == nil {
		t.Fatal("cached review did not start background refresh")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-started
	if m.Busy || m.loadingModal().active {
		t.Fatal("background refresh blocked the cached review")
	}
	close(release)
	m.Update(<-done)
	if m.Session != &current || m.Session.RevisionStatus != session.Current {
		t.Fatalf("refreshed session = %#v, want current cached review", m.Session)
	}
}

func TestPullRequestOpeningCapacityAndFailureRetainBrowserAndReviews(t *testing.T) {
	store := pickerStore(t)
	identity := source.Identity{Repository: "owner/repo", Number: 99}
	called := 0
	m := NewCurrentRepositoryBrowser(context.Background(), store, "owner/repo", t.TempDir(),
		func(context.Context, string) ([]source.PullRequest, error) {
			return []source.PullRequest{{Identity: identity, Title: "blocked"}}, nil
		},
		func(context.Context, string, source.Identity, func(string)) (*review.Session, error) {
			called++
			return nil, errors.New("open failed")
		})
	defer m.Close()
	m.Update(m.Init()())
	for i := 1; i <= maxTabs; i++ {
		s := largeSession(1, 1)
		s.Inventory.Comparison.Metadata.Identity = source.Identity{Repository: "owner/repo", Number: i}
		m.openReviewTab(s)
	}
	cmd := m.openSelectedPullRequest(identity)
	if cmd != nil || called != 0 || len(m.tabs) != maxTabs || m.ActionError == nil || !strings.Contains(m.ActionError.Error(), "9 reviews") {
		t.Fatalf("capacity did not retain active review: called=%d tabs=%d err=%v", called, len(m.tabs), m.ActionError)
	}
}

func TestGuideConsentCancelsOrSwitchesToDerivedSession(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	raw, err := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	original, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return original, nil })
	defer m.Close()
	m.SetLifecycle(store, fakeGitHub{meta}, nil)
	calls := 0
	m.SetGuideLifecycle(func(_ context.Context, s *review.Session, _ func(string)) (*review.Session, error) {
		calls++
		derived := s.Snapshot
		derived.DerivedFrom = s.ID
		derived.Guides = &guide.Bundle{Status: guide.Unavailable, Reason: "provider unavailable"}
		return store.Create(derived)
	})
	m.Update(m.Init()())
	m.Width = 200
	action(t, m, 'g')
	if m.top() != pageGuideConsent || calls != 0 || !strings.Contains(m.View().Content, "store:false") {
		t.Fatal("guide action did not wait for informed consent")
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview || calls != 0 || m.Session.ID != original.ID {
		t.Fatal("cancelled consent changed the review")
	}
	action(t, m, 'g')
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.Busy {
		t.Fatal("confirmed guide generation did not start")
	}
	m.Update(cmd())
	if calls != 1 || m.Session.ID == original.ID || m.Session.DerivedFrom != original.ID || len(m.Session.ReviewedSliceIDs) != 0 {
		t.Fatal("confirmed guide generation did not switch to a new unread session")
	}
}
