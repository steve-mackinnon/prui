package tui

import (
	"context"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/syntax"
	"prui/internal/theme"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestExpandedSourceIndependentSyntax(t *testing.T) {
	m := navigationModel("@@ -2 +2 @@\n-/*\n+//\n", "package p\n/*\nvar x = 1\n*/\n", "package p\n//\nvar x = 1\n*/\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	m.Session.Inventory.Files[0].OldPath = []byte("x.go")
	m.Session.Inventory.Files[0].NewPath = []byte("x.go")
	m.navigation.mode = "expanded"
	var found bool
	for _, row := range m.navigationDetail(0) {
		if row.rawSource != "var x = 1" {
			continue
		}
		found = true
		if len(row.oldSyntax) != 1 || row.oldSyntax[0].Kind != syntax.Comment || len(row.syntax) == 0 || row.syntax[0].Kind != syntax.Keyword {
			t.Fatalf("wrong side state: old=%v new=%v", row.oldSyntax, row.syntax)
		}
		if row.target != nil || row.oldTarget != nil {
			t.Fatal("context acquired target")
		}
		split := projectSideBySideDetail([]diffLine{row})
		if len(split) != 1 || !reflect.DeepEqual(split[0].sideBySide.old.line.syntax, row.oldSyntax) {
			t.Fatal("split lost OLD state")
		}
	}
	if !found {
		t.Fatal("missing context")
	}
	for _, mode := range []string{"OLD", "NEW"} {
		m.navigation.mode = mode
		for _, row := range m.navigationDetail(0) {
			if row.rawSource == "var x = 1" && len(row.syntax) == 0 {
				t.Fatal("full source lacks syntax", mode)
			}
		}
	}
}

func TestSourceSyntaxCacheReuseAndLimits(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-package p\n+package q\n", "package p\n", "package q\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	f := &m.Session.Inventory.Files[0]
	f.OldPath, f.NewPath = []byte("x.go"), []byte("x.go")
	cache := m.sourceSyntaxCache()
	inv := m.navigationInventory()
	tokens := cache.tokens(nil, inv, 0, false)
	if len(tokens[1]) == 0 {
		t.Fatal("missing Go tokens")
	}
	used := cache.bytes
	cache.tokens(nil, inv, 0, false)
	if cache.bytes != used || len(cache.entries) != 1 {
		t.Fatal("relexed cached source")
	}
	// Same OID, different extension must not reuse Go tokens.
	inv.Files[0].NewPath = []byte("x.unknown-extension")
	if got := cache.tokens(nil, inv, 0, false); len(got) != 0 {
		t.Fatal("reused lexer across rename")
	}
	used = cache.bytes
	cache.tokens(nil, inv, 0, false)
	if cache.bytes != used || len(cache.entries) != 2 {
		t.Fatal("failed attempt retried")
	}
	inv.Files[0].NewPath = []byte("other.go")
	cache.spans = 31999
	inv.FullSource.Blobs[f.NewOID] = []byte("var x = 123\n")
	if got := cache.tokens(nil, inv, 0, false); got != nil || cache.spans != 31999 {
		t.Fatal("partially admitted spans")
	}
	used = cache.bytes
	cache.tokens(nil, inv, 0, false)
	if cache.bytes != used {
		t.Fatal("span failure retried")
	}
	inv.Files[0].NewPath = []byte("time.go")
	cache.elapsed = 500 * time.Millisecond
	used = cache.bytes
	if cache.tokens(nil, inv, 0, false) != nil || cache.bytes != used {
		t.Fatal("time budget exceeded")
	}
	inv.Files[0].NewPath = []byte("bytes.go")
	cache.elapsed = 0
	cache.bytes = 4 << 20
	if cache.tokens(nil, inv, 0, false) != nil || cache.bytes != 4<<20 {
		t.Fatal("byte budget exceeded")
	}
	// Snapshot replacement gets a fresh cache; no path-keyed cross-snapshot reuse.
	m.Session = largeTextSession(1, 1)
	if m.sourceSyntaxCache() == cache {
		t.Fatal("cache crossed snapshot")
	}
}

func TestSourceSyntaxMissingAndOversize(t *testing.T) {
	m := navigationModel("@@ -1 +1 @@\n-a\n+b\n", "a\n", "b\n", inventory.Range{Start: 1, Count: 1}, inventory.Range{Start: 1, Count: 1})
	defer m.Close()
	inv := m.navigationInventory()
	inv.Files[0].NewPath = []byte("x.go")
	cache := m.sourceSyntaxCache()
	delete(inv.FullSource.Blobs, "new")
	if cache.tokens(nil, inv, 0, false) != nil || len(cache.entries) != 0 {
		t.Fatal("missing source cached as failed lex")
	}
	inv.FullSource.Blobs["new"] = []byte("package p\n")
	if len(cache.tokens(nil, inv, 0, false)) == 0 {
		t.Fatal("newly loaded source not lexed")
	}
	inv.Files[0].NewOID = "large"
	inv.FullSource.Blobs["large"] = []byte(strings.Repeat("x", syntax.MaxBytes+1))
	used := cache.bytes
	if cache.tokens(nil, inv, 0, false) != nil || cache.bytes != used {
		t.Fatal("oversize blob lexed")
	}
}

func TestSourceSyntaxProjectionReuseAndRendering(t *testing.T) {
	text := "var 日本 = \"hello\" // \t\x1b[31m"
	m := navigationModel("@@ -2 +2 @@\n-old\n+new\n", text+"\nold\n", text+"\nnew\n", inventory.Range{Start: 2, Count: 1}, inventory.Range{Start: 2, Count: 1})
	defer m.Close()
	f := &m.Session.Inventory.Files[0]
	f.OldPath, f.NewPath = []byte("x.go"), []byte("x.go")
	m.navigation.mode = "expanded"
	lines := m.cachedFileDetail(false)
	cache := m.sourceSyntaxCache()
	used, elapsed := cache.bytes, cache.elapsed
	m.Session.Guides = &guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Sections: []guide.Section{{UnitIDs: []string{m.Session.Inventory.Units[0].ID}}}}}}
	m.guideExpanded = true
	detail := m.cachedGuideDetail(0)
	found := false
	for _, row := range detail.lines {
		if row.rawSource == text {
			found = true
			if len(row.syntax) == 0 || len(row.oldSyntax) == 0 {
				t.Fatal("guide lacks syntax")
			}
		}
	}
	if !found {
		t.Fatal("guide lacks context")
	}
	for _, name := range []string{theme.GitHubLight, theme.GitHubDark} {
		m.theme = themeForName(t, name)
		m.styles = stylesFor(m.theme)
		for _, profile := range []colorprofile.Profile{colorprofile.TrueColor, colorprofile.Ascii} {
			m.colorProfile = profile
			for _, row := range lines {
				if row.rawSource != text {
					continue
				}
				for _, horizontal := range []int{0, 5, 13} {
					for _, width := range []int{15, 70, 180} {
						plain := row
						plain.syntax = nil
						actual := m.syntaxText(row, horizontal, width, "")
						if ansi.Strip(actual) != ansi.Strip(m.syntaxText(plain, horizontal, width, "")) {
							t.Fatal("syntax changed escaped/clipped text")
						}
					}
				}
				if profile == colorprofile.TrueColor {
					rendered := m.syntaxText(row, 0, 180, "")
					cells := canvasCells(rendered, visibleWidth(row.Text), 1)
					keyword, _ := m.theme.Color(theme.FocusedBorder)
					if !sameCanvasColor(cells.CellAt(1, 0).Style.Fg, keyword) {
						t.Fatal("keyword palette lost")
					}
				}
			}
		}
		m.fileCache = fileDetailCache{}
		m.cachedFileDetail(true)
	}
	if cache.bytes != used || cache.elapsed != elapsed {
		t.Fatal("projections relexed source")
	}
}

func TestSourceSyntaxCanceledAttemptIsCached(t *testing.T) {
	m := navigationModel("", "package p\n", "package p\n", inventory.Range{}, inventory.Range{})
	defer m.Close()
	inv := m.navigationInventory()
	inv.Files[0].NewPath = []byte("x.go")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cache := m.sourceSyntaxCache()
	if cache.tokens(ctx, inv, 0, false) != nil {
		t.Fatal("canceled work highlighted")
	}
	used := cache.bytes
	if cache.tokens(context.Background(), inv, 0, false) != nil || cache.bytes != used {
		t.Fatal("canceled attempt retried")
	}
}
