package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

func TestThemeCanvasCoversScreenStates(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*Model)
	}{
		{"empty", func(m *Model) {}},
		{"loading", func(m *Model) { m.Loading = true }},
		{"error", func(m *Model) { m.Err = errors.New("synthetic error") }},
		{"review", func(m *Model) { m.openReviewTab(kindsSession()) }},
		{"picker", func(m *Model) { m.Stack = []page{pagePicker} }},
		{"repository", func(m *Model) { m.Stack = []page{pageRepositoryPicker} }},
		{"pull-requests", func(m *Model) { m.Stack = []page{pagePullRequestPicker} }},
		{"help", func(m *Model) { m.Stack = []page{pageHelp} }},
		{"themes", func(m *Model) { m.Stack = []page{pageThemePicker} }},
		{"confirmation", func(m *Model) { m.Stack = []page{pageQuitPending} }},
	}
	for _, preset := range []string{theme.CatppuccinMocha, theme.OneDark, theme.TokyoNight, theme.GruvboxDark} {
		for _, tc := range cases {
			t.Run(preset+"/"+tc.name, func(t *testing.T) {
				m := New(context.Background(), nil)
				defer m.Close()
				tc.setup(m)
				m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
				palette, err := theme.Resolve(preset, nil)
				if err != nil {
					t.Fatal(err)
				}
				m.SetTheme(palette)
				logical := m.View().Content
				m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
				view := m.View()
				if view.ForegroundColor != nil || view.BackgroundColor != nil {
					t.Fatal("theme changed terminal defaults")
				}
				if ansi.Strip(view.Content) != plainCanvas(ansi.Strip(logical), 80, 12) {
					t.Fatal("canvas changed logical content")
				}
				cells := themeCanvasBuffer(view.Content, 80, 12)
				for y := 0; y < 12; y++ {
					for x := 0; x < 80; x++ {
						cell := cells.CellAt(x, y)
						if cell.Width == 0 {
							continue
						}
						if cell.Style.Bg == nil {
							t.Fatalf("unpainted background at %d,%d", x, y)
						}
						if cell.Style.Fg == nil {
							t.Fatalf("unpainted foreground at %d,%d", x, y)
						}
					}
				}
			})
		}
	}
}

func TestThemeCanvasWaitsForColorCapability(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
	m.SetTheme(palette)
	baseline := m.View().Content
	for _, p := range []colorprofile.Profile{colorprofile.Unknown, colorprofile.Ascii, colorprofile.NoTTY} {
		m.Update(tea.ColorProfileMsg{Profile: p})
		if got := m.View().Content; got != baseline {
			t.Fatalf("%s gained canvas padding", p)
		}
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if len(strings.Split(m.View().Content, "\n")) != m.Height {
		t.Fatal("color canvas does not fill height")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.Ascii})
	if m.View().Content != baseline {
		t.Fatal("disabling color retained canvas padding")
	}
}

func TestDescriptionBaseOverrideInvalidatesEveryTab(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	s := kindsSession()
	body := "ordinary paragraph\n\n`code`"
	s.PullRequestDescription = &body
	m.openReviewTab(s)
	base, _ := theme.Resolve(theme.CatppuccinMocha, nil)
	m.SetTheme(base)
	before := m.descriptionLines()
	if !m.descriptionCache.valid {
		t.Fatal("cache not populated")
	}
	override, _ := theme.Resolve(theme.CatppuccinMocha, map[theme.Token]string{theme.Foreground: "default", theme.Background: "default"})
	m.SetTheme(override)
	if m.descriptionCache.valid {
		t.Fatal("same-name base overrides retained cache")
	}
	for _, tab := range m.tabs {
		if tab.review != nil && tab.review.descriptionCache.valid {
			t.Fatal("tab retained cache")
		}
	}
	after := m.descriptionLines()
	if strings.Join(before, "\n") == strings.Join(after, "\n") {
		t.Fatal("changed base palette did not rerender Markdown")
	}
	if ansi.Strip(strings.Join(before, "\n")) != ansi.Strip(strings.Join(after, "\n")) {
		t.Fatal("base override changed Markdown content")
	}
}
