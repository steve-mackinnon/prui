package tui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestIssueContextUpdatePrivateAttemptsSourceAndLayoutIntegration(t *testing.T) {
	for _, general := range []bool{false, true} {
		t.Run(map[bool]string{false: "v3", true: "v4"}[general], func(t *testing.T) {
			ctx := context.Background()
			r := testutil.NewRepo(t)
			r.Write(".gitattributes", "generated.go review-generated\n")
			r.Write("generated.go", "old generated\n")
			r.Write("code.go", "value := 12 // café 世界\ncontext needle\n")
			base := r.Commit()
			r.Write("generated.go", "new generated\n")
			r.Write("code.go", "value := 56 // café 世界\ncontext needle\n")
			head := r.Commit()
			meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 7}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
			captured, err := review.OpenWithConfig(ctx, r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{CacheFullSource: true})
			if err != nil {
				t.Fatal(err)
			}
			store, err := session.Open(filepath.Join(t.TempDir(), "private"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			rec, err := store.Create(captured.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			m := New(ctx, nil)
			defer m.Close()
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(rec)
			m.Width, m.Height = 200, 24
			m.selectReviewView(viewFiles)
			code := -1
			for i, f := range rec.Inventory.Files {
				if string(f.NewPath) == "code.go" {
					code = i
				}
			}
			if code < 0 {
				t.Fatal("code missing")
			}
			m.selectFile(code)
			m.Focus = paneDiff
			selectRawTarget(t, m, "RIGHT", 1)
			canonical := *m.selectedDiffTarget()
			wordState := func() []byte {
				var rows []any
				highlighted := 0
				for unit, file := range m.Session.UnitFiles {
					if file != code {
						continue
					}
					for _, row := range unitLines(m.Session, unit) {
						if len(row.wordChanges) > 0 {
							highlighted++
						}
						rows = append(rows, []any{row.Text, row.rawSource, row.searchID, row.wordChanges, row.target, row.oldTarget})
					}
				}
				if highlighted != 2 {
					t.Fatal("context fixture lost word-level emphasis", highlighted)
				}
				b, e := json.Marshal(rows)
				if e != nil {
					t.Fatal(e)
				}
				return b
			}
			wordBefore := wordState()
			// Keep real durable attempt payloads and a prepared suggestion while reading.
			body, _ := source.SuggestionBody("private replacement")
			a := source.SuggestionApplication{Metadata: meta, Target: canonical, CommentID: 9, Branch: "feature", Before: "value := 56 // café 世界", Replacement: "private replacement", Content: "private replacement\ncontext needle\n", CommentBody: body}
			input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": meta.HeadRepository, "refName": a.Branch}, "expectedHeadOid": head, "message": map[string]string{"headline": "Apply review suggestion #9"}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": "code.go", "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
			a.Payload, _ = json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
			m.SuggestionApply = &a
			m.Pending = []source.ReviewComment{{Target: canonical, Body: body}}
			m.ReviewForm = &reviewForm{Body: "private summary"}
			m.draft.attempt = "review"
			m.draft.attempted = &session.DraftAttempt{Kind: "review", Review: &source.PullRequestReview{Identity: meta.Identity, CommitID: head, Event: "COMMENT", Body: "immutable dispatched review", Comments: append([]source.ReviewComment(nil), m.Pending...)}}
			if general {
				m.discussions.editor = &generalCommentEditor{draft: "edited private general", cursor: 3, attemptedBody: "immutable dispatched general", attemptedIDs: map[string]bool{"PR comment:12": true}, uncertain: true}
			}
			if !m.persistDraft(m.reviewTabState) {
				t.Fatal(m.ActionError)
			}
			privateBefore, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || privateBefore.Version != map[bool]int{false: 3, true: 4}[general] {
				t.Fatal("private version", privateBefore, err)
			}
			frozenBefore, _ := json.Marshal(rec.Snapshot)
			reads, writes := 0, 0
			cancelled := false
			m.SetReviewSubmitter(func(context.Context, ReviewSubmission) error { writes++; return nil })
			snapshot := source.IssueContext{Identity: meta.Identity, HeadSHA: strings.Repeat("f", 40), CapturedAt: time.Now().UTC(), Complete: false, Reason: "additional GitHub records", Labels: []string{"bug"}, LinearWorkspace: "work", LinearAuth: "api_key", Linear: []source.ContextIssue{{Identifier: "APP-2", Title: "product", Status: "Done", Description: "saved detail", URL: "https://linear.app/work/issue/APP-2/fix"}}}
			m.SetIssueContextReader(func(c context.Context, id source.Identity, refresh bool) (source.IssueContext, error) {
				reads++
				if id != meta.Identity {
					t.Fatal("context identity")
				}
				if refresh && c.Err() != nil {
					cancelled = true
					late := snapshot
					late.Labels = []string{"late canceled"}
					return late, nil
				}
				if refresh {
					return snapshot, nil
				}
				return source.IssueContext{}, session.ErrIssueContextNotFound
			})
			presentation := func() []byte {
				b, e := json.Marshal([]any{m.Selected, m.Row, m.Focus, m.layout, m.groupFiles, m.collapseGenerated, m.listWidthPreference, m.Horizontal, m.navigation.mode, m.navigation.whitespace, m.Scroll, m.Cursor, m.CursorTarget, m.ContextView})
				if e != nil {
					t.Fatal(e)
				}
				return b
			}
			presentationBefore := presentation()
			keyCmd := func(k tea.KeyPressMsg) tea.Cmd { _, cmd := m.Update(k); return cmd }
			cmd := keyCmd(tea.KeyPressMsg{Code: 'I', Text: "I"})
			if cmd == nil || m.top() != pageIssueContext {
				t.Fatal("I did not dispatch context")
			}
			m.Update(cmd())
			cmd = keyCmd(tea.KeyPressMsg{Code: 'r', Text: "r"})
			if cmd == nil {
				t.Fatal("explicit refresh unavailable")
			}
			m.Update(cmd())
			text := m.issueContextView()
			for _, want := range []string{"PARTIAL", "differs from frozen", "workspace: work", "auth: api_key", "current account access unknown"} {
				if !strings.Contains(text, want) {
					t.Fatal("missing honest context", want, text)
				}
			}
			cmd = keyCmd(tea.KeyPressMsg{Code: 'r', Text: "r"})
			keyCmd(tea.KeyPressMsg{Code: tea.KeyEscape})
			m.Update(cmd())
			if !cancelled || m.top() != pageReview || m.issues.snapshot.Labels[0] != "bug" || reads != 3 || writes != 0 {
				t.Fatal("cancel applied late context or wrote", cancelled, reads, writes)
			}
			if !bytes.Equal(presentationBefore, presentation()) {
				t.Fatal("context open/refresh/cancel mutated reading layout/cursor")
			}
			if !bytes.Equal(wordBefore, wordState()) {
				t.Fatal("context open/refresh/cancel changed word spans/raw text/targets/search IDs")
			}
			// Actual merged shortcuts remain available and keep canonical coordinates.
			keyCmd(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
			keyCmd(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
			keyCmd(tea.KeyPressMsg{Code: 'C', Text: "C"})
			if !m.commitFilter.open {
				t.Fatal("C filter lost")
			}
			keyCmd(tea.KeyPressMsg{Code: tea.KeyEscape})
			if m.selectedDiffTarget() == nil || *m.selectedDiffTarget() != canonical {
				t.Fatal("context/layout changed canonical anchor")
			}
			if !general {
				m.navigation.mode = "NEW"
				m.fileCache = fileDetailCache{}
				searchInput(m, "context needle")
				namedKey(m, tea.KeyEnter)
				if m.selectedDiffTarget() != nil {
					t.Fatal("source search yielded comment target")
				}
				sourceID := m.displayDetail()[m.cursor()].searchID
				if sourceID.Row != 2 {
					t.Fatal("did not select real captured context line", sourceID)
				}
				m.search = [2]*diffSearchState{}
				readOnlyBefore := presentation()
				keyCmd(tea.KeyPressMsg{Code: 'I', Text: "I"})
				cmd = keyCmd(tea.KeyPressMsg{Code: 'r', Text: "r"})
				m.Update(cmd())
				keyCmd(tea.KeyPressMsg{Code: tea.KeyEscape})
				if m.selectedDiffTarget() != nil || !layoutRowHasSourceID(m.displayDetail()[m.cursor()], sourceID) || !bytes.Equal(readOnlyBefore, presentation()) {
					t.Fatal("context retargeted readonly source cursor")
				}
			}
			if !bytes.Equal(wordBefore, wordState()) {
				t.Fatal("context source search changed canonical word metadata")
			}
			// Both historical private attempt shapes survive all context Update calls.
			privateAfter, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(privateBefore, privateAfter) {
				t.Fatal("context mutated private payload/generation", err)
			}
			frozenAfter, _ := json.Marshal(m.Session.Snapshot)
			if !bytes.Equal(frozenBefore, frozenAfter) || m.Session != rec {
				t.Fatal("context changed frozen source bytes/pins")
			}
			restored := New(ctx, nil)
			defer restored.Close()
			restored.SetLifecycle(store, nil, nil)
			restored.openReviewTab(rec)
			if restored.draft.attempt != "review" || restored.draft.attempted.Review.Body != "immutable dispatched review" || restored.SuggestionApply.Target != canonical || restored.Pending[0].Target != canonical || general && (!restored.discussions.editor.uncertain || restored.discussions.editor.attemptedBody != "immutable dispatched general") {
				t.Fatal("restart lost private attempts/canonical targets")
			}
		})
	}
}

func TestIssueContextUpdateShortcutsAndReadonlyCursor(t *testing.T) {
	m := rangeTestModel()
	defer m.Close()
	m.openReviewTab(m.Session)
	m.selectReviewView(viewFiles)
	m.Selected, m.Focus = 1, paneDiff
	selectRawTarget(t, m, "RIGHT", 1)
	want := *m.selectedDiffTarget()
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	if cmd != nil {
		t.Fatal("unconfigured context dispatched network")
	}
	namedKey(m, tea.KeyEscape)
	key(m, 'F')
	if !m.fileFilterEditing {
		t.Fatal("F path filter lost")
	}
	namedKey(m, tea.KeyEscape)
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		return source.Readiness{Identity: id, HeadSHA: want.CommitID, ObservedAt: time.Now()}, nil
	})
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if cmd == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R lost")
	}
	m.Update(cmd())
	namedKey(m, tea.KeyEscape)
	if m.selectedDiffTarget() == nil || *m.selectedDiffTarget() != want {
		t.Fatal("context/readiness moved canonical cursor")
	}
	// Source expansion has read-only row identity; context cannot turn it into a target.
	m.navigation.mode = "NEW"
	m.fileCache = fileDetailCache{}
	_, cmd = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if cmd == nil && !m.searchOpen() {
		t.Fatal("slash search lost")
	}
	namedKey(m, tea.KeyEscape)
}

func TestIssueContextUpdateRejectsForeignIdentityAndSession(t *testing.T) {
	m := commitModel(t)
	defer m.Close()
	s := m.Session
	id := s.Inventory.Comparison.Metadata.Identity
	m.SetIssueContextReader(func(context.Context, source.Identity, bool) (source.IssueContext, error) {
		return source.IssueContext{Identity: source.Identity{Repository: "foreign/repo", Number: id.Number}, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now(), Complete: true, Labels: []string{"foreign private"}}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'I', Text: "I"})
	m.Update(cmd())
	if m.issues.loaded || !strings.Contains(m.issueContextView(), "different PR identity") || strings.Contains(m.issueContextView(), "foreign private") {
		t.Fatal("foreign PR context rendered")
	}
	wrong := *s
	m.Update(IssueContextResult{Target: m.activeTab, Session: &wrong, Generation: m.issues.generation, Snapshot: source.IssueContext{Identity: id, Labels: []string{"late source"}}})
	if m.issues.loaded {
		t.Fatal("different frozen session accepted")
	}
}
