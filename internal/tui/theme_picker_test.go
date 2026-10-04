package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

func TestThemePickerOpensNavigatesAndCancels(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 100, 60
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)

	key(m, 't')
	if m.top() != pageThemePicker || m.ThemePicker.Index != 2 {
		t.Fatalf("t did not open at active theme: page=%v index=%d", m.top(), m.ThemePicker.Index)
	}
	view := ansi.Strip(m.View().Content)
	for _, entry := range theme.BuiltIns() {
		if !strings.Contains(view, entry.DisplayName) {
			t.Fatalf("picker omitted %q:\n%s", entry.DisplayName, view)
		}
	}
	if !strings.Contains(view, "› Dark") {
		t.Fatalf("picker does not mark active theme without color:\n%s", view)
	}
	if !strings.Contains(view, "enter: apply & save") || !strings.Contains(view, "esc/t: cancel") {
		t.Fatalf("picker lacks color-independent controls:\n%s", view)
	}

	namedKey(m, tea.KeyDown)
	if m.ThemePicker.Index != 3 {
		t.Fatalf("down index = %d, want 3", m.ThemePicker.Index)
	}
	key(m, 'k')
	if m.ThemePicker.Index != 2 {
		t.Fatalf("k index = %d, want 2", m.ThemePicker.Index)
	}
	key(m, 't')
	if m.top() != pageReview || m.theme.Name != theme.Dark {
		t.Fatalf("t did not cancel picker without changing theme: page=%v theme=%q", m.top(), m.theme.Name)
	}
}

func TestThemePickerPersistsBeforeApplyingAndKeepsOverrides(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	var saved string
	m.SetThemeSelectionSaver(func(name string) (theme.PersistResult, error) { saved = name; return theme.PersistResult{}, nil })

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if saved != theme.Light || m.theme.Name != theme.Light || m.top() != pageReview {
		t.Fatalf("selection was not persisted then applied: saved=%q theme=%q page=%v", saved, m.theme.Name, m.top())
	}
	if got := m.theme.Syntax(theme.Added); got != "#112233" {
		t.Fatalf("selection lost valid override: added=%q", got)
	}
}

func TestThemePickerRetainsExplicitOverrideThatMatchesItsOriginalBase(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	// This is dark's built-in Added color. Its provenance still matters: when
	// switching to light, it must remain an explicit override rather than
	// silently adopting light's Added color.
	base, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#7ee787"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) { return theme.PersistResult{}, nil })

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if got := m.theme.Syntax(theme.Added); got != "#7ee787" {
		t.Fatalf("switch lost explicit same-as-base override: added=%q", got)
	}
}

func TestThemePickerPersistsButDoesNotReplaceCLISelectedTheme(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	active, err := theme.Resolve(theme.Dark, map[theme.Token]string{theme.Added: "#112233"})
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(active)
	m.SetThemeSelectionLocked(true)
	var saved string
	m.SetThemeSelectionSaver(func(name string) (theme.PersistResult, error) { saved = name; return theme.PersistResult{}, nil })

	key(m, 't')
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "enter: save for next launch") || strings.Contains(view, "enter: apply & save") {
		t.Fatalf("locked picker advertised the wrong action:\n%s", view)
	}
	m.ThemePicker.Index = 1 // light, persisted for the next launch
	namedKey(m, tea.KeyEnter)
	if saved != theme.Light {
		t.Fatalf("picker persisted %q, want %q", saved, theme.Light)
	}
	if m.theme.Name != theme.Dark || m.theme.Syntax(theme.Added) != "#112233" {
		t.Fatalf("picker replaced CLI-selected live theme: %#v", m.theme)
	}
}

func TestThemePickerAppliesSelectionAfterDurabilityWarning(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) {
		return theme.PersistResult{DurabilityWarning: errors.New("private filesystem detail")}, nil
	})

	key(m, 't')
	m.ThemePicker.Index = 1 // light
	namedKey(m, tea.KeyEnter)
	if m.theme.Name != theme.Light {
		t.Fatalf("durability warning did not apply completed selection: %q", m.theme.Name)
	}
	if !strings.Contains(m.notice, "durability could not be confirmed") || strings.Contains(m.notice, "private filesystem") {
		t.Fatalf("durability warning leaked details or was absent: %q", m.notice)
	}
}

