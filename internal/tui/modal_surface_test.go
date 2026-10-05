package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"prui/internal/theme"
)

func TestModalSurfaceContrastAndPadding(t *testing.T) {
	for _, name := range []string{theme.CatppuccinMocha, theme.GitHubLight, theme.Terminal} {
		t.Run(name, func(t *testing.T) {
			m := New(context.Background(), nil)
			defer m.Close()
			palette, _ := theme.Resolve(name, nil)
			m.SetTheme(palette)
			m.colorProfile = colorprofile.TrueColor
			body := "╭──────╮\n│ text │\n│      │\n╰──────╯"
			cells := themeCanvasBuffer(m.modalSurface(body), 8, 4)
			want := modalBackground(palette)
			for y := 0; y < 4; y++ {
				for x := 0; x < 8; x++ {
					if !sameCanvasColor(cells.CellAt(x, y).Style.Bg, want) {
						t.Fatalf("modal background absent at %d,%d", x, y)
					}
				}
			}
			if base := themeBaseColor(palette, theme.Background); base != nil {
				r, _, _, _ := base.RGBA()
				mr, _, _, _ := want.RGBA()
				if (palette.IsLight() && mr >= r) || (!palette.IsLight() && mr <= r) {
					t.Fatal("modal does not contrast with the page")
				}
			}
			m.colorProfile = colorprofile.Ascii
			if got := m.modalSurface(body); got != body || strings.Contains(got, "\x1b") {
				t.Fatal("plain rendering gained color escapes")
			}
		})
	}
}

func TestAllModalRenderersUseSharedSurface(t *testing.T) {
	m := commitModel(t)
	defer m.Close()
	m.Width, m.Height = 160, 40
	key(m, '2')
	palette, _ := theme.Resolve(theme.CatppuccinMocha, nil)
	m.SetTheme(palette)
	m.colorProfile = colorprofile.TrueColor
	m.openCommitFilter()
	for name, render := range map[string]func() string{
		"commits": func() string { return m.commitFilterModalView("background") },
		"loading": func() string {
			return renderLoadingModal(m.Width, m.Height, "background", loadingModal{title: "Loading", notice: "Working"}, m.modalSurface)
		},
		"action": func() string { return renderActionModal(m.Width, m.Height, "background", "Confirm", m.modalSurface) },
		"consent": func() string {
			return renderGuideConsentModal(m.Width, m.Height, "background", "Consent", m.modalSurface)
		},
		"themes": m.themePickerView,
	} {
		t.Run(name, func(t *testing.T) {
			cells := themeCanvasBuffer(m.themeContentBase(render()), m.Width, m.Height)
			found := false
			for y := 0; y < m.Height; y++ {
				for x := 0; x < m.Width; x++ {
					if sameCanvasColor(cells.CellAt(x, y).Style.Bg, modalBackground(palette)) {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("modal did not use the shared surface")
			}
			if !sameCanvasColor(cells.CellAt(0, 0).Style.Bg, themeBaseColor(palette, theme.Background)) {
				t.Fatal("modal styling leaked into the page background")
			}
		})
	}
}
