package tui

import (
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// Selection owns a snapshot of visible, escaped cells, never raw review data.
// Keeping it pane-local prevents multiline copies from including the file rail.
type textSelection struct {
	active, moved bool
	start, end    image.Point
	bounds        image.Rectangle
	content       string
	click         tea.Cmd
}

type clipboardToastExpired uint64

func (m *Model) copyText(text string) tea.Cmd {
	m.clipboardToast = true
	m.clipboardToastGeneration++
	generation := m.clipboardToastGeneration
	return tea.Batch(tea.SetClipboard(text), tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return clipboardToastExpired(generation)
	}))
}

func (m *Model) textSelectionBounds() image.Rectangle {
	if !m.mouseAvailable() || m.searchOpen() || m.commitFilter.open {
		return image.Rectangle{}
	}
	switch m.top() {
	case pageReview:
		switch m.selectedReviewView() {
		case viewDescription:
			return image.Rect(0, 3, m.Width, min(m.Height-1, 3+m.overviewHeight()))
		case viewCommits:
			return m.commitGeometry().Detail
		default:
			if m.commitFilter.subset {
				return m.filteredGeometry().Detail
			}
			return m.workspaceGeometry().Detail
		}
	case pageReadiness, pageIssueContext, pageDiscussions:
		return image.Rect(0, 1, m.Width, m.Height-1)
	case pagePullRequestPicker:
		headers := 1
		if m.Session != nil {
			headers = 2
		}
		g := m.prPickerGeometry(headers)
		left := 0
		if g.split {
			left = g.leftWidth + 3
		}
		return image.Rect(left, g.previewTop, m.Width, g.previewTop+g.previewHeight)
	}
	return image.Rectangle{}
}

func (m *Model) mouseTextSelection(msg tea.MouseMsg) (tea.Cmd, bool) {
	event := msg.Mouse()
	p := image.Pt(event.X, event.Y)
	switch msg.(type) {
	case tea.MouseClickMsg:
		m.textSelection = textSelection{}
		bounds := m.textSelectionBounds().Intersect(image.Rect(0, 0, m.Width, m.Height))
		if event.Button == tea.MouseLeft && p.In(bounds) {
			m.textSelection = textSelection{active: true, start: p, end: p, bounds: bounds, content: m.View().Content}
			// Overview rows can expand cards or navigate; a drag must not
			// perform those actions before the gesture is known to be a click.
			if m.top() == pageReview && m.selectedReviewView() == viewDescription {
				if cmd, handled := m.mouseURL(msg); handled {
					m.textSelection.click = cmd
				}
				return nil, true
			}
		}
	case tea.MouseMotionMsg:
		s := &m.textSelection
		if !s.active {
			return nil, false
		}
		if event.Button != tea.MouseLeft {
			m.textSelection = textSelection{}
			return nil, false
		}
		s.end = image.Pt(max(s.bounds.Min.X, min(p.X, s.bounds.Max.X-1)), max(s.bounds.Min.Y, min(p.Y, s.bounds.Max.Y-1)))
		s.moved = s.moved || s.end != s.start
		return nil, s.moved
	case tea.MouseReleaseMsg:
		s := m.textSelection
		m.textSelection = textSelection{}
		if !s.active {
			return nil, false
		}
		if !s.moved {
			if s.click != nil {
				return s.click, true
			}
			if m.top() == pageReview && m.selectedReviewView() == viewDescription {
				return m.mouseReviewClick(s.start.X, s.start.Y), true
			}
			return nil, false
		}
		// Some terminals release with MouseNone; the drag's left button owns this.
		s.end = image.Pt(max(s.bounds.Min.X, min(p.X, s.bounds.Max.X-1)), max(s.bounds.Min.Y, min(p.Y, s.bounds.Max.Y-1)))
		text, _ := s.render(m.Width, m.Height)
		if strings.TrimSpace(text) == "" {
			return nil, true
		}
		return m.copyText(text), true
	case tea.MouseWheelMsg:
		m.textSelection = textSelection{}
	}
	return nil, false
}

// Cell intersections preserve complete wide and combining graphemes, even
// when a selection endpoint lands on a wide character's continuation column.
func (s textSelection) render(width, height int) (string, string) {
	canvas := themeCanvasBuffer(s.content, width, height)
	start, end := s.start, s.end
	if start.Y > end.Y || (start.Y == end.Y && start.X > end.X) {
		start, end = end, start
	}
	var rows []string
	for y := start.Y; y <= end.Y; y++ {
		left, right := s.bounds.Min.X, s.bounds.Max.X
		if y == start.Y {
			left = start.X
		}
		if y == end.Y {
			right = end.X + 1
		}
		var row strings.Builder
		for x := s.bounds.Min.X; x < s.bounds.Max.X; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil || cell.Width == 0 || x >= right || x+cell.Width <= left {
				continue
			}
			row.WriteString(cell.Content)
			cell.Style.Attrs ^= uv.AttrReverse
		}
		rows = append(rows, strings.TrimRight(row.String(), " "))
	}
	return strings.Join(rows, "\n"), canvas.Render()
}

func (m *Model) clipboardPresentation(content string) string {
	if m.textSelection.active && m.textSelection.moved && m.mouseAvailable() {
		_, content = m.textSelection.render(m.Width, m.Height)
	}
	if !m.clipboardToast || m.Width <= 0 || m.Height <= 0 {
		return content
	}
	toast := clip(" copied to clipboard ", m.Width)
	toast = lipgloss.NewStyle().Reverse(true).Render(toast)
	return lipgloss.NewCanvas(m.Width, m.Height).Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(content),
		lipgloss.NewLayer(toast).X(max(0, (m.Width-visibleWidth(toast))/2)).Y(m.Height-1),
	)).Render()
}
