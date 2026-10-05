package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestDurableDraftsSurvivePublishedEditUncertaintyAndThreadResolution(t *testing.T) {
	store, err := session.Open(t.TempDir() + "/private")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	m := draftTestModel(t, store)
	defer m.Close()
	meta := m.Session.Inventory.Comparison.Metadata
	m.openReviewForm()
	m.ReviewForm.Body = "saved summary"
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 1}
	m.Pending = []source.ReviewComment{{Target: target, Body: "saved pending"}}
	namedKey(m, tea.KeyEscape)
	before, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || before.Summary != "saved summary" || len(before.Pending) != 1 {
		t.Fatal("seed draft not saved", err)
	}
	yes, no := true, false
	m.Viewer = "alice"
	m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", Resolved: &no, CanResolve: &yes, CanUnresolve: &no, Comments: []source.ReviewComment{{ID: 1, Author: "alice", Body: "remote body"}}}}}}
	writes := 0
	m.SetPublishedSubmitter(func(_ context.Context, a PublishedAction) (PublishedValue, error) {
		writes++
		if a.Resolve != nil {
			return PublishedValue{Thread: source.Discussion{ID: a.ThreadID, Resolved: a.Resolve, CanResolve: &no, CanUnresolve: &yes}}, nil
		}
		if a.Body != "remote body/q" {
			t.Fatal("published edit retargeted", a.Body)
		}
		return PublishedValue{}, source.ErrCommentDeliveryUnknown
	})
	if !m.openPublished(1, "", false) {
		t.Fatal("edit unavailable")
	}
	key(m, '/')
	key(m, 'q')
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("explicit edit not dispatched")
	}
	m.Update(cmd())
	key(m, 'x')
	e := m.discussions.published
	if e == nil || !e.uncertain || e.draft != "remote body/qx" || e.attempted.Body != "remote body/q" {
		t.Fatal("published uncertainty lost")
	}
	_, retry := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if retry != nil || writes != 1 {
		t.Fatal("unknown edit retried")
	}
	got, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
	if err != nil || got.Generation != before.Generation || got.Summary != before.Summary || len(got.Pending) != 1 || got.Pending[0].Body != "saved pending" || got.Attempt != "" {
		t.Fatal("published edit altered durable drafts", err)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "remote body") {
		t.Fatal("published editor leaked into durable draft")
	}
	namedKey(m, tea.KeyEscape)
	if !m.openPublished(1, "", true) {
		t.Fatal("resolve unavailable")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(cmd())
	if writes != 2 || !*m.discussions.snapshot.Snapshot.Threads[0].Resolved {
		t.Fatal("canonical resolution missing")
	}
	recovered := draftTestModel(t, store)
	defer recovered.Close()
	if recovered.top() != pageDraftRecovery || recovered.ReviewForm == nil || recovered.ReviewForm.Body != "saved summary" || len(recovered.Pending) != 1 || recovered.Pending[0].Body != "saved pending" {
		t.Fatal("draft recovery lost work")
	}
	if recovered.discussions.published != nil || recovered.draft.attempt != "" || len(recovered.discussions.snapshot.Snapshot.Threads) != 0 {
		t.Fatal("published state entered draft recovery")
	}
	if writes != 2 {
		t.Fatal("recovery wrote remotely")
	}
}
