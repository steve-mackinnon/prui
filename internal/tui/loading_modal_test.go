package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
)

func TestLoadingModalRendersBarAboveEscapedNotice(t *testing.T) {
	got := ansi.Strip(renderLoadingModal(60, 12, "Review\ncontent", loadingModal{
		active:     true,
		cancelable: true,
		frame:      3,
		notice:     "Fetching\x1b[31m pinned objects",
	}))

	bar := strings.Index(got, "▱▱▱▰▰▰▰▰▰")
	notice := strings.Index(got, "↳ Fetching\\x1b[31m pinned objects")
	if !strings.Contains(got, "╭") || !strings.Contains(got, "◒  Loading") || bar < 0 || notice < 0 || bar > notice {
		t.Fatalf("modal does not render bar above escaped notice:\n%s", got)
	}
	if !strings.Contains(got, "esc: cancel") {
		t.Fatalf("cancellable modal lacks escape hint:\n%s", got)
	}
	assertFitsViewport(t, got, 60, 12)
}

func TestLoadingModalWrapsLongNoticeWithinCard(t *testing.T) {
	got := ansi.Strip(renderLoadingModal(40, 12, "Review\ncontent", loadingModal{
		active:     true,
		cancelable: true,
		notice:     "Creating OpenAI analyzer for this confirmed guide request...",
	}))

	if !strings.Contains(got, "Creating OpenAI analyzer") || !strings.Contains(got, "confirmed guide") || !strings.Contains(got, "request...") {
		t.Fatalf("modal clipped its wrapped notice:\n%s", got)
	}
	if strings.Count(got, "↳") != 1 {
		t.Fatalf("wrapped notice should have one leading marker:\n%s", got)
	}
	assertFitsViewport(t, got, 40, 12)
}

func TestLoadingModalBoundsWrappedNoticeToViewport(t *testing.T) {
	got := ansi.Strip(renderLoadingModal(40, 8, "Review", loadingModal{
		active:     true,
		cancelable: true,
		notice:     "Creating an analyzer with a notice too long for the available height",
	}))

	if !strings.Contains(got, "…") || !strings.Contains(got, "esc: cancel") || !strings.Contains(got, "╰") {
		t.Fatalf("height-bounded notice lost its overflow or modal chrome:\n%s", got)
	}
	assertFitsViewport(t, got, 40, 8)
}

func TestModelLoadingModalAnimatesAndEscCancelsOnlyCancelableWork(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.Busy = true
	m.notice = "Fetching pinned objects"
	canceled := 0
	m.cancelAction = func() { canceled++ }

	before := ansi.Strip(m.View().Content)
	if !strings.Contains(before, "◐  Working") || !strings.Contains(before, "▰▰▰▰▰▰") || !strings.Contains(before, "esc: cancel") {
		t.Fatalf("busy action does not render the cancellable modal:\n%s", before)
	}
	_, cmd := m.Update(loadingTick{})
	after := ansi.Strip(m.View().Content)
	if cmd == nil || before == after || !strings.Contains(after, "▱▰▰▰▰▰▰") {
		t.Fatalf("loading tick did not advance the modal:\n%s", after)
	}
	namedKey(m, tea.KeyEscape)
	if canceled != 1 {
		t.Fatalf("escape canceled %d times, want once", canceled)
	}

	m.cancelAction = nil
	namedKey(m, tea.KeyEscape)
	if strings.Contains(ansi.Strip(m.View().Content), "esc: cancel") {
		t.Fatal("non-cancellable loading advertises escape cancellation")
	}
	m.Busy = false
	_, cmd = m.Update(loadingTick{})
	if cmd != nil {
		t.Fatal("stale loading tick scheduled another animation")
	}
}

func TestInitialLoadingModalEscapeCancelsLoader(t *testing.T) {
	m := New(context.Background(), func(ctx context.Context, _ func(string)) (*review.Session, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	defer m.Close()
	cmd := m.Init()
	if !strings.Contains(ansi.Strip(m.View().Content), "esc: cancel") {
		t.Fatalf("initial loading does not advertise escape cancellation:\n%s", m.View().Content)
	}
	namedKey(m, tea.KeyEscape)
	m.Update(cmd())
	if m.Loading || !errors.Is(m.Err, context.Canceled) {
		t.Fatalf("escape did not cancel initial loading: loading=%v err=%v", m.Loading, m.Err)
	}
}

func TestLoadingModalAnimatesInProgram(t *testing.T) {
	m := New(context.Background(), func(ctx context.Context, _ func(string)) (*review.Session, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	h := runProgram(t, m)
	h.expect("initial loading modal", func(f programFrame) bool {
		return f.loading && strings.Contains(f.text, "▰▰▰▰▰▰") && strings.Contains(f.text, "esc: cancel")
	})
	h.expect("advanced loading frame", func(f programFrame) bool {
		return f.loading && strings.Contains(f.text, "▱▰▰▰▰▰▰")
	})
	h.p.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
	h.expect("canceled initial loading", func(f programFrame) bool {
		return !f.loading && strings.Contains(f.failure, context.Canceled.Error())
	})
	h.quit()
}

func TestLoadingModalOmitsCancelHintAndFitsTinyViewport(t *testing.T) {
	got := ansi.Strip(renderLoadingModal(10, 2, "Review", loadingModal{
		active: true, notice: "Saving local reading progress...",
	}))
	if strings.Contains(got, "esc: cancel") {
		t.Fatalf("non-cancellable modal advertises cancellation:\n%s", got)
	}
	if !strings.Contains(got, "Saving") {
		t.Fatalf("tiny modal hides its loading notice:\n%s", got)
	}
	assertFitsViewport(t, got, 10, 2)
}

func TestLoadingModalPreservesVisibleBackgroundCells(t *testing.T) {
	for _, size := range [][2]int{{120, 24}, {40, 12}, {40, 8}, {20, 8}} {
		width, height := size[0], size[1]
		t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
			background := markerBackground(width, height)
			modal := loadingModal{active: true, cancelable: true, notice: "Fetching pinned objects"}
			before := background
			during := renderLoadingModal(width, height, background, modal)
			modal.frame++
			updated := renderLoadingModal(width, height, background, modal)
			modal.notice = "Creating an analyzer with a notice that wraps across multiple rows"
			wrapped := renderLoadingModal(width, height, background, modal)
			if width == 40 && height == 12 && modalTop(during) != modalTop(wrapped) {
				t.Fatalf("modal moved vertically as notice wrapped: %d to %d", modalTop(during), modalTop(wrapped))
			}
			after := background
			for _, frame := range []string{during, updated, wrapped} {
				assertBackgroundOutsideModal(t, before, frame, after, width, height)
			}
		})
	}
}

func modalTop(content string) int {
	for row, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "╭") {
			return row
		}
	}
	return -1
}

