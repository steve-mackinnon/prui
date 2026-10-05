package tui

import (
	"image"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"prui/internal/inventory"
)

func fileName(f inventory.FileChange) string {
	p := f.NewPath
	if len(p) == 0 {
		p = f.OldPath
	}
	return Escape(path.Base(string(p)))
}

// Preserve the filename first, reserving room for directory context when possible.
func fileRailName(f inventory.FileChange, width int) (string, int) {
	nameWidth := width
	if fileDirectory(f) != "" && width >= 16 {
		nameWidth = width - 8
	}
	return middleTruncate(fileName(f), nameWidth), width
}

func fileDirectory(f inventory.FileChange) string {
	directory := func(p []byte) string {
		d := path.Dir(string(p))
		if d == "." {
			return ""
		}
		return Escape(d)
	}
	p := f.NewPath
	if len(p) == 0 {
		p = f.OldPath
	}
	if len(f.OldPath) > 0 && len(f.NewPath) > 0 && string(f.OldPath) != string(f.NewPath) {
		return Escape(string(f.OldPath)) + " -> " + Escape(string(f.NewPath))
	}
	return directory(p)
}

func (m *Model) filteredFiles() []int {
	if m.Session == nil {
		return nil
	}
	query := strings.ToLower(m.fileFilter)
	files := make([]int, 0, len(m.Session.Inventory.Files))
	for i, f := range m.Session.Inventory.Files {
		if query == "" || strings.Contains(strings.ToLower(string(f.NewPath)), query) || strings.Contains(strings.ToLower(string(f.OldPath)), query) {
			if !m.collapseGenerated || query != "" || m.fileCategory(i) != inventory.Generated || len(m.Session.UnitFiles) > m.Selected && m.Selected >= 0 && m.Session.UnitFiles[m.Selected] == i {
				files = append(files, i)
			}
		}
	}
	if m.groupFiles {
		sort.SliceStable(files, func(i, j int) bool {
			return categoryOrder(m.fileCategory(files[i])) < categoryOrder(m.fileCategory(files[j]))
		})
	}
	return files
}

func (m *Model) fileFilterKey(v tea.KeyPressMsg) {
	switch v.String() {
	case "ctrl+c":
		m.fileFilterEditing = false
	case "esc":
		m.fileFilter, m.fileFilterEditing = "", false
	case "enter":
		m.fileFilterEditing = false
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(m.fileFilter)
		if size > 0 {
			m.fileFilter = m.fileFilter[:len(m.fileFilter)-size]
		}
	default:
		if v.Text != "" && len(m.fileFilter)+len(v.Text) <= 1024 {
			m.fileFilter += v.Text
		}
	}
	files := m.filteredFiles()
	if len(files) > 0 {
		selected := m.Session.UnitFiles[m.Selected]
		for _, f := range files {
			if f == selected {
				return
			}
		}
		m.selectFile(files[0])
	}
}

const fileFilterControl = "▽ Filter (F)"

// Header text and hit bounds share the same width calculation.
func (m *Model) fileFilterHeader() (string, image.Rectangle) {
	prefix := "Files · "
	if m.listWidth() < visibleWidth(prefix+fileFilterControl)+2 {
		prefix = ""
	}
	start := 2 + visibleWidth(prefix)
	end := min(start+visibleWidth(fileFilterControl), 1+m.listWidth())
	label := prefix + fileFilterControl
	if m.groupFiles {
		label += " · grouped"
	}
	return label, image.Rect(start, 2, end, 3)
}

func (m *Model) openFileFilter() {
	m.fileFilterEditing = true
	m.Focus = paneList
}

func categoryOrder(c inventory.Category) int {
	for i, category := range inventory.Categories {
		if c == category {
			return i
		}
	}
	return len(inventory.Categories)
}
func (m *Model) fileCategory(i int) inventory.Category {
	if c, ok := m.Session.Inventory.Classifications[m.Session.Inventory.Files[i].ID]; ok {
		return c.Category
	}
	// Old frozen sessions have no attribute evidence; don't reinterpret the checkout.
	return inventory.Support
}
func (m *Model) categoryCounts() map[inventory.Category]int {
	counts := map[inventory.Category]int{}
	for i := range m.Session.Inventory.Files {
		counts[m.fileCategory(i)]++
	}
	return counts
}
func categoryLabel(c inventory.Category) string {
	switch c {
	case inventory.Implementation:
		return "Implementation"
	case inventory.Tests:
		return "Tests"
	case inventory.Documentation:
		return "Documentation"
	case inventory.Generated:
		return "Generated"
	case inventory.Assets:
		return "Assets"
	default:
		return "Support"
	}
}
