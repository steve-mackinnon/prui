package tui

import "prui/internal/theme"

// pickerWindow is the exact visible result interval, after chrome and optional
// selected-result details have claimed their rows. Bounds are half-open.
type pickerWindow struct {
	headerRows, footerRows, detailRows int
	start, end                         int
}

func layoutPicker(height, headers, rows, details, selected int) pickerWindow {
	height = max(1, height)
	w := pickerWindow{headerRows: min(headers, max(0, height-1))}
	if height > w.headerRows+1 {
		w.footerRows = 1
	}
	capacity := max(1, height-w.headerRows-w.footerRows)
	if rows > 0 && capacity >= details+3 {
		w.detailRows = details
	}
	capacity -= w.detailRows
	selected = max(0, min(selected, rows-1))
	w.start = max(0, selected-capacity+1)
	w.end = min(rows, w.start+capacity)
	return w
}

func (w pickerWindow) itemAt(y int) (int, bool) {
	row := y - w.headerRows
	if row < 0 || row >= w.end-w.start {
		return 0, false
	}
	return w.start + row, true
}

// themePickerRow distinguishes nonselectable headings from catalog indices.
type themePickerRow struct {
	index   int
	heading string
}

func themePickerRows() []themePickerRow {
	entries := theme.BuiltIns()
	rows := make([]themePickerRow, 0, len(entries)+2)
	for _, group := range []theme.Appearance{theme.AppearanceTerminal, theme.AppearanceDark, theme.AppearanceLight} {
		first := true
		for i, entry := range entries {
			if entry.Appearance != group {
				continue
			}
			if first && group != theme.AppearanceTerminal {
				rows = append(rows, themePickerRow{index: -1, heading: themeGroupLabel(group)})
			}
			first = false
			rows = append(rows, themePickerRow{index: i})
		}
	}
	return rows
}

func themeGroupLabel(group theme.Appearance) string {
	switch group {
	case theme.AppearanceDark:
		return "Dark"
	case theme.AppearanceLight:
		return "Light"
	default:
		return "Terminal"
	}
}

func themePickerSelectedRow(rows []themePickerRow, selected int) int {
	for i, row := range rows {
		if row.index == selected {
			return i
		}
	}
	return 0
}

func themePickerPosition(selected int) int {
	position := 0
	for _, row := range themePickerRows() {
		if row.index < 0 {
			continue
		}
		position++
		if row.index == selected {
			return position
		}
	}
	return 1
}

// Theme list rows and mouse targets share the same bounded viewport.
func themePickerLayout(width, height, selected int) pickerWindow {
	sampleRows := 0
	if width >= 60 && height >= 18 {
		sampleRows = 3
	}
	rows := themePickerRows()
	return layoutPicker(max(1, height-5-sampleRows), 1, len(rows), 0, themePickerSelectedRow(rows, selected))
}

func themePickerBounds(width, height int) (left, top, modalWidth int, available bool) {
	if width < 36 || height < 8 {
		return 0, 0, 0, false
	}
	modalWidth = min(78, width-2)
	rows := min(height, len(themePickerRows())+7)
	if width >= 60 && height >= 18 {
		rows = min(height, len(themePickerRows())+10)
	}
	left, top = themeModalOrigin(width, height, modalWidth, rows)
	return left, top, modalWidth, true
}

// themePickerSampleBounds locates the same sample rectangle used by View.
func themePickerSampleBounds(width, height, selected int) (left, top, sampleWidth int) {
	left, top, modalWidth, _ := themePickerBounds(width, height)
	window := themePickerLayout(width, height, selected)
	return left + 2, top + 2 + window.end - window.start, modalWidth - 4
}

func themeModalOrigin(width, height, modalWidth, rows int) (left, top int) {
	return max(0, (width-modalWidth)/2), max(0, (height-rows)/2)
}
