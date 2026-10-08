package tui

import (
	tea "charm.land/bubbletea/v2"
	"encoding/json"
	"prui/internal/inventory"
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedCollapseDiscoverabilityAndReveal(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 120, 18)
	m.Session.Inventory.Classifications = map[string]inventory.Classification{}
	for i, f := range m.Session.Inventory.Files {
		c := inventory.Implementation
		if i > 0 {
			c = inventory.Generated
		}
		m.Session.Inventory.Classifications[f.ID] = inventory.Classification{Category: c}
	}
	m.groupFiles = true
	m.collapseGenerated = true
	before := m.Session.Inventory
	selected := m.Selected
	files := m.filteredFiles()
	if !reflect.DeepEqual(files, []int{0}) {
		t.Fatal(files)
	}
	rows, _ := m.reviewListPresentation()
	text := ""
	for _, r := range rows {
		text += r.text + "\n"
	}
	if !strings.Contains(text, "Generated · 2") || !strings.Contains(text, "Alt+C: reveal") {
		t.Fatal(text)
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	if len(m.filteredFiles()) != 3 {
		t.Fatal("missing generated files")
	}
	key(m, 'j')
	if m.Session.UnitFiles[m.Selected] != 1 {
		t.Fatal("generated file unreachable")
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	if m.Session.UnitFiles[m.Selected] != 1 || len(m.filteredFiles()) != 2 {
		t.Fatal("collapse hid selection")
	}
	m.fileFilter = string(m.Session.Inventory.Files[2].NewPath)
	if len(m.filteredFiles()) != 1 {
		t.Fatal("filter did not reveal generated")
	}
	if !reflect.DeepEqual(before, m.Session.Inventory) || selected != 0 {
		t.Fatal("presentation mutated raw inventory")
	}
}

func TestGeneratedBodyCollapsePreservesTargetsAndRawCache(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 200, 24)
	m.Session.Inventory.Classifications = map[string]inventory.Classification{}
	for i, f := range m.Session.Inventory.Files {
		c := inventory.Implementation
		if i == 1 {
			c = inventory.Generated
		}
		m.Session.Inventory.Classifications[f.ID] = inventory.Classification{Category: c}
	}
	raw := append([]diffLine(nil), m.cachedFileDetail(false)...)
	target, commentID := m.cursorAnchor()
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	if !strings.Contains(diffText(m.presentedFileDetail(false)), "Generated content collapsed") {
		t.Fatal("generated body not collapsed")
	}
	if !reflect.DeepEqual(raw, m.cachedFileDetail(false)) {
		t.Fatal("raw source cache changed")
	}
	if after, id := m.cursorAnchor(); !reflect.DeepEqual(after, target) || id != commentID {
		t.Fatal("cursor anchor changed")
	}
	m.selectFile(1)
	if strings.Contains(diffText(m.presentedFileDetail(false)), "Generated content collapsed") {
		t.Fatal("selected generated content not revealed")
	}
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModAlt})
	if !reflect.DeepEqual(raw, m.presentedFileDetail(false)) {
		t.Fatal("reveal lost original rows/targets")
	}
}
func diffText(lines []diffLine) string {
	var s strings.Builder
	for _, l := range lines {
		s.WriteString(l.Text)
		s.WriteByte('\n')
	}
	return s.String()
}

func TestGeneratedCursorCrossingKeepsRawAnchor(t *testing.T) {
	m := largeModel(largeTextSession(3, 3), 200, 24)
	m.Session.Inventory.Classifications = map[string]inventory.Classification{}
	for i, f := range m.Session.Inventory.Files {
		c := inventory.Implementation
		if i == 0 {
			c = inventory.Generated
		}
		m.Session.Inventory.Classifications[f.ID] = inventory.Classification{Category: c}
	}
	m.collapseGenerated = true
	beforeState, _ := json.Marshal(m.Session)
	m.selectFile(0)
	m.Focus = paneDiff
	m.cursorActive = true
	before := m.displayDetail()
	next := m.fileOffset(1)
	for i := next; i < len(before); i++ {
		if before[i].target != nil {
			next = i
			break
		}
	}
	want := *before[next].target
	// moveCursor's selection synchronizer must preserve the new raw anchor when
	// the formerly selected generated file contracts above it.
	for steps := 0; m.Session.UnitFiles[m.Selected] == 0 && steps < len(before); steps++ {
		m.moveCursor(1)
	}
	got, _ := m.cursorAnchor()
	if got == nil || *got != want {
		t.Fatalf("cursor changed across generated boundary: got %+v want %+v index %d", got, want, m.cursor())
	}
	afterState, _ := json.Marshal(m.Session)
	if string(beforeState) != string(afterState) {
		t.Fatal("raw source or reading progress changed")
	}
}

func TestFilesJKKeepsHeaderAcrossGeneratedCollapse(t *testing.T) {
	m := largeModel(largeTextSession(2, 3), 120, 24)
	defer m.Close()
	m.Session.Inventory.Classifications = map[string]inventory.Classification{
		m.Session.Inventory.Files[0].ID: {Category: inventory.Generated},
	}
	m.collapseGenerated = true
	m.selectFile(0)
	m.Focus, m.cursorActive = paneDiff, true
	m.setCursor(m.fileOffset(1) - 1)
	key(m, 'j')
	if got, want := m.cursor(), m.fileOffset(1); got != want {
		t.Fatalf("j lost the file header as generated source collapsed: got %d, want %d", got, want)
	}
	if m.Session.UnitFiles[m.Selected] != 1 {
		t.Fatal("selected file did not follow the header")
	}
	key(m, 'n')
	if target, _ := m.cursorAnchor(); target == nil || target.Path != "file-1" {
		t.Fatalf("n did not reach the new file's comment target: %+v", target)
	}
}
