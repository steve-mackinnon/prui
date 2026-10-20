package syntax

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Keep aligned with ignoredSuffixes in the pinned Chroma registry.go. Instead
// of trying all 13 suffixes against every glob, strip only a suffix actually
// present in the basename. The unmodified filename is always tried first.
var lexerBackupSuffixes = [...]string{
	"~", ".bak", ".old", ".orig",
	".dpkg-dist", ".dpkg-old", ".ucf-dist", ".ucf-new", ".ucf-old",
	".rpmnew", ".rpmorig", ".rpmsave", ".in",
}

func matchLexer(path string) chroma.Lexer {
	name := filepath.Base(path)
	names := []string{name}
	for _, suffix := range lexerBackupSuffixes {
		if strings.HasSuffix(name, suffix) {
			names = append(names, strings.TrimSuffix(name, suffix))
		}
	}
	// Preserve the original registry order, one match per pattern, primary-name
	// precedence and Chroma's own priority sorting (including equal priorities).
	for _, aliases := range []bool{false, true} {
		matched := chroma.PrioritisedLexers{}
		for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
			patterns := lexer.Config().Filenames
			if aliases {
				patterns = lexer.Config().AliasFilenames
			}
			for _, pattern := range patterns {
				if matchesFilename(pattern, names) {
					matched = append(matched, lexer)
				}
			}
		}
		if len(matched) > 0 {
			sort.Sort(matched)
			return matched[0]
		}
	}
	return nil
}

func matchesFilename(pattern string, names []string) bool {
	for _, name := range names {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			panic(err) // Same invalid-registry-pattern behavior as Chroma.
		}
		if matched {
			return true
		}
	}
	return false
}
