// Package syntax captures optional, theme-independent source highlighting.
package syntax

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

type Kind uint8

const (
	Plain Kind = iota
	Keyword
	String
	Number
	Comment
	Name
	Operator
)

// Span uses byte offsets in a raw source line, excluding its patch marker.
type Span struct {
	Start, End int
	Kind       Kind
}
type Lines map[int][]Span

// Patch stores old/new spans by zero-based patch row. Context can differ by side.
type Row struct{ Old, New []Span }
type Patch map[int]Row

const MaxBytes = 256 << 10
const MaxSpans = 16000

// Tokenize is deliberately best effort. Limits apply before and during lexing;
// Chroma's own regexp timeout bounds individual regexp matches.
func Tokenize(ctx context.Context, path string, source []byte) (lines Lines) {
	if ctx.Err() != nil || len(source) > MaxBytes || !utf8.Valid(source) {
		return nil
	}
	lexer := lexers.Match(path)
	if lexer == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			lines = nil
		}
	}()
	it, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, string(source))
	if err != nil {
		return nil
	}
	lines = Lines{}
	line, column, offset, count := 1, 0, 0, 0
	deadline := time.Now().Add(100 * time.Millisecond)
	for {
		if ctx.Err() != nil || time.Now().After(deadline) {
			return nil
		}
		token := it()
		if token == chroma.EOF {
			break
		}
		// Reject lexers that normalize or invent text: offsets must match the blob.
		if offset+len(token.Value) > len(source) || string(source[offset:offset+len(token.Value)]) != token.Value {
			return nil
		}
		offset += len(token.Value)
		kind := category(token.Type)
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				line++
				column = 0
			}
			if len(part) > 0 && kind != Plain {
				lines[line] = append(lines[line], Span{column, column + len(part), kind})
				count++
				if count > MaxSpans {
					return nil
				}
			}
			column += len(part)
		}
	}
	if offset != len(source) {
		return nil
	}
	return lines
}

func category(t chroma.TokenType) Kind {
	switch {
	case t.InCategory(chroma.Keyword):
		return Keyword
	case t.InSubCategory(chroma.LiteralString):
		return String
	case t.InSubCategory(chroma.LiteralNumber):
		return Number
	case t.InCategory(chroma.Comment):
		return Comment
	case t.InCategory(chroma.Name) && t != chroma.Name && t != chroma.NameOther:
		return Name
	case t.InCategory(chroma.Operator):
		return Operator
	default:
		return Plain
	}
}
