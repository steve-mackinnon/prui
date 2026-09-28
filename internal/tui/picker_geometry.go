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

// The theme modal is clipped rather than scrolled. The compact fallback shows
// only the current candidate and therefore has no result list to click.
func themePickerBounds(width, height int) (left, top, modalWidth int, available bool) {
	if width < 20 || height < 6 {
		return 0, 0, 0, false
	}
	modalWidth = min(58, max(36, width-4))
	left, top = themeModalOrigin(width, height, modalWidth, len(theme.BuiltInNames())+7)
	return left, top, modalWidth, true
}

func themeModalOrigin(width, height, modalWidth, rows int) (left, top int) {
	return max(0, (width-modalWidth)/2), max(0, (height-rows)/2)
}
