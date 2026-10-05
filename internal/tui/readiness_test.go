package tui

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/json"
	"errors"
	"prui/internal/source"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadinessRefreshRacesAndPinnedCodeIndependence(t *testing.T) {
	m := commitModel(t)
	s := m.Session
	id := s.Inventory.Comparison.Metadata.Identity
	calls := 0
	m.SetReadinessReader(func(_ context.Context, got source.Identity) (source.Readiness, error) {
		calls++
		if got != id {
			t.Fatal("identity")
		}
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("f", 40), BaseSHA: strings.Repeat("b", 40), ObservedAt: time.Now(), HeadVerified: true}, nil
	})
	first := m.refreshReadiness()
	second := m.refreshReadiness()
	m.applyReadinessResult(first().(ReadinessResult))
	if m.readiness.loaded {
		t.Fatal("old refresh accepted")
	}
	m.applyReadinessResult(second().(ReadinessResult))
	if !m.readiness.loaded || m.Session != s || calls != 2 {
		t.Fatal("refresh changed pinned code or lost result")
	}
	text := strings.Join(m.readinessLines(), "\n")
	for _, want := range []string{"Live head differs", s.Inventory.Comparison.Metadata.HeadSHA, strings.Repeat("f", 40), "Required approving reviews: unknown"} {
		if !strings.Contains(text, want) {
			t.Fatal("missing revision/state", want, text)
		}
	}
	old := ReadinessResult{Target: m.activeTab, Session: s, Generation: m.readiness.generation, Snapshot: source.Readiness{Identity: source.Identity{Repository: "wrong/repo", Number: 1}}}
	m.applyReadinessResult(old)
	if !m.readiness.stale {
		t.Fatal("wrong identity accepted")
	}
}
func TestReadinessLoadingFailureAndUnknownNotPassing(t *testing.T) {
	m := commitModel(t)
	m.SetReadinessReader(func(context.Context, source.Identity) (source.Readiness, error) {
		return source.Readiness{}, errors.New("synthetic denied")
	})
	c := m.refreshReadiness()
	if !strings.Contains(strings.Join(m.readinessLines(), "\n"), "Loading · readiness unknown") {
		t.Fatal("loading looked passing")
	}
	m.applyReadinessResult(c().(ReadinessResult))
	if !m.readiness.stale || m.readiness.loading {
		t.Fatal("failure lost")
	}
	m.readiness.loaded = true
	m.readiness.snapshot = source.Readiness{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("a", 40), Checks: []source.ReadinessCheck{{Name: "ci\x1b[31m", Required: "unknown", State: "unknown", SHA: strings.Repeat("a", 40), URL: "https://example.com/failure"}}, Reviews: []source.ReadinessReview{{Author: "alice", Decision: "DISMISSED", SHA: strings.Repeat("b", 40)}}}
	text := strings.Join(m.readinessLines(), "\n")
	for _, want := range []string{"Readiness stale", "unknown", "https://example.com/failure", "older head", "DISMISSED"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Contains(text, "No observed merge blockers") {
		t.Fatal("untrusted or passing output", text)
	}
}
func TestReadinessControlsResizeAndOffline(t *testing.T) {
	m := commitModel(t)
	m.Width = 40
	m.Height = 12
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if m.top() != pageReadiness || !strings.Contains(m.readinessView(), "offline") {
		t.Fatal("readiness unavailable not visible")
	}
	m.readiness.loaded = true
	m.readiness.snapshot = source.Readiness{Identity: m.Session.Inventory.Comparison.Metadata.Identity, HeadSHA: strings.Repeat("f", 40)}
	m.readinessKey("end")
	view := m.readinessView()
	if m.readiness.scroll == 0 || strings.Count(view, "\n") > m.Height {
		t.Fatal("viewport not bounded", view)
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.readiness.scroll < 0 {
		t.Fatal("negative scroll")
	}
	m.readinessKey("home")
	if m.readiness.scroll != 0 {
		t.Fatal("home")
	}
	m.readinessKey("esc")
	if m.top() != pageReview {
		t.Fatal("escape")
	}
}

// Combined main-branch regression: remote readiness must not repin source or
// disturb canonical progress, durable draft state, navigation, or the user's C
// commit-filter control even when an older refresh arrives after a newer one.
func TestReadinessCombinedRefreshPreservesPinnedDraftsNavigationAndCommitFilter(t *testing.T) {
	m := commitModel(t)
	key(m, '2')
	key(m, 'C')
	if !m.commitFilter.open || m.top() != pageReview {
		t.Fatal("C no longer opens captured commit filter")
	}
	key(m, 'C')
	if m.commitFilter.open {
		t.Fatal("C no longer closes captured commit filter")
	}
	m.navigation = codeNavigation{mode: "NEW", whitespace: true}
	m.Selected = 1
	m.Focus = paneDiff
	m.Scroll[1] = 7
	m.Horizontal = 3
	m.Session.ReviewedSliceIDs = []string{"already-read"}
	frozen := m.Session.Inventory.Comparison.Metadata
	m.Pending = []source.ReviewComment{{Target: source.ReviewCommentTarget{Identity: frozen.Identity, CommitID: frozen.HeadSHA, Path: "changed.go", Side: "RIGHT", Line: 2}, Body: "pending draft"}}
	m.ReviewForm = &reviewForm{Body: "review summary", Cursor: 4, Event: 1}
	m.draft = draftState{loaded: true, generation: 12, saved: []byte("saved private draft"), attempt: "review"}
	pinned, err := json.Marshal(m.Session)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := json.Marshal(draftContent(m.reviewTabState))
	if err != nil {
		t.Fatal(err)
	}
	navigation := m.navigation
	reads := 0
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		reads++
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("f", 40), BaseSHA: frozen.BaseSHA, ObservedAt: time.Now(), HeadVerified: true}, nil
	})
	_, first := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if first == nil || m.top() != pageReadiness {
		t.Fatal("Alt+R did not open readiness from Files")
	}
	_, second := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if second == nil {
		t.Fatal("readiness r did not refresh")
	}
	current := second().(ReadinessResult)
	old := first().(ReadinessResult)
	m.Update(current)
	m.Update(old)
	m.readinessKey("esc")
	after, err := json.Marshal(m.Session)
	if err != nil {
		t.Fatal(err)
	}
	afterDrafts, err := json.Marshal(draftContent(m.reviewTabState))
	if err != nil {
		t.Fatal(err)
	}
	if reads != 2 || !m.readiness.loaded || m.readiness.snapshot.HeadSHA != strings.Repeat("f", 40) || m.top() != pageReview {
		t.Fatal("readiness race/control failure")
	}
	if !bytes.Equal(pinned, after) || !bytes.Equal(drafts, afterDrafts) || m.draft.generation != 12 || string(m.draft.saved) != "saved private draft" || m.draft.attempt != "review" {
		t.Fatal("refresh mutated pinned source, progress, or private drafts")
	}
	if m.navigation != navigation || m.Selected != 1 || m.Focus != paneDiff || m.Scroll[1] != 7 || m.Horizontal != 3 {
		t.Fatal("refresh disturbed reading navigation")
	}
	key(m, 'C')
	if !m.commitFilter.open {
		t.Fatal("commit filter unavailable after readiness")
	}
}

func TestReadinessInFlightResultPreservesUncertainThreadEditorAndExclusiveKeys(t *testing.T) {
	m := commitModel(t)
	s := m.Session
	m.SetReadinessReader(func(_ context.Context, id source.Identity) (source.Readiness, error) {
		return source.Readiness{Identity: id, HeadSHA: strings.Repeat("f", 40), ObservedAt: time.Now()}, nil
	})
	request := m.refreshReadiness()
	editor := &publishedEditor{draft: "retain edit", cursor: 4, uncertain: true, attempted: PublishedAction{CommentID: 42, Body: "immutable attempted body"}, notice: "outcome unknown"}
	m.discussions.published = editor
	before := *editor
	m.Update(request())
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if m.Session != s || m.discussions.published != editor || !reflect.DeepEqual(before, *editor) || m.top() == pageReadiness {
		t.Fatal("readiness retargeted or disturbed the uncertain thread editor")
	}
	m.discussions.published = nil
	m.Composer = &commentComposer{Draft: "local inline", Cursor: 3, PendingIndex: -1}
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModAlt})
	if m.Composer == nil || m.Composer.Draft != "local inline" || m.top() == pageReadiness {
		t.Fatal("readiness shortcut stole inline editor input")
	}
}
