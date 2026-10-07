package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"crypto/sha1"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/source"
)

func navigationModel(patch, old, newText string, oldRange, newRange inventory.Range) *Model {
	s := largeTextSession(1, 1)
	f := &s.Inventory.Files[0]
	f.OldPath = append([]byte(nil), f.NewPath...)
	f.OldOID = "old"
	f.NewOID = "new"
	f.OldMode = "100644"
	f.NewMode = "100644"
	s.Inventory.Patches["p"] = []byte(patch)
	s.Inventory.Units[0].OldRange = oldRange
	s.Inventory.Units[0].NewRange = newRange
	s.Inventory.FullSource = &inventory.FullSource{Blobs: map[string][]byte{"old": []byte(old), "new": []byte(newText)}}
	m := largeModel(s, 180, 24)
	m.selectReviewView(viewFiles)
	return m
}

func TestExpandedContextZeroCountAndCanonicalMapping(t *testing.T) {
	for _, tc := range []struct {
		name, patch, old, newText string
		o, n                      inventory.Range
	}{
		{"insert middle", "@@ -1,0 +2 @@\n+x\n", "a\nb\n", "a\nx\nb\n", inventory.Range{Start: 1, Count: 0}, inventory.Range{Start: 2, Count: 1}},
		{"delete middle", "@@ -2 +1,0 @@\n-x\n", "a\nx\nb\n", "a\nb\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 1, Count: 0}},
		{"insert eof", "@@ -2,0 +3 @@\n+x\n", "a\nb\n", "a\nb\nx\n", inventory.Range{Start: 2, Count: 0}, inventory.Range{Start: 3, Count: 1}},
		{"delete eof", "@@ -3 +2,0 @@\n-x\n", "a\nb\nx\n", "a\nb\n", inventory.Range{Start: 3, Count: 1}, inventory.Range{Start: 2, Count: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := navigationModel(tc.patch, tc.old, tc.newText, tc.o, tc.n)
			defer m.Close()
			canonical := unitLines(m.Session, 0)
			var targets []source.ReviewCommentTarget
			for _, l := range canonical {
				if l.target != nil {
					targets = append(targets, *l.target)
				}
			}
			for _, split := range []bool{false, true} {
				for _, wrap := range []bool{false, true} {
					m.navigation.mode = "expanded"
					if split {
						m.layout = diffLayoutSideBySide
					} else {
						m.layout = diffLayoutUnified
					}
					m.Width = 180
					if wrap && !split {
						m.Width = 70
					}
					var got []source.ReviewCommentTarget
					seen := map[string]int{}
					for _, l := range m.navigationDetail(0) {
						if l.Class == classUnavailable {
							t.Fatal(l.Text)
						}
						if l.target != nil {
							got = append(got, *l.target)
						}
						if l.Class == classContext {
							seen[l.rawSource]++
						}
					}
					if !reflect.DeepEqual(got, targets) || seen["a"] != 1 || seen["b"] != 1 {
						t.Fatalf("targets %v / %v, context %v", got, targets, seen)
					}
					for _, l := range m.displayDetail() {
						if l.target != nil && !containsTarget(targets, *l.target) {
							t.Fatal("invented target")
						}
						if l.sideBySide != nil {
							for _, target := range rowTargets(*l.sideBySide) {
								if !containsTarget(targets, target) {
									t.Fatal("invented split target")
								}
							}
						}
					}
				}
			}
			for _, mode := range []string{"OLD", "NEW"} {
				m.navigation.mode = mode
				docs, skipped := searchDocuments(context.Background(), m.Session, searchScope{Source: mode})
				want := tc.old
				if mode == "NEW" {
					want = tc.newText
				}
				if len(docs) != len(strings.Split(strings.TrimSuffix(want, "\n"), "\n")) || skipped != 0 {
					t.Fatal(mode, docs, skipped)
				}
				for _, l := range m.navigationDetail(0) {
					if l.target != nil {
						t.Fatal("full source target")
					}
				}
			}
		})
	}
}
func containsTarget(targets []source.ReviewCommentTarget, target source.ReviewCommentTarget) bool {
	for _, x := range targets {
		if x == target {
			return true
		}
	}
	return false
}

func TestWhitespaceProjectionKeepsProgressAndSearchReveal(t *testing.T) {
	m := navigationModel("@@ -1,2 +1,2 @@\n-a b\n+ab\n tail\n", "a b\ntail\n", "ab\ntail\n", inventory.Range{Start: 1, Count: 2}, inventory.Range{Start: 1, Count: 2})
	defer m.Close()
	m.Session.ReviewedSliceIDs = []string{m.Session.Slices[0].FileID}
	before := append([]string(nil), m.Session.ReviewedSliceIDs...)
	m.codeNavigationKey("ctrl+w")
	if !strings.Contains(joinNavigation(m.navigationDetail(0)), "Whitespace-only change hidden") || !reflect.DeepEqual(before, m.Session.ReviewedSliceIDs) {
		t.Fatal("presentation changed progress or failed to hide")
	}
	searchInput(m, "ab")
	m.activateSearchMatch()
	if m.navigation.whitespace || m.selectedDiffTarget() == nil || m.selectedDiffTarget().Line != 1 {
		t.Fatal("search did not reveal canonical target")
	}
}
func joinNavigation(lines []diffLine) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.Text)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestUnresolvedNavigationUsesAuthoritativeStatusAndPartialCoverage(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	defer m.Close()
	no, yes := false, true
	m.discussions.loaded = true
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{{ID: "resolved", Resolved: &yes}, {ID: "unknown"}, {ID: "open", Resolved: &no}}
	m.nextUnresolved(1)
	if m.discussions.selected != 2 || !m.discussions.detail || !strings.Contains(m.discussions.notice, "Partial coverage") {
		t.Fatal("incorrect unresolved navigation")
	}
	m.nextUnresolved(-1)
	if m.discussions.selected != 2 {
		t.Fatal("unknown treated unresolved")
	}
}

