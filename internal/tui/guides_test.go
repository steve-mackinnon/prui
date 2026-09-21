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
	completeAction(t, m, m.Init())
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
	if !strings.Contains(ansi.Strip(m.View().Content), "FULL INVENTORY") {
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
	if m.rows() != nil || !strings.Contains(ansi.Strip(m.View().Content), "FILE SLICES") {
		t.Fatal("G did not expose the deterministic file plan")
	}
	m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !strings.Contains(ansi.Strip(m.View().Content), "GUIDES") {
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
	for _, line := range guideList(m.Session, rows, m.Row, 36, true, 0) {
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
	if !strings.Contains(view, "marking: whole files") || !strings.Contains(m.footer(), "including its units under other guides") {
		t.Fatal("marking caveat absent; a section looks independently completable")
	}
	if !strings.Contains(view, "1/3 read") {
		t.Fatal("progress is not file-slice based", view)
	}
}

func TestGuideDetailAndScroll(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	m := loaded(t, s, 60, 5)
	rows := m.rows()

	for i, r := range rows {
		m.Row = i
		got := m.detail()
		want := detailFor(s, r.guide).lines
		if len(got) != len(want) {
			t.Fatalf("row %d detail length = %d, want %d", i, len(got), len(want))
		}
		for j := range want {
			if got[j].styledLine != want[j].styledLine || !sameTarget(got[j].target, want[j].target) {
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

func TestGuideDetailPreservesRawCommentTargets(t *testing.T) {
	s := kindsSession()
	s.Inventory.Comparison.Metadata.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	s.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Text", Sections: []guide.Section{{Title: "Change", UnitIDs: []string{"u-text"}}}}}}

	raw := unitLines(s, 1)
	detail := detailFor(s, 0).lines
	for _, rawLine := range raw {
		if rawLine.target == nil {
			continue
		}
		found := false
		for _, detailLine := range detail {
			if detailLine.Text == rawLine.Text && detailLine.target != nil && *detailLine.target == *rawLine.target {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("guide detail lost raw target for %q: %#v", rawLine.Text, rawLine.target)
		}
	}
	for _, line := range detail {
		if line.Class == classFileHeader && line.Text == pathLabel(s.Inventory.Files[0]) && line.target != nil {
			t.Fatalf("guide boundary is unexpectedly commentable: %#v", line.target)
		}
	}
}

func sameTarget(a, b *source.ReviewCommentTarget) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func TestGuideDetailFileJumps(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	m := loaded(t, s, 60, 5)
	rows := m.rows()

	bRow := -1
	for i, r := range rows {
		if r.kind == portionRow && r.guide == 0 && string(s.Inventory.Files[r.file].NewPath) == "b.go" {
			bRow = i
			break
		}
	}
	if bRow < 0 {
		t.Fatal("no b.go row in first guide")
	}
	b := rows[bRow]
	want, ok := anchorFor(detailFor(s, b.guide), b)
	if !ok {
		t.Fatal("no b.go detail anchor")
	}
	m.Row, m.Selected = bRow, b.units[0]
	namedKey(m, tea.KeyEnter)
	if m.Focus != paneDiff || m.GuideScroll[b.guide] != want {
		t.Fatalf("Enter focus/offset = %v/%d, want diff/%d", m.Focus, m.GuideScroll[b.guide], want)
	}
	view := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(view) < 3 || view[2] != fileDivider(s.Inventory.Files[b.file]) {
		t.Fatalf("jumped diff body = %q, want b.go file divider", view)
	}

	m.Focus = paneList
	m.GuideScroll[0] = 1
	m.Row, m.Selected = 0, rows[0].units[0]
	namedKey(m, tea.KeyEnter)
	if m.Focus != paneDiff || m.GuideScroll[0] != 1 {
		t.Fatal("Enter on a guide row did not preserve its saved offset")
	}

	m.Focus = paneList
	m.Row, m.Selected = bRow, b.units[0]
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focus != paneDiff || m.GuideScroll[b.guide] != want {
		t.Fatal("tab on a file row did not jump and focus the diff")
	}
}

func TestLFromGuidePanePreservesGuideScroll(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	m := loaded(t, s, 120, 5)
	for i, row := range m.rows() {
		if row.kind == portionRow {
			m.Row = i
			break
		}
	}
	guide := m.rows()[m.Row].guide
	m.GuideScroll[guide] = 1

	key(m, 'l')

	if m.Focus != paneDiff {
		t.Fatal("l did not focus the guide diff")
	}
	if got := m.GuideScroll[guide]; got != 1 {
		t.Fatalf("l reset guide scroll to %d, want 1", got)
	}
}

func TestGuideDetailJumpsRepeatedFileOccurrence(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	s.Guides.Items[0].Sections = append(s.Guides.Items[0].Sections, s.Guides.Items[1].Sections[0])
	m := loaded(t, s, 120, 5)
	rows := m.rows()
	occurrences := make([]row, 0, 2)
	secondRow := -1
	for i, r := range rows {
		if r.kind == portionRow && r.guide == 0 && string(s.Inventory.Files[r.file].NewPath) == "a.go" {
			occurrences = append(occurrences, r)
			if len(occurrences) == 2 {
				secondRow = i
			}
		}
	}
	if len(occurrences) != 2 {
		t.Fatalf("a.go occurrences = %d, want 2", len(occurrences))
	}
	first, ok := anchorFor(detailFor(s, 0), occurrences[0])
	if !ok {
		t.Fatal("no first a.go anchor")
	}
	second, ok := anchorFor(detailFor(s, 0), occurrences[1])
	if !ok || second == first {
		t.Fatalf("second a.go anchor = %d, want distinct from %d", second, first)
	}
	if secondRow < 0 {
		t.Fatal("no second a.go row")
	}
	m.Row, m.Selected = secondRow, rows[secondRow].units[0]
	namedKey(m, tea.KeyEnter)
	wantScroll := min(second, max(0, len(m.detail())-m.bodyHeight()))
	if m.GuideScroll[0] != wantScroll {
		t.Fatalf("second a.go jump = %d, want %d", m.GuideScroll[0], wantScroll)
	}
}

func TestSideBySideGuideJumpUsesProjectedRepeatedFileAnchor(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	// The final section repeats a.go. Its anchor must be measured in rendered
	// rows: its paired deletion/addition consumes one split row, not two
	// unified lines.
	s.Guides.Items[0].Sections = append(s.Guides.Items[0].Sections,
		s.Guides.Items[1].Sections[0],
		s.Guides.Items[0].Sections[1],
	)
	m := loaded(t, s, sideBySideMinimumWidth, 3)
	key(m, 'S')

	rows := m.rows()
	occurrences := make([]int, 0, 2)
	for i, r := range rows {
		if r.kind == portionRow && r.guide == 0 && string(s.Inventory.Files[r.file].NewPath) == "a.go" {
			occurrences = append(occurrences, i)
		}
	}
	if len(occurrences) != 2 {
		t.Fatalf("a.go occurrences = %d, want 2", len(occurrences))
	}
	m.Row, m.Selected = occurrences[1], rows[occurrences[1]].units[0]

	want := -1
	for i, line := range m.displayDetail() {
		if line.sideBySide != nil && line.sideBySide.full != nil && line.sideBySide.full.Text == fileDivider(s.Inventory.Files[rows[occurrences[1]].file]) {
			want = i
		}
	}
	if want < 0 {
		t.Fatal("repeated file divider absent from split guide detail")
	}
	namedKey(m, tea.KeyEnter)
	if got := m.GuideScroll[0]; got != want {
		t.Fatalf("split repeated a.go jump = %d, want rendered anchor %d", got, want)
	}
}

func TestGuideFallbackEnterOnlyFocusesDiff(t *testing.T) {
	s, _, _ := guidedSession(t, nil)
	m := loaded(t, s, 120, 5)
	m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !m.Files || m.rows() != nil {
		t.Fatal("G did not retain the deterministic file plan")
	}
	m.Scroll[m.Selected] = 1
	namedKey(m, tea.KeyEnter)
	if m.Focus != paneDiff || m.Scroll[m.Selected] != 1 {
		t.Fatal("file plan Enter changed its unit scroll")
	}
	key(m, 'i')
	m.Focus = paneList
	m.Scroll[m.Selected] = 1
	namedKey(m, tea.KeyEnter)
	if m.Focus != paneDiff || m.Scroll[m.Selected] != 1 {
		t.Fatal("inventory Enter changed its unit scroll")
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
	lines := guideList(s, rows, m.Row, 33, true, 0)
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

func TestGuidePathMiddleTruncationPreservesBothEndsAndDisplayWidth(t *testing.T) {
	const path = `internal/非常に長いディレクトリ/feature/guide-overflow_test.go`
	got := middleTruncate(path, 30)
	if got == path {
		t.Fatal("long guide path was not truncated")
	}
	if !strings.Contains(got, "…") || !strings.HasPrefix(got, "internal/") || !strings.HasSuffix(got, "overflow_test.go") {
		t.Fatalf("middle truncation = %q, want both path ends and an ellipsis", got)
	}
	if visibleWidth(got) > 30 {
		t.Fatalf("middle truncation width = %d, want <= 30: %q", visibleWidth(got), got)
	}
}

func TestGuideListMiddleTruncatesUnselectedFilePathsWithoutDroppingSuffix(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	file := fileIndex(s, "a.go")
	s.Inventory.Files[file].NewPath = []byte("internal/very/long/guide/path/overflow_test.go")
	rows := rowsFor(s, newExpansion())
	lines := guideList(s, rows, 0, 45, true, 0)
	for _, line := range lines {
		if line.row < 0 || rows[line.row].kind != portionRow || rows[line.row].file != file {
			continue
		}
		if !strings.Contains(line.text, "…") || !strings.HasSuffix(line.text, " [text_hunk]") {
			t.Fatalf("guide file row = %q, want middle-truncated path with suffix", line.text)
		}
		if visibleWidth(line.text) > 45 {
			t.Fatalf("guide file row width = %d, want <= 45: %q", visibleWidth(line.text), line.text)
		}
		return
	}
	t.Fatal("long guide file row not found")
}

func TestSelectedOverflowingGuidePathAdvancesOnlyWhileListFocused(t *testing.T) {
	s, _, _ := guidedSession(t, splitAnalyzer{})
	file := fileIndex(s, "a.go")
	s.Inventory.Files[file].NewPath = []byte("internal/very/long/guide/path/that/must/scroll/overflow_test.go")
	m := loaded(t, s, 120, 24)
	for m.rows()[m.Row].kind != portionRow || m.rows()[m.Row].file != file {
		key(m, 'j')
	}
	if !m.guidePathScrollEligible() {
		t.Fatal("selected overflowing guide path is not eligible to scroll")
	}
	_, cmd := m.Update(guidePathTick{generation: m.guidePathGeneration})
	if m.guidePathOffset != 0 || cmd == nil {
		t.Fatalf("first guide path tick offset/command = %d/%v, want 0/non-nil", m.guidePathOffset, cmd)
	}
	_, cmd = m.Update(guidePathTick{generation: m.guidePathGeneration})
	if m.guidePathOffset != 0 || cmd == nil {
		t.Fatalf("second guide path tick offset/command = %d/%v, want 0/non-nil", m.guidePathOffset, cmd)
	}
	_, cmd = m.Update(guidePathTick{generation: m.guidePathGeneration})
	if m.guidePathOffset != 1 || cmd == nil {
		t.Fatalf("guide path tick offset/command = %d/%v, want 1/non-nil", m.guidePathOffset, cmd)
	}
	m.Focus = paneDiff
	_, cmd = m.Update(guidePathTick{generation: m.guidePathGeneration})
	if cmd != nil || m.guidePathOffset != 1 {
		t.Fatalf("unfocused guide path tick offset/command = %d/%v, want 1/nil", m.guidePathOffset, cmd)
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
	for _, line := range guideList(s, m.rows(), m.Row, 36, true, 0) {
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
	if !strings.Contains(view, "FILE SLICES") || !strings.Contains(view, "Guides unavailable") {
		t.Fatal("fallback does not render the deterministic file plan", view)
	}
	key(m, 'n')
	if m.Selected != 1 {
		t.Fatal("fallback navigation is not unit-by-unit")
	}

	for _, b := range []*guide.Bundle{nil, {Status: guide.Generated}} {
		m.Session.Guides = b
		if m.rows() != nil || !strings.Contains(ansi.Strip(m.View().Content), "FILE SLICES") {
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
