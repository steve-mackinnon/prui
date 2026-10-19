// Package syntax captures optional, theme-independent source highlighting.
package syntax

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
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
	// Append categories: numeric values are persisted in saved reviews.
	Function
	Type
	Macro
	Constant
	Attribute
	Builtin
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
func Tokenize(ctx context.Context, path string, source []byte) Lines {
	lines, _ := TokenizeWithStatus(ctx, path, source)
	return lines
}

const MaxDuration = time.Second

func Supported(path string) bool { return matchLexer(path) != nil }

// TokenizeWithStatus retains the reason a best-effort capture was unavailable.
func TokenizeWithStatus(ctx context.Context, path string, source []byte) (Lines, Status) {
	if cache, ok := ctx.Value(tokenCacheKey{}).(*tokenCache); ok {
		return cache.capture(ctx, path, source, tokenizeDefault)
	}
	return tokenizeDefault(ctx, path, source)
}

// TokenizeThrough captures at most the given one-based source line. The lexer
// still receives the complete pinned bytes for state and lookahead. Ready means
// the requested prefix is complete, not that the unused tail was tokenized.
// A nonpositive line requests the ordinary full capture.
func TokenizeThrough(ctx context.Context, path string, source []byte, through int) (Lines, Status) {
	if through <= 0 {
		return TokenizeWithStatus(ctx, path, source)
	}
	compute := func(c context.Context, p string, b []byte) (Lines, Status) {
		return tokenizeThrough(c, p, b, MaxDuration, through)
	}
	if cache, ok := ctx.Value(tokenCacheKey{}).(*tokenCache); ok {
		return cache.captureThrough(ctx, path, source, through, compute)
	}
	return compute(ctx, path, source)
}

func tokenizeDefault(ctx context.Context, path string, source []byte) (Lines, Status) {
	return tokenize(ctx, path, source, MaxDuration)
}

func contextStatus(ctx context.Context) Status {
	if ctx.Err() == context.DeadlineExceeded {
		return Timeout
	}
	return Canceled
}

func tokenize(ctx context.Context, path string, source []byte, duration time.Duration) (lines Lines, status Status) {
	return tokenizeThrough(ctx, path, source, duration, 0)
}

func tokenizeThrough(ctx context.Context, path string, source []byte, duration time.Duration, through int) (lines Lines, status Status) {
	if ctx.Err() != nil {
		return nil, contextStatus(ctx)
	}
	if len(source) > MaxBytes {
		return nil, ByteLimit
	}
	if !utf8.Valid(source) {
		return nil, InvalidText
	}
	lexer := matchLexer(path)
	if lexer == nil {
		return nil, Unsupported
	}
	defer func() {
		if recover() != nil {
			lines, status = nil, LexerError
		}
	}()
	deadline := time.Now().Add(duration)
	it, err := tokenLexer(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, string(source))
	if err != nil {
		return nil, LexerError
	}
	lines = Lines{}
	rust := lexer.Config().Name == "Rust"
	line, column, offset, count := 1, 0, 0, 0
	for {
		if ctx.Err() != nil {
			return nil, contextStatus(ctx)
		}
		if time.Now().After(deadline) {
			return nil, Timeout
		}
		token := it()
		if token == chroma.EOF {
			break
		}
		// Reject lexers that normalize or invent text: offsets must match the blob.
		if offset+len(token.Value) > len(source) || string(source[offset:offset+len(token.Value)]) != token.Value {
			return nil, InvalidOffsets
		}
		offset += len(token.Value)
		kind := category(token.Type)
		// Chroma uses FunctionMagic for Rust macros, but also for ordinary
		// special methods in other languages (for example Python's __init__).
		if rust && token.Type == chroma.NameFunctionMagic && strings.HasSuffix(token.Value, "!") {
			kind = Macro
		}
		parts := strings.Split(token.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				line++
				column = 0
				if through > 0 && line > through {
					if ctx.Err() != nil {
						return nil, contextStatus(ctx)
					}
					if time.Now().After(deadline) {
						return nil, Timeout
					}
					return lines, Ready
				}
			}
			if len(part) > 0 && kind != Plain {
				lines[line] = append(lines[line], Span{column, column + len(part), kind})
				count++
				if count > MaxSpans {
					return nil, SpanLimit
				}
			}
			column += len(part)
		}
	}
	if offset != len(source) {
		return nil, InvalidOffsets
	}
	return lines, Ready
}

func category(t chroma.TokenType) Kind {
	switch t {
	case chroma.KeywordType, chroma.NameClass:
		return Type
	case chroma.KeywordConstant, chroma.NameConstant:
		return Constant
	case chroma.NameFunction, chroma.NameFunctionMagic:
		return Function
	case chroma.NameAttribute, chroma.NameDecorator, chroma.CommentPreproc, chroma.CommentPreprocFile:
		return Attribute
	case chroma.NameBuiltin, chroma.NameBuiltinPseudo:
		return Builtin
	}
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
