package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/session"
	"prui/internal/source"
)

func TestSuggestionPublishedUncertaintyKeepsPrivateDraftAndExactAttempt(t *testing.T) {
	for _, shape := range []string{"range", "file"} {
		t.Run(shape, func(t *testing.T) {
			store, err := session.Open(t.TempDir() + "/private")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m := draftTestModel(t, store)
			defer m.Close()
			m.Width, m.Height = 120, 24
			meta := m.Session.Inventory.Comparison.Metadata
			meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
			m.Session.Inventory.Comparison.Metadata = meta
			target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 2, StartSide: "RIGHT", StartLine: 1}
			pendingTarget := target
			if shape == "file" {
				pendingTarget.SubjectType, pendingTarget.Side, pendingTarget.StartSide, pendingTarget.Line, pendingTarget.StartLine = "file", "", "", 0, 0
			}
			body, _ := source.SuggestionBody("queued replacement")
			m.Pending = []source.ReviewComment{{Target: pendingTarget, Body: body}}
			if !m.persistDraft(m.reviewTabState) {
				t.Fatal(m.ActionError)
			}
			seeded, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil {
				t.Fatal(err)
			}
			yes, no := true, false
			m.Viewer = "alice"
			m.discussions.snapshot = DiscussionSnapshot{CurrentVerified: true, Snapshot: source.DiscussionSnapshot{Complete: true, Threads: []source.Discussion{{ID: "thread", CurrentAnchor: &target, Resolved: &no, CanResolve: &yes, CanUnresolve: &no, Comments: []source.ReviewComment{{ID: 9, Author: "alice", Body: "published body", Target: target}}}}}}
			edits, applications := 0, 0
			m.SetPublishedSubmitter(func(_ context.Context, a PublishedAction) (PublishedValue, error) {
				edits++
				if a.Resolve != nil {
					return PublishedValue{Thread: source.Discussion{ID: a.ThreadID, Resolved: a.Resolve, CanResolve: &no, CanUnresolve: &yes}}, nil
				}
				if a.CommentID != 9 || a.Body != "published body/q" {
					t.Fatal("published edit changed identity/body", a)
				}
				return PublishedValue{}, source.ErrCommentDeliveryUnknown
			})
			if !m.openPublished(9, "", false) {
				t.Fatal("published editor unavailable")
			}
			key(m, '/')
			key(m, 'q')
			for _, control := range []rune{'s', 'a', 'p'} {
				m.Update(tea.KeyPressMsg{Code: control, Mod: tea.ModCtrl})
			}
			if m.Composer != nil || m.SuggestionApply != nil || edits != 0 {
				t.Fatal("published editor routed suggestion keys")
			}
			_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("published edit not dispatched")
			}
			m.Update(cmd())
			key(m, 'x')
			editor := m.discussions.published
			if !editor.uncertain || editor.attempted.Body != "published body/q" || editor.draft != "published body/qx" {
				t.Fatal("published immutable attempt lost")
			}
			_, retry := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if retry != nil || edits != 1 {
				t.Fatal("published uncertainty retried")
			}
			unchanged, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil || unchanged.Generation != seeded.Generation || unchanged.Pending[0].Target != pendingTarget {
				t.Fatal("published uncertainty changed private draft", err)
			}
			namedKey(m, tea.KeyEscape)
			if !m.openPublished(9, "", true) {
				t.Fatal("resolve unavailable")
			}
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("resolve not dispatched")
			}
			m.Update(cmd())
			if edits != 2 || !*m.discussions.snapshot.Snapshot.Threads[0].Resolved {
				t.Fatal("thread resolution lost")
			}
			resolved, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil || resolved.Generation != seeded.Generation || resolved.Pending[0].Target != pendingTarget {
				t.Fatal("resolution changed private drafts", err)
			}

			// The wire fixture is independent of application preparation; only the fake
			// committer sees it, so this test cannot make a GitHub write.
			appliedBody, _ := source.SuggestionBody("applied replacement")
			a := source.SuggestionApplication{Metadata: meta, Target: target, CommentID: 9, Branch: "feature", Before: "first\nsecond", Replacement: "applied replacement", Content: "applied replacement\n", CommentBody: appliedBody}
			input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": target.Path, "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
			a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
			m.acceptSuggestionPrepared(suggestionPrepared{target: m.activeTab, application: a})
			m.SetSuggestionActions(nil, func(_ context.Context, got source.SuggestionApplication) (source.SuggestionResult, error) {
				applications++
				saved, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
				if err != nil || saved.Attempted == nil || saved.Attempted.Application == nil || string(saved.Attempted.Application.Payload) != string(a.Payload) || got.Target != target {
					t.Fatal("dispatch preceded exact durable intent", err)
				}
				return source.SuggestionResult{}, source.ErrCommentDeliveryUnknown
			})
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd != nil || !m.SuggestionConfirm || applications != 0 {
				t.Fatal("first confirmation wrote")
			}
			_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil {
				t.Fatal("second confirmation did not dispatch", m.ActionError)
			}
			m.Update(cmd())
			m.SuggestionApply.Replacement = "later preview change"
			m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
			_, retry = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if retry != nil || applications != 1 {
				t.Fatal("uncertain suggestion retried")
			}
			saved, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil || saved.Generation <= seeded.Generation || saved.Version != 3 || saved.SuggestionApply != nil || saved.Pending[0].Target != pendingTarget || saved.Pending[0].Body != body || saved.Attempted.Application.Replacement != a.Replacement || string(saved.Attempted.Application.Payload) != string(a.Payload) {
				t.Fatal("immutable suggestion or queued target lost", err)
			}
			raw, _ := json.Marshal(saved)
			if strings.Contains(string(raw), "published body") {
				t.Fatal("published editor leaked into private draft")
			}
			restored := draftTestModel(t, store)
			defer restored.Close()
			restored.Width, restored.Height = 120, 24
			if restored.top() != pageDraftRecovery || restored.SuggestionApply == nil || restored.SuggestionApply.Content != a.Content || restored.Pending[0].Target != pendingTarget || restored.discussions.published != nil {
				t.Fatal("combined draft recovery lost isolation")
			}
			namedKey(restored, tea.KeyEnter)
			if restored.top() != pageSuggestionApply {
				t.Fatal("recovery did not open suggestion confirmation")
			}
			_, retry = restored.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if retry != nil || edits != 2 || applications != 1 || !restored.unsentReviewDrafts() {
				t.Fatal("restart wrote or lost quit protection")
			}
		})
	}
}

