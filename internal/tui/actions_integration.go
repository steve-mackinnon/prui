package tui

import (
	"image"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Actions never competes with a field, a modal, or an in-flight operation.
func (m *Model) actionsAvailable() bool {
	return m.top() == pageReview && m.Session != nil && !m.Loading && !m.Busy && m.Err == nil &&
		m.Composer == nil && m.CommentMenu == nil && m.discussions.editor == nil &&
		m.discussions.published == nil && !m.fileFilterEditing && !m.commitFilter.open && !m.searchOpen() &&
		!m.loadingModal().active
}

type reviewHeaderControl struct {
	label  string
	bounds image.Rectangle
}

// Header controls reserve their cells before the PR title is clipped. Rendering
// and clicks use these same bounds, including narrow terminals.
func (m *Model) reviewHeaderControls() []reviewHeaderControl {
	labels := []string{"Actions [Space]", "Review [R]"}
	if m.Width < 48 {
		labels = labels[:1]
	}
	if m.Width < 18 {
		labels[0] = "Actions"
	}
	width := len(strings.Join(labels, "  "))
	x := max(0, m.Width-width)
	controls := make([]reviewHeaderControl, 0, len(labels))
	for _, label := range labels {
		end := min(m.Width, x+len(label))
		controls = append(controls, reviewHeaderControl{label, image.Rect(x, 0, end, 1)})
		x = end + 2
	}
	return controls
}

func (m *Model) reviewHeader(title string) string {
	controls := m.reviewHeaderControls()
	end := controls[0].bounds.Min.X
	left := clip(title, max(0, end-2))
	line := left + strings.Repeat(" ", max(0, end-visibleWidth(left)))
	for i, control := range controls {
		if i > 0 {
			line += "  "
		}
		line += clip(control.label, control.bounds.Dx())
	}
	return m.styleLine(classTitle, line)
}

func (m *Model) reviewHeaderClick(x, y int) (tea.Cmd, bool) {
	if y != 0 || !m.actionsAvailable() {
		return nil, false
	}
	for i, control := range m.reviewHeaderControls() {
		if !image.Pt(x, y).In(control.bounds) {
			continue
		}
		if i == 0 {
			m.openActions()
		} else {
			m.openReviewForm()
		}
		return nil, true
	}
	return nil, false
}
