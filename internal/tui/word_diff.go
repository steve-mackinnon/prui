package tui

import (
	"charm.land/lipgloss/v2"
	"prui/internal/syntax"
	"unicode"
	"unicode/utf8"
)

// Presentation-only word spans with bounded token and line alignment.
const wordDiffMaxBytes = 256 << 10
const wordDiffMaxLine = 4096
const wordDiffMaxTokens = 256
const wordDiffMaxWork = 1 << 20

type wordToken struct {
	text       string
	start, end int
}

func wordTokens(text string) []wordToken {
	var out []wordToken
	category := func(r rune) int {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
			return 1
		}
		if unicode.IsSpace(r) {
			return 2
		}
		return 0
	}
	for at := 0; at < len(text); {
		start := at
		r, size := utf8.DecodeRuneInString(text[at:])
		at += size
		kind := category(r)
		for kind != 0 && at < len(text) {
			r, size = utf8.DecodeRuneInString(text[at:])
			if category(r) != kind {
				break
			}
			at += size
		}
		out = append(out, wordToken{text[start:at], start, at})
		if len(out) > wordDiffMaxTokens {
			return nil
		}
	}
	return out
}

func changedWords(old, new string, budget *int) ([]syntax.Span, []syntax.Span) {
	a, b, _, ok := compareWords(old, new, budget)
	if !ok {
		return nil, nil
	}
	return a, b
}

// Only a unique set of optimal matching token coordinates is accepted. Multiple
// edit orderings around the same matches are harmless; repeated-token matches
// at different source coordinates are ambiguous and must fall back.
func compareWords(old, new string, budget *int) ([]syntax.Span, []syntax.Span, int, bool) {
	if len(old) > wordDiffMaxLine || len(new) > wordDiffMaxLine || !utf8.ValidString(old) || !utf8.ValidString(new) {
		return nil, nil, 0, false
	}
	a, b := wordTokens(old), wordTokens(new)
	if len(a) == 0 || len(b) == 0 {
		return nil, nil, 0, false
	}
	work := (len(a) + 1) * (len(b) + 1)
	if 2*work > *budget {
		return nil, nil, 0, false
	}
	*budget -= 2 * work
	stride := len(b) + 1
	suffix, prefix := make([]int, work), make([]int, work)
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i].text == b[j].text {
				suffix[i*stride+j] = 1 + suffix[(i+1)*stride+j+1]
			} else {
				suffix[i*stride+j] = max(suffix[(i+1)*stride+j], suffix[i*stride+j+1])
			}
		}
	}
	for i := 0; i < len(a); i++ {
		for j := 0; j < len(b); j++ {
			if a[i].text == b[j].text {
				prefix[(i+1)*stride+j+1] = 1 + prefix[i*stride+j]
			} else {
				prefix[(i+1)*stride+j+1] = max(prefix[i*stride+j+1], prefix[(i+1)*stride+j])
			}
		}
	}
	matched := suffix[0]
	if matched == 0 {
		return nil, nil, 0, false
	}
	score := 2 * matched * 10000 / (len(a) + len(b))
	oldMatch, newMatch := make([]bool, len(a)), make([]bool, len(b))
	candidates := 0
	for i := range a {
		for j := range b {
			if a[i].text == b[j].text && prefix[i*stride+j]+1+suffix[(i+1)*stride+j+1] == matched {
				candidates++
				oldMatch[i] = true
				newMatch[j] = true
			}
		}
	}
	if candidates != matched {
		return nil, nil, score, false
	}
	var left, right []syntax.Span
	for i, t := range a {
		if !oldMatch[i] {
			left = append(left, syntax.Span{Start: t.start, End: t.end, Kind: syntax.Operator})
		}
	}
	for i, t := range b {
		if !newMatch[i] {
			right = append(right, syntax.Span{Start: t.start, End: t.end, Kind: syntax.Operator})
		}
	}
	return left, right, score, true
}

func attachWordChanges(lines []diffLine, patchBytes int) {
	if patchBytes > wordDiffMaxBytes {
		return
	}
	budget := wordDiffMaxWork
	for i := 0; i < len(lines); {
		if lines[i].Class != classRemoved || lines[i].oldLine == 0 {
			i++
			continue
		}
		start := i
		for i < len(lines) && lines[i].Class == classRemoved && lines[i].oldLine > 0 {
			i++
		}
		added := i
		for i < len(lines) && lines[i].Class == classAdded && lines[i].newLine > 0 {
			i++
		}
		count := added - start
		if count != i-added || count > 16 {
			continue
		}
		type pair struct {
			old, new []syntax.Span
			score    int
			ok       bool
		}
		// Reserve the entire alignment matrix before comparing any candidates;
		// missing scores from a depleted budget must never imply uniqueness.
		required := 0
		for a := 0; a < count; a++ {
			for b := 0; b < count; b++ {
				oldText, newText := lines[start+a].rawSource, lines[added+b].rawSource
				if len(oldText) > wordDiffMaxLine || len(newText) > wordDiffMaxLine {
					required = wordDiffMaxWork + 1
					break
				}
				required += 2 * (len(wordTokens(oldText)) + 1) * (len(wordTokens(newText)) + 1)
			}
		}
		if required > budget {
			continue
		}
		pairs := make([]pair, count*count)
		for a := 0; a < count; a++ {
			for b := 0; b < count; b++ {
				left, right, score, ok := compareWords(lines[start+a].rawSource, lines[added+b].rawSource, &budget)
				pairs[a*count+b] = pair{left, right, score, ok}
			}
		}
		aligned := true
		for a := 0; a < count; a++ {
			diagonal := pairs[a*count+a]
			if !diagonal.ok {
				aligned = false
				break
			}
			for b := 0; b < count; b++ {
				if b != a && (pairs[a*count+b].score >= diagonal.score || pairs[b*count+a].score >= diagonal.score) {
					aligned = false
				}
			}
		}
		if !aligned {
			continue
		}
		for a := 0; a < count; a++ {
			pair := pairs[a*count+a]
			lines[start+a].wordChanges = escapedSpans([]byte(lines[start+a].rawSource), pair.old)
			lines[added+a].wordChanges = escapedSpans([]byte(lines[added+a].rawSource), pair.new)
		}
	}
}

func (m *Model) wordHighlightedText(line diffLine, horizontal, width int, prefix string) string {
	prefix = clip(prefix, width)
	runes := []rune(line.Text)
	start := len(string(runes[:min(max(horizontal, 0), len(runes))]))
	text := clip(line.Text[start:], max(0, width-visibleWidth(prefix)))
	spans := cropSpans(line.wordChanges, start, start+len(text), 0)
	out := prefix
	last := 0
	render := func(a, b int, changed bool) {
		if a >= b {
			return
		}
		piece := line
		piece.searchID = searchSourceID{}
		piece.wordChanges = nil
		piece.Text = text[a:b]
		piece.syntax = cropSpans(line.syntax, start+a, start+b, 0)
		styled := m.syntaxText(piece, 0, visibleWidth(piece.Text), "")
		if changed {
			styled = lipgloss.NewStyle().Bold(true).Underline(true).TabWidth(lipgloss.NoTabConversion).Render(styled)
		}
		out += styled
	}
	for _, span := range spans {
		render(last, span.Start, false)
		render(span.Start, span.End, true)
		last = span.End
	}
	render(last, len(text), false)
	return out
}