func TestFullSourceUnavailableCoverageAndSearchActivation(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-a\n+b\n", "a\n", "b\nneedle\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	m.codeNavigationKey("alt+n")
	searchInput(m, "needle")
	m.activateSearchMatch()
	if m.selectedDiffTarget() != nil || !m.searchMatchAtCursor() {
		t.Fatal("full search invented anchor or failed to navigate")
	}
	delete(m.Session.Inventory.FullSource.Blobs, "new")
	docs, skipped := searchDocuments(context.Background(), m.Session, searchScope{Source: "NEW"})
	if len(docs) != 0 || skipped != 1 {
		t.Fatal("coverage unavailable not visible")
	}
}

func TestUnresolvedNavigationDeduplicatesTimelineAndSkipsRetained(t *testing.T) {
	m := largeModel(largeTextSession(1, 1), 120, 24)
	defer m.Close()
	no := false
	m.discussions.loaded = true
	m.discussions.snapshot.Snapshot.Timeline = true
	m.discussions.snapshot.Snapshot.Threads = []source.Discussion{
		{ID: "first", Resolved: &no, Comments: []source.ReviewComment{{ID: 1}, {ID: 2, ParentID: 1}}},
		{ID: "stale", Resolved: &no, Retained: true, Comments: []source.ReviewComment{{ID: 3}}},
		{ID: "second", Resolved: &no, Comments: []source.ReviewComment{{ID: 4}, {ID: 5, ParentID: 4}}},
	}
	m.discussions.selected = 0
	m.nextUnresolved(1)
	if m.discussions.selectedID != "inline:4" {
		t.Fatal("did not skip own reply and retained thread", m.discussions.selectedID)
	}
	m.nextUnresolved(1)
	if m.discussions.selectedID != "inline:1" {
		t.Fatal("did not wrap to distinct thread", m.discussions.selectedID)
	}
	m.nextUnresolved(-1)
	if m.discussions.selectedID != "inline:4" {
		t.Fatal("previous navigation revisited reply", m.discussions.selectedID)
	}
}