func TestModelLoadingOverlayKeepsBackgroundCoordinatesThroughResize(t *testing.T) {
	m := New(context.Background(), nil)
	m.Loading = false
	m.Session = screenSession()
	m.notice = "Fetching pinned objects"

	for _, size := range [][2]int{{120, 24}, {40, 12}, {40, 8}} {
		width, height := size[0], size[1]
		m.Busy = true
		m.Update(tea.WindowSizeMsg{Width: width, Height: height})
		m.Busy = false
		before := m.View().Content
		m.Busy = true
		opened := m.View().Content
		m.loadingFrame++
		updated := m.View().Content
		m.Busy = false
		after := m.View().Content
		for _, during := range []string{opened, updated} {
			assertModelBackgroundCells(t, before, during, after, width, height)
		}
	}
	// The compact fallback remains legible after resizing below card size.
	m.Busy = true
	m.Update(tea.WindowSizeMsg{Width: 18, Height: 5})
	compact := ansi.Strip(m.View().Content)
	if !strings.Contains(compact, "Fetching") {
		t.Fatalf("compact resize lost loading notice: %q", compact)
	}
	assertFitsViewport(t, compact, 18, 5)
}

func assertModelBackgroundCells(t *testing.T, before, during, after string, width, height int) {
	t.Helper()
	base, shown, restored := cellCanvas(before, width, height), cellCanvas(during, width, height), cellCanvas(after, width, height)
	left, top := -1, -1
	for y := 0; y < height && top < 0; y++ {
		for x := 0; x < width; x++ {
			if shown.CellAt(x, y).Content == "╭" {
				left, top = x, y
				break
			}
		}
	}
	if top < 0 {
		t.Fatal("modal card missing")
	}
	cardWidth := min(60, max(36, width-4))
	cardWidth = min(cardWidth, width)
	bottom := top
	for bottom < height && shown.CellAt(left, bottom).Content != "╰" {
		bottom++
	}
	if bottom == height {
		t.Fatal("modal bottom missing")
	}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if base.CellAt(x, y).Content != restored.CellAt(x, y).Content {
				t.Fatalf("background not restored at (%d,%d)", x, y)
			}
			if y >= top && y <= bottom && x >= left && x < left+cardWidth {
				continue
			}
			if base.CellAt(x, y).Content != shown.CellAt(x, y).Content {
				t.Fatalf("background shifted at (%d,%d): before %q, during %q", x, y, base.CellAt(x, y).Content, shown.CellAt(x, y).Content)
			}
		}
	}
}

func cellCanvas(content string, width, height int) *lipgloss.Canvas {
	canvas := lipgloss.NewCanvas(width, height)
	uv.NewStyledString(content).Draw(canvas, canvas.Bounds())
	return canvas
}

func markerBackground(width, height int) string {
	lines := make([]string, height)
	for row := range lines {
		lines[row] = fmt.Sprintf("%02d", row) + strings.Repeat(".", width-4) + fmt.Sprintf("%02d", row)
	}
	return strings.Join(lines, "\n")
}

func assertBackgroundOutsideModal(t *testing.T, before, during, after string, width, height int) {
	t.Helper()
	base := strings.Split(before, "\n")
	shown := strings.Split(ansi.Strip(during), "\n")
	restored := strings.Split(after, "\n")
	if len(shown) != height {
		t.Fatalf("modal changed frame height: got %d, want %d", len(shown), height)
	}
	for row := 0; row < height; row++ {
		if restored[row] != base[row] {
			t.Fatalf("row %d was not restored", row)
		}
		left := strings.IndexRune(shown[row], '╭')
		if left < 0 {
			left = strings.IndexRune(shown[row], '│')
		}
		if left < 0 {
			left = strings.IndexRune(shown[row], '╰')
		}
		for col := 0; col < width; col++ {
			if left >= 0 && col >= left && col < left+min(60, max(36, width-4)) {
				continue
			}
			if col >= len([]rune(shown[row])) || []rune(shown[row])[col] != []rune(base[row])[col] {
				t.Fatalf("background moved or vanished at (%d,%d): before %q, during %q", col, row, base[row], shown[row])
			}
		}
	}
}

func assertFitsViewport(t *testing.T, text string, width, height int) {
	t.Helper()
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		t.Fatalf("modal has %d lines, height is %d:\n%s", len(lines), height, text)
	}
	for _, line := range lines {
		if visibleWidth(line) > width {
			t.Fatalf("modal exceeds width %d: %q", width, line)
		}
	}
}
