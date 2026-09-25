package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/review"
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
