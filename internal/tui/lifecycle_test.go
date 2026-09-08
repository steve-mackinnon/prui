package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

func action(t *testing.T, m *Model, k rune) {
	t.Helper()
	_, cmd := m.Update(tea.KeyPressMsg{Code: k, Text: string(k)})
	if cmd != nil {
		m.Update(cmd())
	}
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
	m.Update(m.Init()())
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
	if cmd != nil {
		m.Update(cmd())
	}
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
		if !strings.Contains(m.View().Content, "freshness: unchecked") {
			t.Fatal("narrow layout hid freshness")
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