func TestThemePickerPersistenceFailureKeepsThemeAndShowsSafeFeedback(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	base, err := theme.Resolve(theme.Dark, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(base)
	m.SetThemeSelectionSaver(func(string) (theme.PersistResult, error) {
		return theme.PersistResult{}, errors.New("private config contents must not be shown")
	})

	key(m, 't')
	m.ThemePicker.Index = 1
	namedKey(m, tea.KeyEnter)
	if m.theme.Name != theme.Dark || m.top() != pageReview {
		t.Fatalf("failed save changed active theme: theme=%q page=%v", m.theme.Name, m.top())
	}
	if m.ActionError == nil || strings.Contains(m.ActionError.Error(), "private config") {
		t.Fatalf("failed save did not provide safe feedback: %v", m.ActionError)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "theme selection was not saved") {
		t.Fatalf("failed save feedback not visible:\n%s", view)
	}
}

func TestThemePickerYieldsToExistingModalInputAndFitsTinyTerminals(t *testing.T) {
	m := New(context.Background(), nil)
	m.openReviewTab(largeSession(1, 1))
	m.Composer = &commentComposer{}
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole composer input: page=%v", m.top())
	}
	m.Composer = nil
	m.CommentMenu = &commentActionMenu{}
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole action-menu input: page=%v", m.top())
	}
	m.CommentMenu = nil
	m.Busy = true
	key(m, 't')
	if m.top() != pageReview {
		t.Fatalf("theme picker stole loading input: page=%v", m.top())
	}
	m.Busy = false
	key(m, 't')
	for _, size := range [][2]int{{10, 2}, {19, 5}, {20, 6}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(m.View().Content)
		lines := strings.Split(view, "\n")
		if len(lines) > size[1] {
			t.Fatalf("%dx%d picker has %d lines:\n%s", size[0], size[1], len(lines), view)
		}
		for _, line := range lines {
			if visibleWidth(line) > size[0] {
				t.Fatalf("%dx%d picker overflow: %q", size[0], size[1], line)
			}
		}
	}
}

func TestThemePickerIsAvailableOnNavigationPickersOnly(t *testing.T) {
	m := New(context.Background(), nil)
	for _, p := range []page{pagePicker, pageRepositoryPicker, pagePullRequestPicker} {
		m.Stack = []page{p}
		key(m, 't')
		if m.top() != pageThemePicker {
			t.Fatalf("t did not open from picker page %v: stack=%v", p, m.Stack)
		}
		key(m, 't')
	}
	for _, p := range []page{pageHelp, pageURL, pageEvidence, pageGuideConsent} {
		m.Stack = []page{p}
		key(m, 't')
		if m.top() != p {
			t.Fatalf("t unexpectedly opened from page %v: stack=%v", p, m.Stack)
		}
	}
}

func TestThemePickerBoundsAndCandidatePosition(t *testing.T) {
	m := New(context.Background(), nil)
	m.ThemePicker.Index = len(theme.BuiltInNames()) - 1
	for _, size := range [][2]int{{35, 8}, {36, 8}, {60, 18}, {80, 24}, {1, 1}} {
		m.Width, m.Height = size[0], size[1]
		view := ansi.Strip(m.themePickerView())
		lines := strings.Split(view, "\n")
		if len(lines) > m.Height {
			t.Fatalf("%v overflow rows: %s", size, view)
		}
		for _, line := range lines {
			if visibleWidth(line) > m.Width {
				t.Fatalf("%v overflow columns: %q", size, line)
			}
		}
		if m.Width >= 35 && !strings.Contains(view, fmt.Sprintf("%d/%d", themePickerPosition(m.ThemePicker.Index), len(theme.BuiltInNames()))) {
			t.Fatalf("%v missing candidate position: %s", size, view)
		}
		if m.Width >= 36 && m.Height >= 8 && (!strings.Contains(view, "esc/t: cancel") || !strings.Contains(view, "enter: apply & save")) {
			t.Fatalf("%v clipped hints: %s", size, view)
		}
	}
}

func TestThemePickerSampleIsTransientAndIncludesOverrides(t *testing.T) {
	m := New(context.Background(), nil)
	m.colorProfile = colorprofile.TrueColor
	overrides := map[theme.Token]string{theme.Title: "#123456", theme.Foreground: "#abcdef", theme.Background: "#123abc"}
	active, err := theme.Resolve(theme.Dark, overrides)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(active)
	m.Width, m.Height = 80, 24
	m.ThemePicker.Index = len(theme.BuiltInNames()) - 1
	before := m.styleLine(classTitle, "active title")
	view := m.themePickerView()
	for _, word := range []string{"Title", "@@ hunk @@", "+ added", "- removed", "warning", "border", "selected", "focused"} {
		if !strings.Contains(ansi.Strip(view), word) {
			t.Fatalf("missing sample %q: %s", word, ansi.Strip(view))
		}
	}
	if !strings.Contains(view, "18;52;86") {
		t.Fatalf("sample lost title override: %q", view)
	}
	for _, color := range []string{"171;205;239", "18;58;188"} {
		if !strings.Contains(view, color) {
			t.Fatalf("sample lost base channel override %q: %q", color, view)
		}
	}
	if m.theme.Name != active.Name || m.styleLine(classTitle, "active title") != before {
		t.Fatal("sample mutated active palette")
	}
	m.Height = 17
	if strings.Contains(ansi.Strip(m.themePickerView()), "@@ hunk @@") {
		t.Fatal("small picker retained sample")
	}
}