func TestSourceSearchNextPreviousAndResize(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-a\n+b\n", "a\n", "b\nneedle\nneedle\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	m.codeNavigationKey("alt+n")
	searchInput(m, "needle")
	m.activateSearchMatch()
	keyMsg := func(code rune, mod tea.KeyMod) { m.Update(tea.KeyPressMsg{Code: code, Mod: mod}) }
	keyMsg(tea.KeyF3, 0)
	if m.searchState().selected != 1 || !m.searchMatchAtCursor() {
		t.Fatal("next match")
	}
	keyMsg(tea.KeyF3, tea.ModShift)
	if m.searchState().selected != 0 || !m.searchMatchAtCursor() {
		t.Fatal("previous match")
	}
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	if !m.searchMatchAtCursor() || m.selectedDiffTarget() != nil {
		t.Fatal("resize lost read-only match")
	}
	key(m, 'S')
	if !m.searchMatchAtCursor() || m.selectedDiffTarget() != nil {
		t.Fatal("split fallback lost read-only match")
	}
}

func TestExpandedContextWrappingPreservesCoordinates(t *testing.T) {
	text := strings.Repeat("long context 日本 ", 30)
	m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", text+"\nold\ntail\n", text+"\nnew\ntail\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	m.navigation.mode = "expanded"
	for _, width := range []int{70, 180, 220} {
		m.Width = width
		m.layout = diffLayoutSideBySide
		rows := m.displayDetail()
		sourceParts := 0
		for _, line := range rows {
			cells := []diffLine{line}
			if line.sideBySide != nil {
				cells = nil
				for _, cell := range []*diffCell{line.sideBySide.old, line.sideBySide.new} {
					if cell != nil && cell.line != nil {
						if cell.line.oldLine != 0 && cell.number != cell.line.oldLine || cell.line.newLine != 0 && cell.line.oldLine == 0 && cell.number != cell.line.newLine {
							t.Fatal("split number differs from pinned coordinate", cell)
						}
						cells = append(cells, *cell.line)
					}
				}
			}
			for _, cell := range cells {
				if cell.rawSource == text {
					sourceParts++
					if cell.target != nil || cell.oldLine != 1 || cell.newLine != 1 {
						t.Fatal("wrapped context invented target/changed line")
					}
				}
			}
		}
		if sourceParts < 2 {
			t.Fatal("long context not wrapped", width, sourceParts)
		}
	}
}

func TestNavigationScopeDoesNotLeakIntoGuide(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-a\n+b\n", "a\n", "b\nextra\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	m.codeNavigationKey("alt+n")
	m.selectReviewView(viewGuide)
	if m.currentSearchScope().Source != "" || m.codeNavigationKey("alt+o") {
		t.Fatal("source scope leaked into guide")
	}
	if strings.Contains(joinNavigation(m.cachedFileDetail(false)), "Full NEW") {
		t.Fatal("source projection leaked into guide fallback")
	}
}

func TestCommitSubsetPreservesCanonicalNavigationAndSearch(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-a\n+b\n", "a\n", "b\nextra\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	m.codeNavigationKey("alt+n")
	searchInput(m, "extra")
	m.closeSearchPopovers()
	beforeSession, beforeSelected, beforeNavigation := m.Session, m.Selected, m.navigation
	beforeSearch := m.searchState().selected
	m.commitFilter.subset = true
	if m.searchAvailable() {
		t.Fatal("canonical search offered in a selected commit comparison")
	}
	for _, key := range []string{"alt+o", "ctrl+d", "ctrl+w", "ctrl+e"} {
		if m.codeNavigationKey(key) {
			t.Fatalf("source control %s consumed in selected commit comparison", key)
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyF3})
	if m.Session != beforeSession || m.Selected != beforeSelected || m.navigation != beforeNavigation || m.searchState().selected != beforeSearch {
		t.Fatal("read-only comparison changed canonical navigation/search state")
	}
	m.commitFilter.subset = false
	if !m.searchAvailable() || !m.codeNavigationKey("ctrl+d") {
		t.Fatal("All changes did not restore navigation and search")
	}
}

