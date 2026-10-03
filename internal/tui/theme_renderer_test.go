package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/theme"
)

type themeRenderChange struct {
	name, marker  string
	width, height int
}
type themeRenderFrame struct {
	profile       colorprofile.Profile
	content       string
	width, height int
}
type themeRenderModel struct {
	*Model
	frames chan themeRenderFrame
}

func (m themeRenderModel) Init() tea.Cmd { return nil }
func (m themeRenderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if change, ok := msg.(themeRenderChange); ok {
		resolved, _ := theme.Resolve(change.name, nil)
		m.SetTheme(resolved)
		m.Loading = false
		m.Err = errors.New(change.marker)
		_, _ = m.Model.Update(tea.WindowSizeMsg{Width: change.width, Height: change.height})
	} else {
		_, _ = m.Model.Update(msg)
	}
	return m, nil
}
func (m themeRenderModel) View() tea.View {
	view := m.Model.View()
	m.frames <- themeRenderFrame{m.colorProfile, view.Content, m.Width, m.Height}
	return view
}

type themeRenderOutput struct {
	sync.Mutex
	text    strings.Builder
	updates chan struct{}
}

func (o *themeRenderOutput) Write(p []byte) (int, error) {
	o.Lock()
	n, _ := o.text.Write(p)
	o.Unlock()
	select {
	case o.updates <- struct{}{}:
	default:
	}
	return n, nil
}
func (o *themeRenderOutput) snapshot() string { o.Lock(); defer o.Unlock(); return o.text.String() }

func TestThemeRendererProfilesAndRepainting(t *testing.T) {
	for _, profile := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.ANSI256, colorprofile.ANSI, colorprofile.ASCII, colorprofile.NoTTY} {
		t.Run(profile.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			model := New(ctx, nil)
			model.Loading = false
			model.Err = errors.New("INITIAL")
			model.Width, model.Height = 40, 6
			resolved, _ := theme.Resolve("catppuccin-mocha", nil)
			model.SetTheme(resolved)
			output := &themeRenderOutput{updates: make(chan struct{}, 64)}
			frames := make(chan themeRenderFrame, 128)
			program := tea.NewProgram(themeRenderModel{model, frames}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(output), tea.WithoutSignalHandler(), tea.WithWindowSize(40, 6), tea.WithColorProfile(profile), tea.WithEnvironment([]string{"TERM=xterm-256color"}))
			done := make(chan error, 1)
			go func() { _, err := program.Run(); done <- err }()
			t.Cleanup(func() {
				program.Kill()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("renderer did not stop")
				}
				model.Close()
			})
			waitFrame := func(marker string, width int) themeRenderFrame {
				t.Helper()
				for {
					select {
					case f := <-frames:

						if f.profile <= colorprofile.ASCII && strings.HasSuffix(ansi.Strip(f.content), " ") {
							t.Fatal("startup/colorless frame acquired canvas padding")
						}
						if f.profile == profile && f.width == width && strings.Contains(ansi.Strip(f.content), marker) {
							return f
						}
					case <-ctx.Done():
						t.Fatalf("no logical frame for %q", marker)
					}
				}
			}
			waitOutput := func(marker string) string {
				t.Helper()
				for {
					captured := output.snapshot()
					if strings.Contains(ansi.Strip(captured), marker) {
						return captured
					}
					select {
					case <-output.updates:
					case <-ctx.Done():
						t.Fatalf("no renderer output for %q: %q", marker, captured)
					}
				}
			}
			initial := waitFrame("INITIAL", 40)
			waitOutput("INITIAL")
			if profile <= colorprofile.ASCII && (strings.HasSuffix(ansi.Strip(initial.content), " ") || len(strings.Split(initial.content, "\n")) != 3) {
				t.Fatalf("colorless initial content padded: %q", initial.content)
			}
			program.Send(themeRenderChange{"tokyo-night", "FIRST-THEME", 40, 6})
			waitFrame("FIRST-THEME", 40)
			first := waitOutput("FIRST-THEME")
			checkThemeRendererColors(t, first, profile)
			program.Send(themeRenderChange{"gruvbox-dark", "RESIZED", 32, 5})
			resized := waitFrame("RESIZED", 32)
			captured := waitOutput("RESIZED")
			checkThemeRendererColors(t, captured, profile)
			if profile > colorprofile.ASCII {
				cells := replayThemeTerminal(captured, 40, 6)
				finalTheme, _ := theme.Resolve("gruvbox-dark", nil)
				fg, _ := finalTheme.Color(theme.Foreground)
				bg, _ := finalTheme.Color(theme.Background)
				for y := 0; y < 5; y++ {
					for x := 0; x < 32; x++ {
						cell := cells.CellAt(x, y)
						if !sameCanvasColor(cell.Style.Fg, profile.Convert(fg)) || !sameCanvasColor(cell.Style.Bg, profile.Convert(bg)) {
							t.Fatalf("rendered cell (%d,%d) retained stale/missing colors: %#v\noutput=%q", x, y, cell.Style, captured)
						}
					}
				}
			}

			if profile > colorprofile.ASCII {
				if lines := strings.Split(ansi.Strip(resized.content), "\n"); len(lines) != 5 || ansi.StringWidth(lines[0]) != 32 {
					t.Fatalf("resized logical canvas dimensions: %q", resized.content)
				}
			} else if strings.HasSuffix(resized.content, " ") {
				t.Fatal("colorless resize padded content")
			}

			program.Send(themeRenderChange{"terminal", "INHERITED", 32, 5})
			inherited := waitFrame("INHERITED", 32)
			captured = waitOutput("INHERITED")
			if len(strings.Split(inherited.content, "\n")) != 3 {
				t.Fatal("inherited palette retained canvas padding")
			}
			if profile > colorprofile.ASCII {
				cells := replayThemeTerminal(captured, 40, 6)
				for y := 0; y < 5; y++ {
					for x := 0; x < 32; x++ {
						cell := cells.CellAt(x, y)
						if cell.Style.Fg != nil || cell.Style.Bg != nil {
							t.Fatalf("inherited rendered cell (%d,%d) retained theme colors", x, y)
						}
					}
				}
			}
			for _, sequence := range []string{"\x1b]10;", "\x1b]11;", "\x1b]110", "\x1b]111"} {
				if strings.Contains(captured, sequence) {
					t.Fatalf("theme mutated terminal defaults: %q", sequence)
				}
			}
		})
	}
}

