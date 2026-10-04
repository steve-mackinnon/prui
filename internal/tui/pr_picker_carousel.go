package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/source"
)

type prCarouselTarget struct {
	identity source.Identity
	title    string
	width    int
}
type prCarouselState struct {
	target                    prCarouselTarget
	offset, pause, generation int
}
type prCarouselTick struct{ generation int }

func nextPRCarouselTick(generation int) tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return prCarouselTick{generation} })
}

func (m *Model) prCarouselTarget() prCarouselTarget {
	if m.top() != pagePullRequestPicker || m.Busy || m.Loading {
		return prCarouselTarget{}
	}
	pr, ok := m.selectedPickerPR()
	if !ok {
		return prCarouselTarget{}
	}
	width := m.prPickerGeometry(1).leftWidth - 2
	if m.Width >= 100 {
		width -= visibleWidth(m.prPickerSuffix(pr, false))
	} else {
		width = min(120, max(0, m.Width-2))
		if width >= 32 {
			statusWidth := visibleWidth("⏳ Checks pending")
			authorWidth := min(20, max(1, width-statusWidth-15))
			width -= 3 + authorWidth + 2 + statusWidth + 2
		}
	}
	title := fmt.Sprintf("#%d  %s", pr.Identity.Number, Escape(pr.Title))
	if width <= 0 || visibleWidth(title) <= width {
		return prCarouselTarget{}
	}
	return prCarouselTarget{identity: pr.Identity, title: title, width: width}
}

// A target change invalidates queued timers; only the selected overflowing title
// animates, leaving metadata columns and neighboring PRs stationary.
func (m *Model) syncPRCarousel() tea.Cmd {
	target := m.prCarouselTarget()
	if target == m.prCarousel.target {
		return nil
	}
	m.prCarousel.target = target
	m.prCarousel.offset, m.prCarousel.pause = 0, 6
	m.prCarousel.generation++
	if target.width > 0 {
		return nextPRCarouselTick(m.prCarousel.generation)
	}
	return nil
}

func (m *Model) advancePRCarousel(tick prCarouselTick) tea.Cmd {
	c := &m.prCarousel
	if tick.generation != c.generation || c.target.width == 0 || c.target != m.prCarouselTarget() {
		return nil
	}
	if c.pause > 0 {
		c.pause--
	} else {
		last := max(0, visibleWidth(c.target.title)-c.target.width)
		if c.offset >= last {
			c.offset = 0
			c.pause = 6
		} else {
			c.offset++
			if c.offset == last {
				c.pause = 6
			}
		}
	}
	return nextPRCarouselTick(c.generation)
}

func (m *Model) prPickerTitle(title string, width int) string {
	offset := 0
	if m.top() == pagePullRequestPicker && title == m.prCarousel.target.title && width == m.prCarousel.target.width {
		offset = m.prCarousel.offset
	}
	if offset > 0 {
		identity, text, ok := strings.Cut(title, "  ")
		prefix := identity + "  "
		if ok && visibleWidth(prefix) < width {
			return prefix + ansi.Cut(text, offset, offset+width-visibleWidth(prefix))
		}
	}
	return ansi.Cut(title, offset, offset+max(0, width))
}

func prCheckAccent(status string) lineClass {
	switch status {
	case "✓", "✅ Checks pass":
		return classAdded
	case "!", "❌ Checks fail":
		return classRemoved
	case "…", "⏳ Checks pending":
		return classWarning
	default:
		return classMetadata
	}
}

func (m *Model) prPickerSuffix(pr source.PullRequest, styled bool) string {
	status := "?"
	switch pr.Checks {
	case source.ChecksPassed:
		status = "✓"
	case source.ChecksFailed:
		status = "!"
	case source.ChecksPending:
		status = "…"
	}
	author := ""
	if pr.Author != "" {
		author = clip("@"+Escape(pr.Author), 12)
	}
	if styled {
		status = m.styleLine(prCheckAccent(status), status)
		author = m.styleLine(classHunk, author)
	}
	if author != "" {
		return "  " + author + " " + status
	}
	return "  " + status
}

func (m *Model) paddedPRPickerTitle(title string, width int) string {
	title = m.prPickerTitle(title, width)
	return title + strings.Repeat(" ", max(0, width-visibleWidth(title)))
}
