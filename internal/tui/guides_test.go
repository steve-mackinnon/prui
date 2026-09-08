package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

// splitAnalyzer puts one file's units under two different guides, which is the
// case the deterministic file plan cannot represent.
type splitAnalyzer struct{}

func (splitAnalyzer) Analyze(_ context.Context, in guide.Input) (guide.Bundle, error) {
	pick := func(path string, kind inventory.Kind) []string {
		var ids []string
		for _, u := range in.Units {
			if string(u.Path) == path && u.Kind == kind {
				ids = append(ids, u.ID)
			}
		}
		return ids
	}
	return guide.Bundle{Status: guide.Generated, Provider: "openai", Model: "test-model", Items: []guide.Item{
		{Title: "Greeting flow", Description: "Adds a greeting.", Sections: []guide.Section{
			{Title: "Add greeting", Description: "Changes the greeting text.", UnitIDs: pick("a.go", inventory.TextHunk)},
			{Title: "Persist greeting", Description: "Stores the greeting.", UnitIDs: pick("b.go", inventory.TextHunk)},
		}},
		{Title: "File shape", Description: "Mode and identity changes.", Sections: []guide.Section{
			{Title: "Record a.go identity", Description: "Object identity of the changed file.", UnitIDs: pick("a.go", inventory.FileMetadata)},
		}},
	}}, nil
}

func guidedSession(t *testing.T, a guide.Analyzer) (*review.Session, source.Metadata, string) {
	t.Helper()
	r := testutil.NewRepo(t)
	r.Write("a.go", "old\n")
	r.Write("b.go", "old\n")
	base := r.Commit()
	r.Write("a.go", "new\n")
	r.Write("b.go", "new\n")
	r.Write("README.md", "docs\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 42}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	s, e := review.OpenWithConfig(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), nil, review.Config{Analyzer: a})
	if e != nil {
		t.Fatal(e)
	}
	return s, meta, r.Dir
}

func loaded(t *testing.T, s *review.Session, width, height int) *Model {
	t.Helper()
	m := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) { return s, nil })
	t.Cleanup(m.Close)
	m.Update(m.Init()())
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func fileIndex(s *review.Session, path string) int {
	for i, f := range s.Inventory.Files {
		if string(f.NewPath) == path {
			return i
		}
	}
	return -1
}

