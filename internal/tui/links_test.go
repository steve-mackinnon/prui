package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestURLLinksPreserveStylesAndHover(t *testing.T) {
	text := "界 \x1b[31mhttps://example.com/a_(b)?x=1&y=2\x1b[m. tail"
	linked := linkifyURLs(text)
	cells := themeCanvasBuffer(linked, 80, 1)
	if got := cells.CellAt(3, 0).Link.URL; got != "https://example.com/a_(b)?x=1&y=2" {
		t.Fatalf("URL: %q", got)
	}
	if ansi.Strip(linked) != ansi.Strip(text) {
		t.Fatal("text changed")
	}
	if cells.CellAt(3, 0).Style.Underline != 0 {
		t.Fatal("underlined without hover")
	}
	hovered := hoverURL(linked, 80, 1, 4, 0)
	cells = themeCanvasBuffer(hovered, 80, 1)
	if cells.CellAt(3, 0).Style.Underline == 0 || cells.CellAt(40, 0).Style.Underline != 0 {
		t.Fatal("hover boundary")
	}
}

func TestURLValidation(t *testing.T) {
	for _, s := range []string{"javascript:alert(1)", "file:///tmp/a", "https://", "https://host/\x1b", "https://host/\\x1b"} {
		if validBrowserURL(s) {
			t.Fatalf("accepted %q", s)
		}
	}
	if !validBrowserURL("https://example.com/%20?q=a&b=c") {
		t.Fatal("valid URL refused")
	}
}

func TestClippedAndPannedURLKeepsFullDestination(t *testing.T) {
	source := "+ https://example.com/very/long/path?q=a&b=c"
	for _, test := range []struct {
		content               string
		horizontal, prefix, x int
	}{
		{"+ https://exam", 0, 0, 3},
		{"12 │ example.com/", 10, 5, 5},
	} {
		got := linkSourceURLs(test.content, source, test.horizontal, test.prefix)
		cells := themeCanvasBuffer(got, 40, 1)
		if cells.CellAt(test.x, 0).Link.URL != source[2:] {
			t.Fatalf("clipped destination: %q", cells.CellAt(test.x, 0).Link.URL)
		}
	}
}

func TestDescriptionURLMouseRouting(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	s := screenSession()
	body := "[Reference](https://example.com/full/path)"
	s.PullRequestDescription = &body
	m.openReviewTab(s)
	m.Width, m.Height = 100, 20
	m.selectReviewView(viewDescription)
	cells := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
	x, y := -1, -1
	for row := 0; row < m.Height && x < 0; row++ {
		for col := 0; col < m.Width; col++ {
			if cells.CellAt(col, row).Link.URL == "https://example.com/full/path" {
				x, y = col, row
				break
			}
		}
	}
	if x < 0 {
		t.Fatal("Markdown destination missing")
	}
	if cells.CellAt(x, y).Style.Underline != 0 {
		t.Fatal("Markdown link underlined before hover")
	}
	m.Update(tea.MouseMotionMsg{X: x, Y: y})
	hovered := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
	if hovered.CellAt(x, y).Style.Underline == 0 {
		t.Fatal("mouse motion did not underline")
	}
	_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if cmd == nil {
		t.Fatal("URL click did not return browser command")
	}
	m.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	if m.linkMouseX != 0 || m.linkMouseY != 0 {
		t.Fatal("hover did not move away")
	}
	m.Busy = true
	_, cmd = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if cmd != nil || m.linkMouseValid {
		t.Fatal("modal allowed URL activation")
	}
}

func TestBrowserOpenerPassesURLAsSingleArgument(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("shell fixture requires macOS or Linux")
	}
	directory := t.TempDir()
	capture := filepath.Join(directory, "args")
	name := "open"
	if runtime.GOOS == "linux" {
		name = "xdg-open"
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$#\" \"$1\" > \"$PRUI_URL_CAPTURE\"\n"
	if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	t.Setenv("PRUI_URL_CAPTURE", capture)
	target := "https://example.com/?x=$(touch%20bad)&y=1"
	if err := openBrowserURL(target); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "1\n"+target+"\n" {
		t.Fatalf("browser arguments: %q", data)
	}
	if err := openBrowserURL("file:///tmp/a"); err == nil {
		t.Fatal("unsafe scheme launched")
	}
}

func TestCodeURLMouseRouting(t *testing.T) {
	for _, layout := range []diffLayout{diffLayoutUnified, diffLayoutSideBySide} {
		m := New(context.Background(), nil)
		s := screenSession()
		target := "https://example.com/" + strings.Repeat("long/", 35)
		s.Inventory.Patches["patch"] = []byte("@@ -1 +1 @@\n-old\n+// " + target + "\n")
		m.openReviewTab(s)
		m.Width, m.Height = 180, 20
		m.selectReviewView(viewFiles)
		m.layout = layout
		cells := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
		x, y := -1, -1
		for row := 0; row < m.Height && x < 0; row++ {
			for col := 0; col < m.Width; col++ {
				if cells.CellAt(col, row).Link.URL == target {
					x, y = col, row
					break
				}
			}
		}
		if x < 0 {
			m.Close()
			t.Fatalf("layout %v: full URL not preserved in clipped code", layout)
		}
		m.Update(tea.MouseMotionMsg{X: x, Y: y})
		hovered := themeCanvasBuffer(m.View().Content, m.Width, m.Height)
		if hovered.CellAt(x, y).Style.Underline == 0 {
			t.Fatal("code hover missing")
		}
		_, cmd := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
		if cmd == nil || m.Composer != nil {
			t.Fatal("code URL click did not open browser exclusively")
		}
		m.Close()
	}
}