func TestThemePickerScrollMouseUsesVisibleCandidate(t *testing.T) {
	m := New(context.Background(), nil)
	m.Stack = []page{pageThemePicker}
	m.Width, m.Height = 36, 8
	m.ThemePicker.Index = len(theme.BuiltInNames()) - 1
	left, top, _, _ := themePickerBounds(m.Width, m.Height)
	m.mousePickerClick(left+2, top+2)
	if m.ThemePicker.Index != len(theme.BuiltInNames())-1 {
		t.Fatalf("click selected hidden row: %d", m.ThemePicker.Index)
	}
}

func TestThemePickerSampleThresholdAndDimensions(t *testing.T) {
	for _, size := range [][2]int{{59, 18}, {60, 17}, {60, 18}, {60, 19}, {80, 18}, {80, 24}, {36, 8}} {
		for _, profile := range []colorprofile.Profile{colorprofile.Unknown, colorprofile.Ascii, colorprofile.ANSI, colorprofile.TrueColor} {
			m := New(context.Background(), nil)
			m.Width, m.Height = size[0], size[1]
			m.colorProfile = profile
			m.ThemePicker.Index = len(theme.BuiltInNames()) - 1
			view := ansi.Strip(m.themePickerView())
			wantSample := m.Width >= 60 && m.Height >= 18
			if strings.Contains(view, "@@ hunk @@") != wantSample {
				t.Fatalf("size=%v profile=%v unexpected sample visibility", size, profile)
			}
			lines := strings.Split(view, "\n")
			if len(lines) > m.Height {
				t.Fatalf("size=%v profile=%v overflow rows", size, profile)
			}
			for _, line := range lines {
				if visibleWidth(line) > m.Width {
					t.Fatalf("size=%v profile=%v overflow columns: %q", size, profile, line)
				}
			}
			if !strings.Contains(view, "esc/t: cancel") || !strings.Contains(view, "enter: apply & save") {
				t.Fatalf("size=%v profile=%v lost controls", size, profile)
			}
		}
	}
}

func TestThemePickerFinalViewPreservesCandidateInheritedChannels(t *testing.T) {
	for _, tc := range []struct {
		name      string
		candidate string
		overrides map[theme.Token]string
	}{
		{name: "terminal inherits both channels", candidate: theme.Terminal},
		{name: "candidate inherits foreground", candidate: theme.TokyoNight, overrides: map[theme.Token]string{theme.Foreground: "default"}},
		{name: "candidate inherits background", candidate: theme.TokyoNight, overrides: map[theme.Token]string{theme.Background: "default"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(context.Background(), nil)
			active, err := theme.Resolve(theme.CatppuccinMocha, nil)
			if err != nil {
				t.Fatal(err)
			}
			m.SetTheme(active)
			m.themeOverrides = tc.overrides
			m.Width, m.Height = 80, 24
			m.colorProfile = colorprofile.TrueColor
			m.Stack = []page{pageThemePicker}
			for i, name := range theme.BuiltInNames() {
				if name == tc.candidate {
					m.ThemePicker.Index = i
				}
			}
			candidate, err := theme.Resolve(tc.candidate, tc.overrides)
			if err != nil {
				t.Fatal(err)
			}
			before := m.styleLine(classTitle, "active title")
			writes := 0
			m.saveTheme = func(name string) (theme.PersistResult, error) { writes++; return theme.PersistResult{}, nil }
			view := m.View().Content
			cells := canvasCells(view, m.Width, m.Height)
			left, top, modalWidth, _ := themePickerBounds(m.Width, m.Height)
			window := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index)
			sampleY := top + 2 + window.end - window.start
			// The second sample row begins with ordinary text. It must use candidate
			// defaults even when the surrounding frame has explicit Mocha channels.
			for _, x := range []int{left + 2, left + modalWidth - 3} {
				style := cells.CellAt(x, sampleY+1).Style
				if !sameCanvasColor(style.Fg, themeBaseColor(candidate, theme.Foreground)) || !sameCanvasColor(style.Bg, themeBaseColor(candidate, theme.Background)) {
					t.Fatalf("candidate cell (%d,%d) inherited active channels: %#v", x, sampleY+1, style)
				}
			}
			if !sameCanvasColor(cells.CellAt(0, 0).Style.Bg, themeBaseColor(active, theme.Background)) {
				t.Fatal("active frame lost background")
			}
			if m.theme.Name != active.Name || m.styleLine(classTitle, "active title") != before || writes != 0 {
				t.Fatal("candidate mutated active palette or saved config")
			}
			if !reflect.DeepEqual(m.themeOverrides, tc.overrides) {
				t.Fatal("candidate mutated overrides")
			}
		})
	}
}