func checkThemeRendererColors(t *testing.T, content string, profile colorprofile.Profile) {
	t.Helper()
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	var state byte
	var pen uv.Style
	colors := 0
	for len(content) > 0 {
		seq, _, n, next := ansi.DecodeSequence(content, state, parser)
		state = next
		if n == 0 {
			break
		}
		content = content[n:]
		if !ansi.HasCsiPrefix(seq) || parser.Command() != 'm' {
			continue
		}
		uv.ReadStyle(parser.Params(), &pen)
		for _, c := range []interface{}{pen.Fg, pen.Bg} {
			if c == nil {
				continue
			}
			colors++
			switch profile {
			case colorprofile.ASCII, colorprofile.NoTTY:
				t.Fatal("colorless renderer emitted color")
			case colorprofile.ANSI:
				if _, ok := c.(ansi.BasicColor); !ok {
					t.Fatalf("ANSI renderer emitted %T", c)
				}
			case colorprofile.ANSI256:
				switch c.(type) {
				case ansi.BasicColor, ansi.IndexedColor:
				default:
					t.Fatalf("ANSI256 renderer emitted %T", c)
				}
			}
		}
	}
	if profile > colorprofile.ASCII && colors == 0 {
		t.Fatal("colored renderer omitted palette colors")
	}
}

// replayThemeTerminal models the cursor, SGR, and erase operations emitted by
// Bubble Tea's real renderer. It deliberately ignores terminal mode requests.
func replayThemeTerminal(content string, width, height int) uv.ScreenBuffer {
	screen := uv.NewScreenBuffer(width, height)
	parser := ansi.GetParser()
	defer ansi.PutParser(parser)
	var state byte
	var style uv.Style
	x, y := 0, 0
	erase := func(x0, y0, x1, y1 int) {
		for row := y0; row <= y1; row++ {
			for column := x0; column <= x1; column++ {
				screen.SetCell(column, row, &uv.Cell{Content: " ", Width: 1, Style: style})
			}
		}
	}
	for len(content) > 0 {
		seq, w, n, next := ansi.DecodeSequence(content, state, parser)
		state = next
		if n == 0 {
			break
		}
		content = content[n:]
		if w > 0 {
			if x >= width {
				x = 0
				y++
			}
			screen.SetCell(x, y, &uv.Cell{Content: seq, Width: w, Style: style})
			x += w
			continue
		}
		switch seq {
		case "\r":
			x = 0
			continue
		case "\n":
			x = 0
			y++
			continue
		case "\b":
			x = max(0, x-1)
			continue
		}
		if seq == "\x1bM" {
			y = max(0, y-1)
			continue
		}
		if !ansi.HasCsiPrefix(seq) {
			continue
		}
		a, _ := parser.Param(0, 1)
		b, _ := parser.Param(1, 1)
		switch byte(parser.Command()) {
		case 'm':
			uv.ReadStyle(parser.Params(), &style)
		case 'H', 'f':
			x = max(0, b-1)
			y = max(0, a-1)
		case 'G':
			x = max(0, a-1)
		case 'd':
			y = max(0, a-1)
		case 'A':
			y = max(0, y-a)
		case 'B':
			y += a
		case 'C':
			x += a
		case 'D':
			x = max(0, x-a)
		case 'E':
			x = 0
			y += a
		case 'F':
			x = 0
			y = max(0, y-a)
		case 'X':
			erase(x, y, min(width-1, x+a-1), y)
		case 'K':
			mode, _ := parser.Param(0, 0)
			switch mode {
			case 0:
				erase(x, y, width-1, y)
			case 1:
				erase(0, y, x, y)
			case 2:
				erase(0, y, width-1, y)
			}
		case 'J':
			mode, _ := parser.Param(0, 0)
			switch mode {
			case 0:
				erase(x, y, width-1, y)
				erase(0, y+1, width-1, height-1)
			case 1:
				erase(0, 0, width-1, y-1)
				erase(0, y, x, y)
			case 2, 3:
				erase(0, 0, width-1, height-1)
			}
		}
	}
	return screen
}
