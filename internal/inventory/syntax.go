package inventory

import (
	"bytes"
	"prui/internal/syntax"
)

// hunkSyntax retains only categories for displayed source, never blob text.
func hunkSyntax(h hunk, old, new syntax.Lines) syntax.Patch {
	result := syntax.Patch{}
	a, b := h.old.Start, h.newRange.Start
	inHunk := false
	for i, raw := range bytes.Split(h.patch, []byte{'\n'}) {
		if bytes.HasPrefix(raw, []byte("@@ ")) {
			inHunk = true
			continue
		}
		if !inHunk || len(raw) == 0 {
			continue
		}
		row := syntax.Row{}
		switch raw[0] {
		case '-':
			row.Old = old[a]
			a++
		case '+':
			row.New = new[b]
			b++
		case ' ':
			row.Old = old[a]
			row.New = new[b]
			a++
			b++
		}
		if len(row.Old)+len(row.New) > 0 {
			result[i] = row
		}
	}
	return result
}