func TestGuideNavigation(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	if s.Guides == nil || s.Guides.Status != guide.Generated || len(s.Guides.Items) != 3 {
		t.Fatal("fixture did not produce two guides plus the ungrouped coverage guide")
	}
	m := loaded(t, s, 120, 40)
	rows := m.rows()
	if len(rows) == 0 || rows[0].kind != guideRow {
		t.Fatal("guides are not the default hierarchy")
	}
	// Every row must resolve to a real frozen unit, and walking with n/p must
	// reach all of them without leaving the hierarchy.
	for i := range rows {
		if m.Row != i {
			t.Fatalf("row %d unreachable; cursor at %d", i, m.Row)
		}
		if m.Selected < 0 || m.Selected >= len(s.Inventory.Units) {
			t.Fatalf("row %d selected a unit outside the inventory: %d", i, m.Selected)
		}
		if !contains(rows[i].units, m.Selected) {
			t.Fatalf("row %d selected a unit it does not contain", i)
		}
		if !strings.Contains(m.View().Content, string(s.Inventory.Units[m.Selected].Kind)) {
			t.Fatalf("row %d renders no raw unit", i)
		}
		key(m, 'n')
	}
	if m.Row != len(rows)-1 {
		t.Fatal("row navigation ran past the hierarchy")
	}

	shared, guides := fileIndex(s, "a.go"), map[int]bool{}
	for _, r := range rows {
		if r.kind == portionRow && r.file == shared {
			guides[r.guide] = true
		}
	}
	if len(guides) != 2 {
		t.Fatalf("a file split across two sections is reachable from %d guides", len(guides))
	}

	// ]/[ move between guides.
	for m.Row > 0 {
		key(m, 'p')
	}
	m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	if rows[m.Row].kind != guideRow || rows[m.Row].guide != 1 {
		t.Fatal("] did not move to the next guide")
	}
	m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	if rows[m.Row].kind != guideRow || rows[m.Row].guide != 0 {
		t.Fatal("[ did not move to the previous guide")
	}

	// Collapsing must hide the subtree without losing the selected unit.
	key(m, 'n')
	key(m, 'n') // a portion row inside the first section
	unit, before := m.Selected, len(rows)
	for m.Row > 0 {
		key(m, 'p')
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	collapsed := m.rows()
	if len(collapsed) >= before {
		t.Fatal("collapse did not hide the guide subtree")
	}
	if m.Selected != unit {
		t.Fatal("collapse lost the selected unit")
	}
	if !contains(collapsed[m.Row].units, m.Selected) {
		t.Fatal("collapsed cursor no longer describes the selected unit")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if len(m.rows()) != before || m.Selected != unit {
		t.Fatal("expansion did not restore the hierarchy and selection")
	}

	// Sections collapse independently of their guide.
	sectionAt := -1
	for i, r := range m.rows() {
		if r.kind == sectionRow {
			sectionAt = i
			break
		}
	}
	if sectionAt < 0 {
		t.Fatal("no section row")
	}
	for m.Row < sectionAt {
		key(m, 'n')
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if len(m.rows()) >= before || m.rows()[m.Row].kind != sectionRow {
		t.Fatal("section collapse did not hide its portions")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// i remains the complete raw source view, and returning from it keeps the
	// cursor on a row that contains the unit the reviewer was reading.
	key(m, 'i')
	if !strings.Contains(m.View().Content, "Full inventory") {
		t.Fatal("raw inventory unavailable from the guide hierarchy")
	}
	for range s.Inventory.Units {
		key(m, 'p')
	}
	for i := range s.Inventory.Units {
		if m.Selected != i {
			t.Fatalf("raw inventory hides unit %d; selection %d", i, m.Selected)
		}
		key(m, 'n')
	}
	key(m, 'i')
	if !contains(m.rows()[m.Row].units, m.Selected) {
		t.Fatal("returning from the raw inventory left the guide cursor elsewhere")
	}

	// G falls back to the deterministic file plan on demand.
	m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if m.rows() != nil || !strings.Contains(m.View().Content, "File slices") {
		t.Fatal("G did not expose the deterministic file plan")
	}
	m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !strings.Contains(m.View().Content, "Guides") {
		t.Fatal("G did not restore the guide hierarchy")
	}
}

func TestGuideMarking(t *testing.T) {
	s, meta, _ := guidedSession(t, splitAnalyzer{})
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saved, err := store.Create(s.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	m := loaded(t, saved, 200, 40)
	m.SetLifecycle(store, fakeGitHub{meta}, nil)

	shared := fileIndex(saved, "a.go")
	at := -1
	for i, r := range m.rows() {
		if r.kind == portionRow && r.file == shared {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatal("no portion row for the shared file")
	}
	for m.Row < at {
		key(m, 'n')
	}
	if m.rows()[m.Row].kind != portionRow {
		t.Fatal("cursor is not on a file portion")
	}
	action(t, m, 'm')
	id := saved.Inventory.Files[shared].ID
	stored, err := store.Load(saved.ID)
	if err != nil || len(stored.ReviewedSliceIDs) != 1 || stored.ReviewedSliceIDs[0] != id {
		t.Fatal("marking a section did not mark the owning file slice", err, stored.ReviewedSliceIDs)
	}
	rows := m.rows()
	marked := 0
	for _, line := range guideList(m.Session, rows, m.Row, 36, true) {
		if strings.Contains(line.text, "[x] a.go") {
			marked++
		}
	}
	if marked != 2 {
		t.Fatalf("read marker appears on %d of the file's guide rows", marked)
	}
	view := ansi.Strip(m.View().Content)
	// The footer is clipped to the terminal, so the binding text is checked at
	// its source; the header caveat has to survive in the rendered view.
	if !strings.Contains(view, "m marks the whole file slice") || !strings.Contains(m.footer(), "including its units under other guides") {
		t.Fatal("marking caveat absent; a section looks independently completable")
	}
	if !strings.Contains(view, "1/3 read") {
		t.Fatal("progress is not file-slice based", view)
	}
}

func TestGuideDetailAndScroll(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	m := loaded(t, s, 120, 5)
	rows := m.rows()

	for i, r := range rows {
		m.Row = i
		got := m.detail()
		want := detailFor(s, r.guide).lines
		if len(got) != len(want) {
			t.Fatalf("row %d detail length = %d, want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("row %d detail line %d = %#v, want %#v", i, j, got[j], want[j])
			}
		}
	}

	first := detailFor(s, 0)
	if len(first.files) != 2 || first.files[0].offset >= first.files[1].offset {
		t.Fatalf("guide detail anchors = %#v, want ordered a.go and b.go occurrences", first.files)
	}
	for _, anchor := range first.files {
		if first.lines[anchor.offset].Class != classFileHeader {
			t.Fatalf("anchor at %d does not point to a file header", anchor.offset)
		}
	}

	// Combining the fixture's two a.go sections proves repeated occurrences
	// retain separate anchors in guide-defined positions.
	s.Guides.Items[0].Sections = append(s.Guides.Items[0].Sections, s.Guides.Items[1].Sections[0])
	repeated := detailFor(s, 0)
	if len(repeated.files) != 3 || repeated.files[0].file != repeated.files[2].file || repeated.files[0].offset == repeated.files[2].offset {
		t.Fatalf("repeated file anchors = %#v, want distinct a.go occurrences", repeated.files)
	}

	m = loaded(t, s, 120, 5)
	key(m, 'J')
	guide := m.rows()[m.Row].guide
	if m.GuideScroll[guide] == 0 {
		t.Fatal("J did not scroll the active guide")
	}
	firstOffset := m.GuideScroll[guide]
	m.Update(tea.KeyPressMsg{Code: ']', Text: "]"})
	second := m.rows()[m.Row].guide
	if m.GuideScroll[second] != 0 {
		t.Fatal("next guide did not start at offset zero")
	}
	m.Update(tea.KeyPressMsg{Code: '[', Text: "["})
	if m.GuideScroll[guide] != firstOffset {
		t.Fatalf("guide offset = %d, want restored %d", m.GuideScroll[guide], firstOffset)
	}
	for range 100 {
		key(m, 'J')
	}
	last := max(0, len(m.detail())-m.bodyHeight())
	if m.offset() != last {
		t.Fatalf("guide offset = %d, want clamp %d", m.offset(), last)
	}
}

func TestGuideContextShowsSelectedGuideDescriptions(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	m := loaded(t, s, 100, 24)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Adds a greeting.", "Changes the greeting text.", "Stores the greeting."} {
		if !strings.Contains(view, want) {
			t.Fatalf("selected guide context missing %q: %s", want, view)
		}
	}
	if strings.Contains(view, "Mode and identity changes.") {
		t.Fatal("non-selected guide description rendered")
	}
}

func TestGuideContextWrapsAndKeepsRowsNavigable(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	s.Guides.Items[0].Description = "One two three four five six seven eight nine ten."
	m := loaded(t, s, 100, 24)
	rows := m.rows()
	lines := guideList(s, rows, m.Row, 33, true)
	for _, line := range lines {
		if line.row == -1 && visibleWidth(line.text) > 33 {
			t.Fatalf("context line exceeds pane width: %q", line.text)
		}
	}
	context := ""
	for _, line := range lines {
		if line.row == -1 {
			context += line.text + " "
		}
	}
	for _, word := range strings.Fields(s.Guides.Items[0].Description) {
		if !strings.Contains(context, word) {
			t.Fatalf("wrapped context lost %q: %q", word, context)
		}
	}
	key(m, 'j')
	if m.Row != 1 || m.rows()[m.Row].kind != sectionRow {
		t.Fatalf("j landed on row %d, want next selectable section row", m.Row)
	}
}

func TestGuideContextWindowingAndUngroupedDescription(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	s.Guides.Items[0].Description = strings.Repeat("a long guide description ", 12)
	m := loaded(t, s, 60, 5)
	for m.rows()[m.Row].kind != portionRow {
		key(m, 'j')
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "a.go [text_hunk]") {
		t.Fatal("selected file row is not visible after long context")
	}
	ungrouped := -1
	for i, r := range m.rows() {
		if s.Guides.Items[r.guide].Ungrouped {
			ungrouped = i
			break
		}
	}
	if ungrouped < 0 {
		t.Fatal("fixture has no ungrouped guide")
	}
	s.Guides.Items[m.rows()[ungrouped].guide].Description = "synthetic guide description"
	m.Row = ungrouped
	for _, line := range guideList(s, m.rows(), m.Row, 36, true) {
		if strings.Contains(line.text, "synthetic guide description") {
			t.Fatal("ungrouped guide rendered a synthesized description")
		}
	}
}

func TestGuideFallback(t *testing.T) {
	s, _, _ := guidedSession(t, nil)
	if s.Guides == nil || s.Guides.Status != guide.Unavailable {
		t.Fatal("fixture is not an analysis_unavailable session")
	}
	m := loaded(t, s, 120, 40)
	if m.rows() != nil {
		t.Fatal("a fallback bundle produced guide rows")
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "File slices") || !strings.Contains(view, "guides: unavailable") {
		t.Fatal("fallback does not render the deterministic file plan", view)
	}
	key(m, 'n')
	if m.Selected != 1 {
		t.Fatal("fallback navigation is not unit-by-unit")
	}

	for _, b := range []*guide.Bundle{nil, {Status: guide.Generated}} {
		m.Session.Guides = b
		if m.rows() != nil || !strings.Contains(m.View().Content, "File slices") {
			t.Fatalf("bundle %v did not fall back to the file plan", b)
		}
	}
	empty := guide.Fallback("analysis not requested")
	m.Session.Guides = &empty
	if rowsFor(m.Session, newExpansion()) != nil {
		t.Fatal("an unavailable bundle produced rows")
	}
	if rowsFor(nil, newExpansion()) != nil {
		t.Fatal("a missing session produced rows")
	}
}

func contains(units []int, u int) bool {
	for _, v := range units {
		if v == u {
			return true
		}
	}
	return false
}
