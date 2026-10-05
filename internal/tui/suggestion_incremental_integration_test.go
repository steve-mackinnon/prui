package tui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/session"
	"prui/internal/source"
)

func TestSuggestionComparisonResetClearsApplicationAndRetainsOriginalDraft(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "prepared", true: "uncertain"}[uncertain], func(t *testing.T) {
			store, err := session.Open(t.TempDir() + "/private")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			m := draftTestModel(t, store)
			defer m.Close()
			m.Width, m.Height = 120, 24
			old := m.Session
			meta := old.Inventory.Comparison.Metadata
			meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
			old.Inventory.Comparison.Metadata = meta
			target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
			body, _ := source.SuggestionBody("private replacement")
			a := source.SuggestionApplication{Metadata: meta, Target: target, CommentID: 9, Branch: "feature", Before: "first\nsecond", Replacement: "private replacement", Content: "private replacement\n", CommentBody: body}
			input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": meta.HeadSHA, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": "text", "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
			a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
			m.SuggestionApply = &a
			m.SuggestionConfirm, m.SuggestionScroll = true, 7
			m.push(pageSuggestionApply)
			m.Pending = []source.ReviewComment{{Target: target, Body: body}, {Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "text", SubjectType: "file"}, Body: "private file draft"}}
			if uncertain {
				if !m.prepareDraftAttempt("suggestion") {
					t.Fatal(m.ActionError)
				}
			} else if !m.persistDraft(m.reviewTabState) {
				t.Fatal(m.ActionError)
			}
			original, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil {
				t.Fatal(err)
			}
			searchCtx, cancelSearch := context.WithCancel(context.Background())
			m.search[0] = &diffSearchState{session: old, cancel: cancelSearch}
			m.navigation = codeNavigation{mode: "NEW", whitespace: true}
			m.incremental = incrementalViewState{drafts: true, offset: 4}
			writes := 0
			m.SetSuggestionActions(nil, func(context.Context, source.SuggestionApplication) (source.SuggestionResult, error) {
				writes++
				return source.SuggestionResult{}, nil
			})
			fresh := *old
			fresh.ID = strings.Repeat("e", 32)
			fresh.Inventory.Comparison.Metadata.HeadSHA = strings.Repeat("c", 40)
			m.Update(ActionResult{Session: &fresh, Reset: true})
			if m.SuggestionApply != nil || m.SuggestionConfirm || m.SuggestionScroll != 0 || m.draft.attempt != "" || m.draft.attempted != nil || m.top() != pageReview {
				t.Fatal("old suggestion application entered new comparison")
			}
			if searchCtx.Err() != context.Canceled || m.search[0] != nil || m.navigation != (codeNavigation{}) || !reflect.DeepEqual(m.incremental, incrementalViewState{}) {
				t.Fatal("parent incremental/navigation reset lost")
			}
			if len(m.Pending) != 0 || m.ActionError != nil {
				t.Fatal("old drafts copied or new draft save blocked", m.ActionError)
			}
			empty, err := store.LoadDraft(context.Background(), session.DraftKeyFor(fresh.Inventory.Comparison.Metadata))
			if err != nil || empty.Generation != 0 {
				t.Fatal("new comparison received original private intent", err)
			}
			retained, err := store.LoadDraft(context.Background(), session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(retained, original) {
				t.Fatal("reset changed original private application", err)
			}
			m.openIncremental()
			ctrlKey(m, 's')
			ctrlKey(m, 'a')
			namedKey(m, tea.KeyEnter)
			if writes != 0 || m.SuggestionApply != nil || m.Composer != nil {
				t.Fatal("read-only incremental view wrote or retargeted")
			}
			m.Update(ActionResult{Session: old, Reset: true})
			if m.top() != pageDraftRecovery || m.SuggestionApply == nil || m.SuggestionApply.Target != target || string(m.SuggestionApply.Payload) != string(a.Payload) || !reflect.DeepEqual(m.Pending, original.Pending) || m.SuggestionConfirm || m.SuggestionScroll != 0 {
				t.Fatal("original comparison failed to recover exact private work")
			}
			if uncertain && (m.draft.attempt != "suggestion" || m.draft.attempted.Application == nil || string(m.draft.attempted.Application.Payload) != string(a.Payload)) {
				t.Fatal("original immutable attempt lost")
			}
			namedKey(m, tea.KeyEnter)
			if m.top() != pageSuggestionApply || writes != 0 {
				t.Fatal("recovery wrote or opened wrong modal")
			}
			if uncertain {
				_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd != nil || writes != 0 {
					t.Fatal("reset enabled uncertain retry")
				}
			}
		})
	}
}
