package syntax

import (
	"crypto/sha256"
	"fmt"
	regexsyntax "regexp/syntax"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

var goDispatchOnce sync.Once
var goDispatcher chroma.Lexer

// Only the pinned, stateless Go rules have the ASCII compatibility assumptions
// needed below. Changes to Chroma's patterns fall back until explicitly reviewed.
const goDispatchPatterns = "ff6654244f09f25e60e82f47d0bbc4ba1f217ed00e896fe92d3092d593c76e04"

func tokenLexer(lexer chroma.Lexer) chroma.Lexer {
	original, ok := lexer.(*chroma.RegexLexer)
	if !ok || lexer != lexers.Go {
		return lexer
	}
	goDispatchOnce.Do(func() {
		goDispatcher = original
		goDispatcher = buildGoDispatcher(original)
	})
	return goDispatcher
}

func buildGoDispatcher(original *chroma.RegexLexer) chroma.Lexer {
	config := *original.Config()
	if config.Name != "Go" || config.CaseInsensitive || config.DotAll || config.NotMultiline {
		return original
	}
	rules, err := original.Rules()
	if err != nil || len(rules) != 1 || len(rules["root"]) == 0 || len(rules["root"]) > 63 {
		return original
	}
	root := rules["root"]
	patterns := make([]string, len(root))
	for i, rule := range root {
		if rule.Mutator != nil {
			return original
		}
		patterns[i] = rule.Pattern
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(patterns, "\x00")))) != goDispatchPatterns {
		return original
	}
	parsed := make([]*regexsyntax.Regexp, len(root))
	for i, pattern := range patterns {
		parsed[i], err = regexsyntax.Parse(pattern, regexsyntax.Perl)
		if err != nil {
			return original
		}
	}

	var states [128]string
	plans := map[string]uint64{}
	dispatched := chroma.Rules{}
	for c := range states {
		var mask uint64
		for i, re := range parsed {
			matches, empty := asciiFirst(re, rune(c))
			if matches || empty {
				mask |= uint64(1) << i
			}
		}
		name := fmt.Sprintf("ascii-%x", mask)
		states[c] = name
		plans[name] = mask
		dispatched[name] = nil
	}
	reset := func(index int) chroma.Mutator {
		return chroma.MutatorFunc(func(state *chroma.LexerState) error {
			next := "root"
			if state.Pos < len(state.Text) {
				c := state.Text[state.Pos]
				if c >= 0 && c < 128 && c != '\v' {
					next = states[c]
				}
			}
			// Select the next candidate list after this match. Go's emitters use
			// explicit root recursion; no extra regex dispatch is needed per token.
			state.Stack[len(state.Stack)-1] = next
			state.State = "root"
			state.Rule = index
			return nil
		})
	}
	dispatched["unmatched"] = []chroma.Rule{{Pattern: `(?s:.)`, Type: chroma.Error, Mutator: reset(-1)}}
	var initialize sync.Once
	initialized := false
	dispatch := chroma.MutatorFunc(func(state *chroma.LexerState) error {
		initialize.Do(func() {
			// Share the root's compiled regexes instead of creating one runner pool
			// per character class. Installation happens during construction, before
			// this lexer is published or receives any source bytes.
			installGoDispatch(state.Rules, plans)
			initialized = true
		})
		state.Stack[len(state.Stack)-1] = states[state.Text[state.Pos]]
		return nil
	})
	// .NET includes VT in \s, unlike Go's regex parser. Unicode and VT retain
	// the complete original rule sequence; assertions are never used to exclude.
	dispatched["root"] = []chroma.Rule{{Pattern: `(?=[\x00-\x0a\x0c-\x7f])`, Mutator: dispatch}}
	for i, rule := range root {
		rule.Mutator = reset(i)
		dispatched["root"] = append(dispatched["root"], rule)
	}
	lexer, err := chroma.NewLexer(&config, func() chroma.Rules { return dispatched })
	if err != nil {
		return original
	}
	lexer.SetRegistry(lexers.GlobalLexerRegistry)
	iterator, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", Nested: true}, " ")
	if err != nil {
		return original
	}
	_ = iterator()
	if !initialized {
		return original
	}
	return lexer
}

func installGoDispatch(rules chroma.CompiledRules, plans map[string]uint64) {
	original := rules["root"][1:]
	fallback := rules["unmatched"][0]
	for state, mask := range plans {
		selected := make([]*chroma.CompiledRule, 0, len(original)+1)
		for i, rule := range original {
			if mask&(uint64(1)<<i) != 0 {
				selected = append(selected, rule)
			}
		}
		rules[state] = append(selected, fallback)
	}
}

// asciiFirst over-approximates the first consuming ASCII character. Empty
// assertions may over-include candidates; they must never exclude a valid rule.
func asciiFirst(re *regexsyntax.Regexp, c rune) (bool, bool) {
	switch re.Op {
	case regexsyntax.OpNoMatch:
		return false, false
	case regexsyntax.OpEmptyMatch, regexsyntax.OpBeginLine, regexsyntax.OpEndLine, regexsyntax.OpBeginText, regexsyntax.OpEndText, regexsyntax.OpWordBoundary, regexsyntax.OpNoWordBoundary:
		return false, true
	case regexsyntax.OpLiteral:
		if len(re.Rune) == 0 {
			return false, true
		}
		return re.Flags&regexsyntax.FoldCase != 0 || re.Rune[0] == c, false
	case regexsyntax.OpCharClass:
		for i := 0; i < len(re.Rune); i += 2 {
			if re.Rune[i] <= c && c <= re.Rune[i+1] {
				return true, false
			}
		}
		return false, false
	case regexsyntax.OpAnyChar:
		return true, false
	case regexsyntax.OpAnyCharNotNL:
		return c != '\n', false
	case regexsyntax.OpCapture:
		return asciiFirst(re.Sub[0], c)
	case regexsyntax.OpConcat:
		matches, empty := false, true
		for _, child := range re.Sub {
			yes, nullable := asciiFirst(child, c)
			matches = matches || yes
			if !nullable {
				empty = false
				break
			}
		}
		return matches, empty
	case regexsyntax.OpAlternate:
		matches, empty := false, false
		for _, child := range re.Sub {
			yes, nullable := asciiFirst(child, c)
			matches = matches || yes
			empty = empty || nullable
		}
		return matches, empty
	case regexsyntax.OpStar, regexsyntax.OpQuest:
		yes, _ := asciiFirst(re.Sub[0], c)
		return yes, true
	case regexsyntax.OpPlus:
		return asciiFirst(re.Sub[0], c)
	case regexsyntax.OpRepeat:
		if re.Max == 0 {
			return false, true
		}
		yes, empty := asciiFirst(re.Sub[0], c)
		return yes, re.Min == 0 || empty
	default:
		return true, true
	}
}
