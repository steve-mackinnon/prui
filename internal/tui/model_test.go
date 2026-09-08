package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"pr-review/internal/review"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

type fakeGitHub struct{ m source.Metadata }

func (g fakeGitHub) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.m, nil
}
func (g fakeGitHub) Token(context.Context) (string, error) { panic("no live network allowed") }
func key(m *Model, k rune)                                 { m.Update(tea.KeyPressMsg{Code: k, Text: string(k)}) }
func namedKey(m *Model, k rune)                            { m.Update(tea.KeyPressMsg{Code: k}) }
func ctrlKey(m *Model, k rune)                             { m.Update(tea.KeyPressMsg{Code: k, Mod: tea.ModCtrl}) }
func TestRawReviewMockedEndToEnd(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\x1b]52;c;attack\a\n"+strings.Repeat("long line\n", 50))
	r.Write("b", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: base, HeadSHA: head}
	m := New(context.Background(), func(c context.Context, n func(string)) (*review.Session, error) {
		return review.Open(c, r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), n)
	})
	if !strings.Contains(m.View().Content, "Loading") {
		t.Fatal("loading absent")
	}
	m.Update(m.Init()())
	if m.Session == nil || m.Err != nil {
		t.Fatal("load failed", m.Err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	for i := range m.Session.Inventory.Units {
		if m.Selected != i {
			t.Fatal("unit inaccessible", i)
		}
		if !strings.Contains(m.View().Content, string(m.Session.Inventory.Units[i].Kind)) {
			t.Fatal("kind absent")
		}
		key(m, 'n')
	}
	key(m, 'p')
	key(m, 'j')
	selected, scroll := m.Selected, m.Scroll[m.Selected]
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 15})
	if m.Selected != selected || m.Scroll[m.Selected] != scroll {
		t.Fatal("resize lost reading position")
	}
	key(m, 'i')
	if !strings.Contains(m.View().Content, "Full inventory") {
		t.Fatal("inventory unavailable")
	}
	key(m, 'i')
	key(m, '?')
	if !strings.Contains(m.View().Content, "Keyboard") {
		t.Fatal("help unavailable")
	}
	key(m, '?')
	plain := Plain(m.Session)
	if strings.ContainsAny(plain, "\x1b\a") || !strings.Contains(plain, `\x1b]52;c;attack\a`) {
		t.Fatal("unsafe terminal content")
	}
	if !strings.Contains(plain, "inventory complete") || !strings.Contains(plain, "analysis: file fallback") {
		t.Fatal("status missing")
	}
	for _, w := range []int{1, 20, 60, 99, 100, 120} {
		m.Update(tea.WindowSizeMsg{Width: w, Height: 10})
		for _, line := range strings.Split(m.View().Content, "\n") {
			if visibleWidth(line) > w {
				t.Fatalf("viewport exceeded %d: %q", w, line)
			}
		}
	}
}

func TestFullDiffDoesNotScrollPastViewport(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n"+strings.Repeat("long line\n", 50))
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	m := New(context.Background(), func(c context.Context, n func(string)) (*review.Session, error) {
		return review.Open(c, r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), source.Defaults(), n)
	})
	m.Update(m.Init()())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 100})
	for i := range m.Session.Inventory.Units {
		if strings.Contains(unitText(m.Session, i), "long line") {
			m.Selected = i
			break
		}
	}
	ctrlKey(m, 'l')
	if m.Focus != paneDiff {
		t.Fatal("diff did not receive focus")
	}

	lastLine := "long line"
	if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
		t.Fatal("full diff is not initially visible")
	}
	for _, k := range []rune{'j', 'J'} {
		key(m, k)
		if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
			t.Fatalf("%q scrolled past a fully visible diff", k)
		}
	}
	namedKey(m, tea.KeyDown)
	namedKey(m, tea.KeyPgDown)
	if m.Scroll[m.Selected] != 0 || !strings.Contains(m.View().Content, lastLine) {
		t.Fatal("downward paging scrolled past a fully visible diff")
	}
}

func TestRawReviewCancelAndFailure(t *testing.T) {
	canceled := make(chan struct{})
	m := New(context.Background(), func(c context.Context, _ func(string)) (*review.Session, error) {
		<-c.Done()
		close(canceled)
		return nil, c.Err()
	})
	cmd := m.Init()
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	key(m, 'q')
	<-canceled
	m.Update(<-done)
	if !errors.Is(m.Err, context.Canceled) {
		t.Fatal("loading cancellation lost", m.Err)
	}
	failed := New(context.Background(), func(context.Context, func(string)) (*review.Session, error) {
		return nil, errors.New("synthetic failure")
	})
	failed.Update(failed.Init()())
	if !strings.Contains(failed.View().Content, "synthetic failure") || strings.Contains(failed.View().Content, "inventory complete") {
		t.Fatal("failure masquerades as empty")
	}
}

func TestModalPagesOwnInputAndBack(t *testing.T) {
	m := New(context.Background(), nil)
	m.Session = &review.Session{}
	m.Loading = false
	m.Selected = 3

	key(m, '?')
	if m.top() != pageHelp {
		t.Fatal("help did not open")
	}
	key(m, 'n')
	if m.Selected != 3 {
		t.Fatal("help leaked review input")
	}
	key(m, 'g')
	if m.top() != pageHelp {
		t.Fatal("help accepted another page opener")
	}
	key(m, 'q')

	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview {
		t.Fatal("esc did not leave help")
	}
	key(m, 'g')
	if m.top() != pageURL || !strings.Contains(m.View().Content, "esc: back") {
		t.Fatal("URL page or back hint missing")
	}
	namedKey(m, tea.KeyEscape)
	if m.top() != pageReview {
		t.Fatal("esc did not leave URL page")
	}

	m.Focus = paneDiff
	namedKey(m, tea.KeyEscape)
	if m.Focus != paneList || m.top() != pageReview {
		t.Fatal("esc did not return to list focus")
	}
}

func TestBindingsRenderHelpAndFooter(t *testing.T) {
	help := renderBindings(groupHelp)
	footer := renderBindings(groupFooter)
	for _, key := range []string{"j/k", "J/K", "ctrl+h/ctrl+l", "esc", "q/ctrl+c"} {
		if !strings.Contains(help, key) {
			t.Fatalf("binding %q missing from help", key)
		}
	}
	for _, key := range []string{"ctrl+h/ctrl+l", "esc", "q/ctrl+c", "m", "N"} {
		if !strings.Contains(footer, key) {
			t.Fatalf("binding %q missing from footer", key)
		}
	}
}

func TestRawReviewIncomplete(t *testing.T) {
	r := testutil.NewRepo(t)
	base := r.Commit()
	r.Write("large", "too much text\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	l := source.Defaults()
	l.BlobBytes = 1
	s, e := review.Open(context.Background(), r.Dir, meta.Identity, fakeGitHub{meta}, source.NewRunner(), l, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(Plain(s), "INCOMPLETE") || !strings.Contains(Plain(s), "per-blob limit") || strings.Contains(Plain(s), "inventory complete") {
		t.Fatal("incomplete review hidden")
	}
}