func TestThemePickerGroupsNavigateInVisualOrder(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 100, 60
	m.Stack = []page{pageThemePicker}
	m.ThemePicker.Index = 0
	view := ansi.Strip(m.themePickerView())
	darkAt, lightAt := strings.Index(view, "── Dark"), strings.Index(view, "── Light")
	if darkAt < 0 || lightAt <= darkAt {
		t.Fatalf("missing ordered group headings: %s", view)
	}
	m.themePickerKey("down")
	if theme.BuiltInNames()[m.ThemePicker.Index] != theme.Dark {
		t.Fatal("first down did not skip Dark heading")
	}
	m.themePickerKey("up")
	if m.ThemePicker.Index != 0 {
		t.Fatal("up did not return terminal")
	}
	m.themePickerKey("up")
	if m.ThemePicker.Index != 0 {
		t.Fatal("up wrapped from terminal")
	}

	// Traverse every selectable entry exactly once, independent of catalog index.
	seen := map[int]bool{m.ThemePicker.Index: true}
	for i := 1; i < len(theme.BuiltInNames()); i++ {
		m.themePickerKey("down")
		if seen[m.ThemePicker.Index] {
			t.Fatalf("revisited candidate %d", m.ThemePicker.Index)
		}
		seen[m.ThemePicker.Index] = true
	}
	last := m.ThemePicker.Index
	m.themePickerKey("down")
	if m.ThemePicker.Index != last {
		t.Fatal("down wrapped")
	}
	if !strings.Contains(ansi.Strip(m.themePickerView()), fmt.Sprintf("%d/%d", len(theme.BuiltInNames()), len(theme.BuiltInNames()))) {
		t.Fatal("position count includes headings or follows catalog order")
	}

	if !strings.Contains(ansi.Strip(m.themePickerView()), "Theme · Light") {
		t.Fatal("title lost selected group")
	}
	m.Width, m.Height = 35, 2
	if !strings.Contains(ansi.Strip(m.themePickerView()), "Light") {
		t.Fatal("compact view lost selected group")
	}
}

func TestThemePickerGroupHeadingsIgnoreMouseSelection(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 100, 60
	m.Stack = []page{pageThemePicker}
	m.ThemePicker.Index = 0
	left, top, _, _ := themePickerBounds(m.Width, m.Height)
	rows := themePickerRows()
	for i, row := range rows {
		if row.index >= 0 {
			continue
		}
		m.mousePickerClick(left+2, top+2+i)
		if m.ThemePicker.Index != 0 {
			t.Fatalf("heading %q selected a theme", row.heading)
		}
	}
	// A regular row still maps to its catalog index after inserted headings.
	for i, row := range rows {
		if row.index == 1 {
			m.mousePickerClick(left+2, top+2+i)
			break
		}
	}
	if m.ThemePicker.Index != 1 {
		t.Fatal("light row click did not select catalog index")
	}
	m.Width, m.Height = 36, 10
	left, top, _, _ = themePickerBounds(m.Width, m.Height)
	window := themePickerLayout(m.Width, m.Height, m.ThemePicker.Index)
	for i := window.start; i < window.end; i++ {
		if rows[i].index >= 0 {
			continue
		}
		m.mousePickerClick(left+2, top+2+i-window.start)
		if m.ThemePicker.Index != 1 {
			t.Fatal("scrolled heading changed selection")
		}
	}
}