func TestExpandedSearchCountsUnavailableFilesOnce(t *testing.T) {
	m := navigationModel("@@ -2 +2 @@\n-b\n+x\n", "a\nb\nc\n", "z\nx\ny\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	m.codeNavigationKey("ctrl+e")
	unavailable := 0
	for _, row := range m.navigationDetail(0) {
		if row.Class == classUnavailable {
			unavailable++
		}
	}
	if unavailable < 2 {
		t.Fatal("fixture must contain multiple unavailable regions")
	}
	documents, skipped := searchDocuments(context.Background(), m.Session, m.currentSearchScope())
	if skipped != 1 || len(documents) != 2 {
		t.Fatalf("documents=%d skipped files=%d; want 2, 1", len(documents), skipped)
	}
}

func TestExpandFetchesSelectedFileOnDemand(t *testing.T) {
	m := navigationModel("", "", "", inventory.Range{}, inventory.Range{})
	m.Session.Inventory.FullSource = nil
	f := &m.Session.Inventory.Files[0]
	f.OldOID = fmt.Sprintf("%x", sha1.Sum([]byte("blob 7\x00before\n")))
	f.NewOID = fmt.Sprintf("%x", sha1.Sum([]byte("blob 6\x00after\n")))
	calls := 0
	m.SetFullSourceLoader(func(_ context.Context, f inventory.FileChange, _ source.PinnedComparison) (*inventory.FullSource, error) {
		calls++
		return &inventory.FullSource{Blobs: map[string][]byte{f.OldOID: []byte("before\n"), f.NewOID: []byte("after\n")}}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	if cmd == nil || m.navigation.mode != "expanded" {
		t.Fatal("E did not request expanded source")
	}
	m.Update(cmd())
	if lines, ok := m.navigationInventory().SourceLines(0, false); !ok || lines[0] != "after" {
		t.Fatal("source not loaded", lines)
	}
	m.Update(tea.KeyPressMsg{Code: 'E'})
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'E'})
	if cmd != nil || calls != 1 {
		t.Fatal("cached source fetched again")
	}
	if m.Session.Inventory.FullSource != nil {
		t.Fatal("on-demand source leaked into saved snapshot")
	}
	copy := *m.Session
	copy.Inventory = m.navigationInventory()
	docs, skipped := searchDocuments(context.Background(), &copy, searchScope{Source: "NEW"})
	if skipped != 0 || len(docs) == 0 {
		t.Fatal("loaded source not searchable", skipped)
	}
	if !strings.Contains(renderBindings(groupFooter), "E: expand") {
		t.Fatal("missing footer binding")
	}
}

func TestExpandSourceFailureCanRetryAndIgnoresReplacedSession(t *testing.T) {
	m := navigationModel("", "", "", inventory.Range{}, inventory.Range{})
	m.Session.Inventory.FullSource = nil
	m.SetFullSourceLoader(func(context.Context, inventory.FileChange, source.PinnedComparison) (*inventory.FullSource, error) {
		return nil, fmt.Errorf("offline")
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	m.Update(cmd())
	if m.navigation.loading || m.navigation.sourceError == "" {
		t.Fatal("failure status missing")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'E'})
	if cmd == nil || m.navigation.mode != "expanded" {
		t.Fatal("E did not retry failed source")
	}
	result := cmd()
	m.Session = largeTextSession(1, 1)
	m.navigation = codeNavigation{}
	m.Update(result)
	if m.navigation.sourceError != "" || m.navigation.source != nil {
		t.Fatal("stale source affected replacement session")
	}
}

func TestGuideFullContextToggle(t *testing.T) {
	m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", "class Example {\nold\n};\n", "class Example {\nnew\n};\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	m.Session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Example", Sections: []guide.Section{{Title: "Methods", UnitIDs: []string{m.Session.Inventory.Units[0].ID}}}}}}
	m.selectReviewView(viewGuide)
	before := joinNavigation(m.baseDetail())
	ctrlKey(m, 'e')
	if !m.guideExpanded {
		t.Fatal("guide context toggle unavailable")
	}
	if !strings.Contains(joinNavigation(m.baseDetail()), "class Example") {
		t.Fatal("enclosing class missing")
	}
	for _, l := range m.baseDetail() {
		if l.rawSource == "class Example {" && l.target != nil {
			t.Fatal("context became commentable")
		}
	}
	if !m.codeNavigationKey("ctrl+e") || joinNavigation(m.baseDetail()) != before {
		t.Fatal("compact guide not restored")
	}
}

func TestGuideFullContextOccurrencesAndUnavailable(t *testing.T) {
	m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", "class Example {\nold\n};\n", "class Example {\nnew\n};\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	id := m.Session.Inventory.Units[0].ID
	m.Session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Sections: []guide.Section{{UnitIDs: []string{id, id}}, {UnitIDs: []string{id}}}}}}
	m.selectReviewView(viewGuide)
	m.codeNavigationKey("ctrl+e")
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		m.layout = layout
		d := m.cachedGuideDetail(0)
		if len(d.files) != 2 || len(d.splitFiles) != 2 || strings.Count(joinNavigation(d.lines), "class Example") != 2 {
			t.Fatal("file context duplicated within section or occurrence lost")
		}
		for _, a := range d.files {
			if d.lines[a.offset].guideAnchor == nil {
				t.Fatal("missing navigation boundary")
			}
		}
		if len(m.displayDetail()) == 0 {
			t.Fatal("empty rendered context")
		}
	}
	m.Session.Inventory.FullSource = nil
	m.guideCache = guideDetailCache{}
	if !strings.Contains(joinNavigation(m.baseDetail()), "E: retry") || !strings.Contains(joinNavigation(m.baseDetail()), "+new") {
		t.Fatal("unavailable context lost patch or recovery instruction")
	}
	if !m.codeNavigationKey("ctrl+d") || m.guideExpanded {
		t.Fatal("compact diff control failed")
	}
}