func TestSuggestionApplicationFreshnessRetainsFrozenComparison(t *testing.T) {
	for _, published := range []bool{false, true} {
		t.Run(map[bool]string{false: "application", true: "published edit"}[published], func(t *testing.T) {
			m := rangeTestModel()
			defer m.Close()
			old := m.Session
			m.openReviewTab(old)
			fresh := *old
			fresh.ID = "new comparison"
			if published {
				m.discussions.published = &publishedEditor{draft: "published edit"}
			} else {
				m.SuggestionApply = &source.SuggestionApplication{Target: source.ReviewCommentTarget{Path: "text", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}, Payload: json.RawMessage(`{"immutable":true}`)}
				m.push(pageSuggestionApply)
			}
			m.Update(PullRequestRefreshResult{Target: m.activeTab, SessionID: old.ID, Generation: m.tabs[m.activeTab].freshnessGeneration, Freshness: PullRequestFreshness{Session: &fresh}})
			if m.Session != old || m.Session.RevisionStatus != session.Stale || !m.unsentReviewDrafts() {
				t.Fatal("freshness displaced frozen action")
			}
			if !published && (m.SuggestionApply == nil || m.SuggestionApply.Target.StartLine != 1 || string(m.SuggestionApply.Payload) != `{"immutable":true}` || m.top() != pageSuggestionApply) {
				t.Fatal("freshness lost application identity")
			}
		})
	}
}
