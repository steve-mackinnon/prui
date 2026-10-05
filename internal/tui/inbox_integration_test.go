package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

func inboxIntegrationSession(t *testing.T, store *session.Store, repository string) *review.Session {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("code.txt", "before\n"+strings.Repeat("context\n", 30))
	base := r.Commit()
	r.Write("code.txt", "after\n"+strings.Repeat("context\n", 30))
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: repository, Number: 7}, BaseRepository: repository, HeadRepository: repository, BaseSHA: base, HeadSHA: head}
	raw, err := review.Open(context.Background(), r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}
func inboxIntegrationDraft(t *testing.T, store *session.Store, s *review.Session, body string) session.Draft {
	t.Helper()
	meta := s.Inventory.Comparison.Metadata
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "code.txt", Side: "RIGHT", StartSide: "RIGHT", StartLine: 1, Line: 2}
	file := target
	file.SubjectType = "file"
	file.Side = ""
	file.StartSide = ""
	file.StartLine = 0
	file.Line = 0
	pending := []source.ReviewComment{{Target: target, Body: "```suggestion\n" + body + "\n```"}, {Target: file, Body: body + " file"}}
	attempt := source.PullRequestReview{Identity: meta.Identity, CommitID: meta.HeadSHA, Event: "COMMENT", Body: body + " summary", Comments: pending[:1]}
	d, err := store.SaveDraft(context.Background(), session.DraftKeyFor(meta), 0, session.Draft{Version: 2, Summary: attempt.Body, Pending: pending, Attempt: "review", Attempted: &session.DraftAttempt{Kind: "review", Review: &attempt}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func TestInboxCombinedSwitchRefreshCancelRestorePreservesFrozenDraftsAndNavigation(t *testing.T) {
	ctx := context.Background()
	store, err := session.Open(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := inboxIntegrationSession(t, store, "o/first")
	second := inboxIntegrationSession(t, store, "o/second")
	firstDraft := inboxIntegrationDraft(t, store, first, "PRIVATE FIRST")
	secondDraft := inboxIntegrationDraft(t, store, second, "PRIVATE SECOND")
	m := New(ctx, nil)
	defer m.Close()
	m.SetLifecycle(store, nil, nil)
	m.openReviewTab(first)
	namedKey(m, tea.KeyEnter)
	key(m, '2')
	m.Width, m.Height = 120, 24
	m.navigation = codeNavigation{mode: "NEW", whitespace: true}
	m.Focus = paneDiff
	m.Scroll[0] = 2
	m.Cursor[0] = 1
	m.Horizontal = 2
	navigation := m.navigation
	scroll, cursor, horizontal := m.Scroll[0], m.Cursor[0], m.Horizontal
	pin, _ := json.Marshal(first)
	draft, _ := json.Marshal(draftContent(m.reviewTabState))
	writes := 0
	m.SetReviewSubmitter(func(context.Context, ReviewSubmission) error { writes++; return nil })
	reads := 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		reads++
		return source.Readiness{Identity: id, ObservedAt: time.Now(), HeadSHA: first.Inventory.Comparison.Metadata.HeadSHA}, nil
	})
	loader := func(_ context.Context, _ source.InboxOptions, refresh bool) (source.Inbox, error) {
		return source.Inbox{Viewer: "account", Complete: !refresh, Problems: []string{"partial remote evidence"}, Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: second.Inventory.Comparison.Metadata.Identity}, Activity: "unknown"}, {PullRequest: source.PullRequest{Identity: first.Inventory.Comparison.Metadata.Identity}, Activity: "read"}}}, nil
	}
	opened := 0
	m.SetInbox(loader, nil, func(_ context.Context, checkout string, id source.Identity, _ func(string)) (*review.Session, error) {
		opened++
		if checkout != "" || id != second.Inventory.Comparison.Metadata.Identity {
			t.Fatal("inbox borrowed repository checkout or identity")
		}
		return store.Load(second.ID)
	}, source.InboxOptions{}, false, false)
	// Actual keyboard dispatch: Files -> switcher -> inbox -> explicit refresh.
	_, switcher := m.Update(tea.KeyPressMsg{Code: 'P', Text: "P"})
	if switcher != nil {
		completeAction(t, m, switcher)
	}
	_, load := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	if load == nil || m.top() != pageInbox {
		t.Fatal("inbox not reachable from real switcher")
	}
	m.Update(load())
	_, refresh := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m.Update(refresh())
	if m.inbox.data.Complete {
		t.Fatal("partial refresh promoted to complete")
	}
	afterPin, _ := json.Marshal(first)
	afterDraft, _ := json.Marshal(draftContent(m.reviewTabState))
	if !bytes.Equal(pin, afterPin) || !bytes.Equal(draft, afterDraft) {
		t.Fatal("inbox refresh changed source or private attempt")
	}
	_, open := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	completeAction(t, m, open)
	if opened != 1 || m.Session.ID != second.ID || m.top() != pageDraftRecovery || m.Pending[0].Target.Identity != second.Inventory.Comparison.Metadata.Identity || m.draft.attempt != "review" {
		t.Fatal("own frozen session/draft recovery lost")
	}
	namedKey(m, tea.KeyEnter)
	key(m, 'P')
	_, load = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m.Update(load())
	key(m, 'j')
	namedKey(m, tea.KeyEnter)
	if m.Session.ID != first.ID || m.navigation != navigation || m.Scroll[0] != scroll || m.Cursor[0] != cursor || m.Horizontal != horizontal || m.Focus != paneDiff {
		t.Fatal("returning to original inbox identity disturbed navigation")
	}
	// Cancel refresh through real dispatch, then deliver late evidence.
	key(m, 'P')
	_, load = m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	m.Update(load())
	_, late := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	namedKey(m, tea.KeyEscape)
	m.Update(late())
	if m.top() != pagePullRequestPicker || m.Session.ID != first.ID {
		t.Fatal("cancel retargeted active review")
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'C')
	if !m.commitFilter.open {
		t.Fatal("C commit filter contract lost")
	}
	key(m, 'C')
	_, ready := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if ready == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R readiness contract lost")
	}
	m.Update(ready())
	namedKey(m, tea.KeyEscape)
	if reads != 1 || writes != 0 {
		t.Fatal("wrong readiness dispatch or implicit review write")
	}
	for _, pair := range []struct {
		s *review.Session
		d session.Draft
	}{{first, firstDraft}, {second, secondDraft}} {
		got, err := store.LoadDraft(ctx, session.DraftKeyFor(pair.s.Inventory.Comparison.Metadata))
		want, _ := json.Marshal(pair.d)
		actual, _ := json.Marshal(got)
		if err != nil || !bytes.Equal(want, actual) {
			t.Fatal("private draft/attempt was changed or retargeted", err)
		}
		saved, err := store.Load(pair.s.ID)
		if err != nil || saved.SnapshotReference != pair.s.SnapshotReference || saved.Generation != pair.s.Generation {
			t.Fatal("frozen saved source changed", err)
		}
	}
}