func TestGuideExpandLoadsSelectedSourceOnDemand(t *testing.T) {
	old, newText := "class Example {\nold\n};\n", "class Example {\nnew\n};\n"
	m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", old, newText, inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	f := &m.Session.Inventory.Files[0]
	f.OldPath, f.NewPath = []byte("x.cpp"), []byte("x.cpp")
	oid := func(b string) string {
		return fmt.Sprintf("%x", sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(b), b))))
	}
	f.OldOID, f.NewOID = oid(old), oid(newText)
	m.Session.Inventory.FullSource = nil
	m.Session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Sections: []guide.Section{{UnitIDs: []string{m.Session.Inventory.Units[0].ID}}}}}}
	m.selectReviewView(viewGuide)
	m.SetFullSourceLoader(func(_ context.Context, f inventory.FileChange, _ source.PinnedComparison) (*inventory.FullSource, error) {
		return &inventory.FullSource{Blobs: map[string][]byte{f.OldOID: []byte(old), f.NewOID: []byte(newText)}}, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'E'})
	if !m.guideExpanded || cmd == nil {
		t.Fatal("guide E did not request source")
	}
	if !strings.Contains(joinNavigation(m.baseDetail()), "Loading pinned") {
		t.Fatal("missing loading state")
	}
	m.Update(cmd())
	if !strings.Contains(joinNavigation(m.baseDetail()), "class Example") {
		t.Fatal("guide cache did not refresh with fetched context")
	}
	if m.Session.Inventory.FullSource != nil {
		t.Fatal("guide source mutated saved snapshot")
	}
	foundSyntax := false
	for _, row := range m.baseDetail() {
		if row.rawSource == "class Example {" {
			foundSyntax = len(row.syntax) > 0 && len(row.oldSyntax) > 0
		}
	}
	if !foundSyntax {
		t.Fatal("loaded guide projection lacks source syntax")
	}

	m.Update(tea.KeyPressMsg{Code: 'E'})
	if m.guideExpanded {
		t.Fatal("E did not restore compact guide")
	}
}

func TestCompactNavigationDoesNotLoadInactiveViewSource(t *testing.T) {
	m := navigationModel("", "", "", inventory.Range{}, inventory.Range{})
	defer m.Close()
	m.Session.Inventory.FullSource = nil
	m.navigation.mode = "expanded"
	m.selectReviewView(viewGuide)
	m.SetFullSourceLoader(func(context.Context, inventory.FileChange, source.PinnedComparison) (*inventory.FullSource, error) {
		t.Fatal("compact guide loaded Files source")
		return nil, nil
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if cmd != nil || m.navigation.loading {
		t.Fatal("compact guide started a source request")
	}
}
