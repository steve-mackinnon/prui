package syntax

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/lexers"
)

func BenchmarkLexerSelection(b *testing.B) {
	for _, extension := range []string{"go", "ts", "unknown-extension"} {
		b.Run(extension, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = matchLexer(fmt.Sprintf("pkg/%d/file%d.%s", i, i, extension))
			}
		})
	}
}

func TestLexerSelectionMatchesChroma(t *testing.T) {
	paths := []string{"", ".", "Dockerfile", "Makefile", "CMakeLists.txt", ".gitignore", "file.go", "file.C", "file.h", "file.d.ts", "file.py", "file.unknown-extension", "file.\xff.go"}
	for _, path := range append([]string(nil), paths...) {
		for _, suffix := range []string{"~", ".bak", ".old", ".orig", ".dpkg-dist", ".dpkg-old", ".ucf-dist", ".ucf-new", ".ucf-old", ".rpmnew", ".rpmorig", ".rpmsave", ".in", ".in~", ".unexpected"} {
			paths = append(paths, "nested/"+path+suffix)
		}
	}
	// Include every registered language, including aliases and unusual patterns.
	// The real registry remains the oracle for ordering and tie-breaking.
	for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
		for _, patterns := range [][]string{lexer.Config().Filenames, lexer.Config().AliasFilenames} {
			if len(patterns) > 0 {
				paths = append(paths, strings.ReplaceAll(patterns[0], "*", "fixture"))
			}
		}
	}
	for _, path := range paths {
		if got, want := matchLexer(path), lexers.Match(path); got != want {
			t.Fatalf("lexer for %q = %v, want %v", path, got, want)
		}
	}
}