func TestThemePickerGroupedLightSampleRemainsIndependent(t *testing.T) {
	m := New(context.Background(), nil)
	active, err := theme.Resolve(theme.CatppuccinMocha, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.SetTheme(active)
	m.Width, m.Height = 60, 18
	m.Stack = []page{pageThemePicker}
	m.colorProfile = colorprofile.TrueColor
	candidateIndex := -1
	for i, entry := range theme.BuiltIns() {
		if entry.Appearance != theme.AppearanceLight {
			continue
		}
		resolved, err := theme.Resolve(entry.Name, nil)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Syntax(theme.Background) != "default" {
			candidateIndex = i
			break
		}
	}
	if candidateIndex < 0 {
		t.Fatal("catalog has no full light palette")
	}
	m.ThemePicker.Index = candidateIndex
	candidate, err := theme.Resolve(theme.BuiltInNames()[candidateIndex], nil)
	if err != nil {
		t.Fatal(err)
	}
	view := m.View().Content
	if !strings.Contains(ansi.Strip(view), "Theme · Light") {
		t.Fatal("light group context missing")
	}
	left, top, _ := themePickerSampleBounds(m.Width, m.Height, m.ThemePicker.Index)
	cell := canvasCells(view, m.Width, m.Height).CellAt(left, top+1)
	if !sameCanvasColor(cell.Style.Fg, themeBaseColor(candidate, theme.Foreground)) || !sameCanvasColor(cell.Style.Bg, themeBaseColor(candidate, theme.Background)) {
		t.Fatalf("light candidate channels = %#v", cell.Style)
	}
	if m.theme.Name != active.Name {
		t.Fatal("light candidate changed active theme")
	}
}

func TestThemePickerEveryGroupedCandidateStaysVisibleOnResize(t *testing.T) {
	m := New(context.Background(), nil)
	m.Stack = []page{pageThemePicker}
	for index, entry := range theme.BuiltIns() {
		m.ThemePicker.Index = index
		for _, size := range [][2]int{{36, 8}, {36, 10}, {60, 18}, {80, 24}} {
			m.Width, m.Height = size[0], size[1]
			view := ansi.Strip(m.themePickerView())
			if !strings.Contains(view, entry.DisplayName) {
				t.Fatalf("candidate %s lost at size %v: %s", entry.Name, size, view)
			}
			if !strings.Contains(view, "Theme · "+themeGroupLabel(entry.Appearance)) {
				t.Fatalf("candidate %s lost group context at size %v", entry.Name, size)
			}
			left, top, _, _ := themePickerBounds(m.Width, m.Height)
			window := themePickerLayout(m.Width, m.Height, index)
			selectedRow := themePickerSelectedRow(themePickerRows(), index)
			m.mousePickerClick(left+2, top+2+selectedRow-window.start)
			if m.ThemePicker.Index != index {
				t.Fatalf("candidate click %s at size %v selected %d", entry.Name, size, m.ThemePicker.Index)
			}
		}
	}
}

func TestThemePickerPreviewsUnderlyingReviewWithoutApplying(t *testing.T) {
	m := New(context.Background(), nil)
	m.Width, m.Height = 140, 60
	m.colorProfile = colorprofile.TrueColor
	m.openReviewTab(largeSession(1, 1))
	base, _ := theme.Resolve(theme.Dark, nil)
	m.SetTheme(base)
	before := m.View().Content
	m.openThemePicker()
	m.ThemePicker.Index = 1
	shown := m.View().Content
	if !strings.Contains(ansi.Strip(shown), "Files") {
		t.Fatal("theme picker hid the review context")
	}
	candidate, _ := theme.Resolve(theme.Light, nil)
	canvas := themeCanvasBuffer(shown, m.Width, m.Height)
	if !reflect.DeepEqual(canvas.CellAt(0, 0).Style.Bg, themeBaseColor(candidate, theme.Background)) {
		t.Fatal("underlying review did not preview the highlighted theme")
	}
	if m.theme.Name != theme.Dark {
		t.Fatal("preview applied theme")
	}
	m.themePickerKey("esc")
	if m.View().Content != before {
		t.Fatal("cancel did not restore original review")
	}
}

func TestThemePickerOverlayPreservesBackgroundCoordinates(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.Ascii, colorprofile.TrueColor} {
		for _, underlying := range []page{pageReview, pagePicker, pageRepositoryPicker, pagePullRequestPicker} {
			m := New(context.Background(), nil)
			m.Width, m.Height = 140, 60
			m.colorProfile = profile
			m.openReviewTab(largeSession(1, 1))
			m.Stack = []page{underlying}
			before := strings.Split(ansi.Strip(m.View().Content), "\n")[0]
			m.openThemePicker()
			shown := strings.Split(ansi.Strip(m.View().Content), "\n")[0]
			if strings.TrimRight(shown, " ") != strings.TrimRight(before, " ") {
				t.Fatalf("background moved for page %v profile %v: %q != %q", underlying, profile, shown, before)
			}
			if m.top() != pageThemePicker {
				t.Fatal("rendering changed modal stack")
			}
		}
	}
}
