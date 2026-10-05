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
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
	"prui/internal/theme"
)

// Real committed source and SQLite, followed by deletion of the repository,
// exercise word spans together with the durable v3/v4 and layout features.
func TestWordDiffRealCaptureDraftOfflineRecovery(t *testing.T) {
	for _, version := range []int{3, 4} {
		t.Run(map[int]string{3: "suggestion-v3", 4: "general-v4"}[version], func(t *testing.T) {
			ctx := context.Background()
			r := testutil.NewRepo(t)
			old := "\talphaCafé := 12 // 世界 long wrapped source words\n\tbetaValue := 34 // second long wrapped source words\ncontext needle\n"
			newText := "\talphaCafé := 56 // 世界 long wrapped source words\n\tbetaValue := 78 // second long wrapped source words\ncontext needle\n"
			r.Write(".gitattributes", "a-generated.go review-generated\n")
			r.Write("a-generated.go", "generated old\n")
			r.Write("z-code.go", old)
			base := r.Commit()
			r.Write("a-generated.go", "generated new\n")
			r.Write("z-code.go", newText)
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
			line := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: head, Path: "z-code.go", Side: "RIGHT", Line: 1}
			rangeTarget := line
			rangeTarget.StartLine = 1
			rangeTarget.StartSide = "RIGHT"
			rangeTarget.Line = 2
			file := line
			file.Side = ""
			file.Line = 0
			file.SubjectType = "file"
			replacement := "\tprivate café := 99\n\tprivate β := 100"
			body, err := source.SuggestionBody(replacement)
			if err != nil {
				t.Fatal(err)
			}
			before := newText[:len(newText)-len("\ncontext needle\n")]
			d := session.Draft{Version: version, Summary: "private edited summary", Event: 2,
				Composer: &session.DraftEditor{Target: rangeTarget, Body: replacement, Before: before, Suggestion: true, PendingIndex: -1},
				Pending:  []source.ReviewComment{{Target: rangeTarget, Body: "private range"}, {Target: file, Body: "private file"}, {Target: line, Body: body}},
				Attempt:  "comment", Attempted: &session.DraftAttempt{Kind: "comment", Comment: &source.ReviewComment{Target: rangeTarget, Body: "immutable dispatched comment"}},
			}
			if version == 4 {
				d.General = &session.GeneralDraft{Body: "edited general café", Cursor: 4, AttemptedBody: "immutable general attempt", ObservedIDs: []string{"PR comment:12"}, Uncertain: true}
			}
			saved, err := store.SaveDraft(ctx, session.DraftKeyFor(meta), 0, d)
			if err != nil {
				t.Fatal(err)
			}
			snapshotBefore, _ := json.Marshal(rec)
			// Rendering after repository deletion proves the comparison never reads
			// working files and the recovered draft uses the same captured coordinates.
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
			snapshotAfter, _ := json.Marshal(offline)
			if string(snapshotBefore) != string(snapshotAfter) {
				t.Fatal("offline source changed")
			}
			m := New(ctx, nil)
			defer m.Close()
			m.SetLifecycle(store, nil, nil)
			m.openReviewTab(offline)
			m.Width = 180
			m.Height = 24
			m.selectReviewView(viewFiles)
			m.theme, _ = theme.Resolve(theme.CatppuccinMocha, nil)
			m.styles = stylesFor(m.theme)
			m.colorProfile = colorprofile.TrueColor
			if m.Composer == nil || m.Composer.Target != rangeTarget || m.Composer.Before != before || m.Composer.Draft != replacement || !m.Composer.Suggestion {
				t.Fatal("suggestion recovery changed raw target/source/body")
			}
			restored := draftContent(m.reviewTabState)
			restored.Generation = saved.Generation
			if !reflect.DeepEqual(restored, saved) {
				t.Fatal("private draft/immutable attempts changed", restored, saved)
			}
			// The recovery overlay and its private attempted bytes remain intact while
			// a second read-only viewer navigates the same offline capture.
			recovered := m
			_ = recovered.View()
			recovered.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
			recovered.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
			privateAfter := draftContent(recovered.reviewTabState)
			privateAfter.Generation = saved.Generation
			if !reflect.DeepEqual(privateAfter, saved) {
				t.Fatal("recovery rendering changed attempted bytes")
			}
			m = New(ctx, nil)
			defer m.Close()
			m.openReviewTab(offline)
			m.Width, m.Height = 180, 24
			m.selectReviewView(viewFiles)
			m.theme, _ = theme.Resolve(theme.CatppuccinMocha, nil)
			m.styles = stylesFor(m.theme)
			m.colorProfile = colorprofile.TrueColor
			m.Focus = paneDiff
			m.cursorActive = true
			code := -1
			for i, f := range m.Session.Inventory.Files {
				if string(f.NewPath) == line.Path {
					code = i
				}
			}
			if code < 0 {
				t.Fatal("captured file missing")
			}
			m.selectFile(code)
			canonical := m.navigationDetail(code)
			changed := 0
			for _, row := range canonical {
				if len(row.wordChanges) > 0 {
					changed++
					if row.rawSource == "" || row.target == nil {
						t.Fatal("highlight lost raw identity")
					}
				}
			}
			if changed != 4 {
				t.Fatalf("ordinary multiline replacement not highlighted: %d", changed)
			}
			for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
				m.layout = layout
				m.fileCache = fileDetailCache{}
				for _, side := range []string{"LEFT", "RIGHT"} {
					want := line
					want.Side = side
					found := false
					for i, row := range m.displayDetail() {
						targets := sourceLineTargets(row)
						if row.sideBySide != nil {
							targets = rowTargets(*row.sideBySide)
						}
						for _, target := range targets {
							if target == want {
								m.setCursor(i)
								m.setSelectedDiffTarget(&target)
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						t.Fatal("raw anchor missing", layout, side)
					}
					for _, width := range []int{70, 180} {
						m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
						if got := m.selectedDiffTarget(); got == nil || *got != want {
							t.Fatal("resize alone", layout, side, width, got)
						}
						m.resizeList(3)
						if got := m.selectedDiffTarget(); got == nil || *got != want {
							t.Fatal("pane resize alone", layout, side, width, got)
						}
						m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
						got := m.selectedDiffTarget()
						if got == nil || *got != want {
							t.Fatal("resize/layout retargeted raw line", layout, side, width, got)
						}
						for _, pan := range []int{0, 3, 12} {
							m.Horizontal = pan
							_ = m.View()
							if got := m.selectedDiffTarget(); got == nil || *got != want {
								t.Fatal("panning retargeted source", got)
							}
						}
						m.Horizontal = 0
					}
				}
			}
			gotSource, ok := m.targetSource(rangeTarget)
			if !ok || gotSource != before {
				t.Fatal("range suggestion source changed", gotSource)
			}
			m.rangeStart = &line
			end := line
			end.Line = 2
			selected, err := m.completeCommentRange(end)
			if err != nil || selected != rangeTarget {
				t.Fatal("range changed", selected, err)
			}
			m.rangeStart = nil
			m.openFileComposer()
			if m.Composer == nil || m.Composer.Target != file {
				t.Fatal("file comment target changed")
			}
			m.Composer = nil
			// Open a real range suggestion from highlighted source, rather than
			// deriving its Before field from the presentation text.
			m.layout = diffLayoutUnified
			m.fileCache = fileDetailCache{}
			for i, row := range m.displayDetail() {
				if row.target != nil && *row.target == end {
					m.setCursor(i)
					m.setSelectedDiffTarget(row.target)
					break
				}
			}
			m.rangeStart = &line
			m.openSuggestionComposer()
			if m.Composer == nil || m.Composer.Target != rangeTarget || m.Composer.Before != before {
				t.Fatal("word emphasis changed suggestion Before/range")
			}
			m.Composer = nil
			m.rangeStart = nil
			for _, mode := range []string{"OLD", "NEW", "expanded"} {
				m.navigation.mode = mode
				m.fileCache = fileDetailCache{}
				rows := m.navigationDetail(code)
				for _, row := range rows {
					if mode != "expanded" && (row.target != nil || len(row.wordChanges) > 0) {
						t.Fatal("full source became paired/commentable")
					}
				}
			}
			m.navigation.mode = "NEW"
			m.fileCache = fileDetailCache{}
			m.Focus = paneDiff
			// Close recovery overlays in memory to test code search without editing drafts.
			m.Composer = nil
			m.CommentMenu = nil
			m.discussions.editor = nil
			searchInput(m, "context needle")
			namedKey(m, tea.KeyEnter)
			if m.selectedDiffTarget() != nil {
				t.Fatal("full-source search became commentable")
			}
			id := m.displayDetail()[m.cursor()].searchID
			if id.Unit != m.Session.Inventory.Files[code].ID+":NEW" || id.Row != 3 {
				t.Fatal("source search selected wrong canonical row", id)
			}
			if !strings.Contains(ansi.Strip(m.View().Content), "context needle") {
				t.Fatal("source search text missing")
			}
			m.resizeList(4)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
			m.Update(tea.WindowSizeMsg{Width: 180, Height: 24})
			if !layoutRowHasSourceID(m.displayDetail()[m.cursor()], id) {
				t.Fatal("full-source search identity changed")
			}
			got, err := store.LoadDraft(ctx, session.DraftKeyFor(meta))
			if err != nil || !reflect.DeepEqual(got, saved) {
				t.Fatal("render/search mutated SQLite private draft", err)
			}
			snapshotRendered, _ := json.Marshal(m.Session)
			snapshotLoaded, _ := json.Marshal(offline)
			if string(snapshotRendered) != string(snapshotLoaded) {
				t.Fatal("presentation mutated pinned session")
			}
		})
	}
}
