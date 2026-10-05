package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

func TestLayoutRealCapturePrivateDraftOfflineRecovery(t *testing.T) {
	for _, general := range []bool{false, true} {
		t.Run(map[bool]string{false: "suggestion-v3", true: "general-v4"}[general], func(t *testing.T) {
			ctx := context.Background()
			r := testutil.NewRepo(t)
			r.Write(".gitattributes", "a-generated.go review-generated\n")
			r.Write("a-generated.go", strings.Repeat("old generated\n", 30))
			r.Write("z-code.go", "old\ncontext needle\n")
			base := r.Commit()
			r.Write("a-generated.go", strings.Repeat("new generated\n", 30))
			r.Write("z-code.go", "new\ncontext needle\n")
			head := r.Commit()
			meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
			captured, err := review.OpenWithConfig(ctx, r.Dir, meta.Identity, incrementalGH{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{CacheFullSource: true})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "store")
			store, err := session.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			rec, err := store.Create(captured.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: head, Path: "z-code.go", Side: "RIGHT", Line: 1}
			d := session.Draft{Version: 3, Composer: &session.DraftEditor{Target: target, Body: "private replacement", Before: "new", Suggestion: true, PendingIndex: -1}, Pending: []source.ReviewComment{{Target: target, Body: "```suggestion\nprivate replacement\n```"}}}
			if general {
				d.General = &session.GeneralDraft{Body: "private edited general", Cursor: 3, AttemptedBody: "immutable dispatched general", ObservedIDs: []string{"PR comment:12"}, Uncertain: true}
				d.Version = 4
			}
			m := New(ctx, nil)
			defer m.Close()
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(rec)
			m.Width = 200
			m.Height = 24
			m.selectReviewView(viewFiles)
			code := -1
			generated := -1
			for i, f := range m.Session.Inventory.Files {
				if string(f.NewPath) == "z-code.go" {
					code = i
				}
				if string(f.NewPath) == "a-generated.go" {
					generated = i
				}
			}
			if code < 0 || generated < 0 || m.fileCategory(generated) != inventory.Generated {
				t.Fatal("real attributes missing")
			}
			m.selectFile(code)
			m.Focus = paneDiff
			m.cursorActive = true
			before, _ := json.Marshal(m.Session)
			for _, side := range []string{"LEFT", "RIGHT"} {
				found := false
				for i, line := range m.displayDetail() {
					if line.target != nil && line.target.Path == "z-code.go" && line.target.Side == side && line.target.Line == 1 {
						m.setCursor(i)
						m.setSelectedDiffTarget(line.target)
						found = true
						break
					}
				}
				if !found {
					t.Fatal("raw target missing", side)
				}
				want := *m.selectedDiffTarget()
				for step, change := range []func(){func() { key(m, 'B') }, func() { m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt}) }, func() { key(m, 'S') }, func() { m.resizeList(7) },
					func() {
						divider := m.workspaceGeometry().Divider
						m.Update(tea.MouseClickMsg{X: divider.Min.X, Y: divider.Min.Y, Button: tea.MouseLeft})
						m.Update(tea.MouseMotionMsg{X: divider.Min.X + 3, Y: divider.Min.Y, Button: tea.MouseLeft})
						m.Update(tea.MouseReleaseMsg{X: divider.Min.X + 3, Y: divider.Min.Y, Button: tea.MouseLeft})
					}, func() {
						m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
						m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
					}} {
					change()
					got := m.selectedDiffTarget()
					if got == nil || *got != want {
						t.Fatal("presentation retargeted", side, step, got, want)
					}
				}
			}
			m.navigation.mode = "NEW"
			m.fileCache = fileDetailCache{}
			searchInput(m, "context needle")
			namedKey(m, tea.KeyEnter)
			if m.selectedDiffTarget() != nil {
				t.Fatal("read-only source became comment target")
			}
			wantID := m.displayDetail()[m.cursor()].searchID
			m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
			m.resizeList(9)
			if m.displayDetail()[m.cursor()].searchID != wantID {
				t.Fatal("read-only source moved during layout")
			}
			// Clear search entirely: manual source navigation has the same anchor.
			m.search = [2]*diffSearchState{}
			key(m, 'S')
			m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
			m.resizeList(5)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
			m.Update(tea.WindowSizeMsg{Width: 200, Height: 24})
			if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], wantID) || m.selectedDiffTarget() != nil {
				t.Fatal("manual read-only source moved or became target")
			}
			ctrlKey(m, 's')
			if m.Composer != nil {
				t.Fatal("read-only row became suggestion")
			}
			for _, mode := range []string{"OLD", "NEW"} {
				m.navigation.mode = mode
				m.fileCache = fileDetailCache{}
				m.layout = diffLayoutUnified
				m.collapseGenerated = true
				m.selectFile(generated)
				genID := searchSourceID{Unit: m.Session.Inventory.Files[generated].ID + ":" + mode, Row: 30}
				codeID := searchSourceID{Unit: m.Session.Inventory.Files[code].ID + ":" + mode, Row: 1}
				found := false
				for i, row := range m.displayDetail() {
					if row.searchID == genID {
						m.setCursor(i)
						found = true
						break
					}
				}
				if !found {
					t.Fatal("manual generated source row missing", mode)
				}
				m.moveCursor(1)
				if m.Session.UnitFiles[m.Selected] != code || !layoutRowHasSourceID(m.displayDetail()[m.cursor()], codeID) || m.selectedDiffTarget() != nil {
					t.Fatal("crossing collapsed generated source changed raw row", mode)
				}
				key(m, 'S')
				if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], codeID) || m.selectedDiffTarget() != nil {
					t.Fatal("manual source split changed raw row", mode)
				}
			}
			after, _ := json.Marshal(m.Session)
			if string(before) != string(after) {
				t.Fatal("layout changed capture or progress")
			}
			saved, err := store.SaveDraft(ctx, session.DraftKeyFor(meta), 0, d)
			if err != nil {
				t.Fatal(err)
			}
			m.draft.loaded = false
			m.loadDraft(m.reviewTabState)
			m.resizeList(4)
			m.Update(tea.WindowSizeMsg{Width: 180, Height: 22})
			got, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(got, saved) {
				t.Fatal("layout changed private persisted bytes", got, err)
			}
			if err = store.Close(); err != nil {
				t.Fatal(err)
			}
			if err = os.RemoveAll(r.Dir); err != nil {
				t.Fatal(err)
			}
			store, err = session.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			offline, err := store.Load(rec.ID)
			if err != nil {
				t.Fatal(err)
			}
			restored, _ := json.Marshal(offline)
			original, _ := json.Marshal(rec)
			if string(restored) != string(original) {
				t.Fatal("offline capture changed")
			}
			got, err = store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(got, saved) {
				t.Fatal("offline private draft changed", got, err)
			}
			recovered := New(ctx, nil)
			defer recovered.Close()
			recovered.SetLifecycle(store, nil, nil)
			recovered.openReviewTab(offline)
			if recovered.Composer == nil || recovered.Composer.Target != target || !recovered.Composer.Suggestion || recovered.Composer.Before != "new" || recovered.Composer.Draft != "private replacement" {
				t.Fatal("offline suggestion retargeted")
			}
			if general {
				g := draftContent(recovered.reviewTabState).General
				if !reflect.DeepEqual(g, saved.General) {
					t.Fatal("immutable general attempt changed", g)
				}
			}
		})
	}
}

func layoutRowHasSourceID(row diffLine, id searchSourceID) bool {
	if row.searchID == id {
		return true
	}
	if row.sideBySide != nil {
		return row.sideBySide.new != nil && row.sideBySide.new.line.searchID == id || row.sideBySide.old != nil && row.sideBySide.old.line.searchID == id
	}
	return false
}
