package source

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// SuggestionBody chooses a fence that cannot be closed by replacement source.
func SuggestionBody(replacement string) (string, error) {
	if !utf8.ValidString(replacement) || strings.ContainsRune(replacement, 0) || len(replacement) > 64<<10 {
		return "", errors.New("invalid or oversized suggestion replacement")
	}
	fence := "```"
	for strings.Contains(replacement, fence) {
		fence += "`"
	}
	return fence + "suggestion\n" + replacement + "\n" + fence, nil
}

// ParseSuggestion accepts exactly one ordinary GitHub suggestion fence. Offset
// fences and multiple blocks need separate anchor semantics and are refused.
func ParseSuggestion(body string) (string, error) {
	lines := strings.Split(body, "\n")
	start, end := -1, -1
	openerIndent := 0
	marker := byte(0)
	length := 0
	activeSuggestion := false
	for i, line := range lines {
		ch, n, info, ok := suggestionFence(line)
		if marker != 0 {
			if ok && ch == marker && n >= length && info == "" {
				if activeSuggestion {
					end = i
				}
				marker = 0
				activeSuggestion = false
			}
			continue
		}
		if !ok {
			continue
		}
		if ch == '`' && strings.HasPrefix(info, "suggestion") {
			if info != "suggestion" {
				return "", errors.New("offset suggestion anchors are unsupported")
			}
			if start >= 0 {
				return "", errors.New("multiple suggestions are unsupported")
			}
			start = i
			openerIndent = len(line) - len(strings.TrimLeft(line, " "))
			activeSuggestion = true
		}
		marker = ch
		length = n
	}
	if start < 0 || end <= start {
		return "", errors.New("no complete supported suggestion")
	}
	content := append([]string(nil), lines[start+1:end]...)
	for i, line := range content {
		remove := 0
		for remove < openerIndent && remove < len(line) && line[remove] == ' ' {
			remove++
		}
		content[i] = line[remove:]
	}
	replacement := strings.Join(content, "\n")
	_, err := SuggestionBody(replacement)
	return replacement, err
}

// Only top-level CommonMark fenced blocks are actionable. Indented/quoted
// examples and contents of either backtick or tilde fences remain ordinary prose.
func suggestionFence(line string) (byte, int, string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent > 3 || indent >= len(line) {
		return 0, 0, "", false
	}
	text := line[indent:]
	ch := text[0]
	if ch != '`' && ch != '~' {
		return 0, 0, "", false
	}
	n := 0
	for n < len(text) && text[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	return ch, n, strings.TrimSpace(text[n:]), true
}

func ReplaceSuggestion(content string, t ReviewCommentTarget, before, replacement string) (string, error) {
	if ValidateReviewCommentTarget(t) != nil || t.Side != "RIGHT" || t.SubjectType != "" {
		return "", errors.New("suggestions require a current right-side line or range")
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) || len(content) > 512<<10 {
		return "", errors.New("unsupported suggestion file")
	}
	if _, err := SuggestionBody(replacement); err != nil {
		return "", err
	}
	start := t.StartLine
	if start == 0 {
		start = t.Line
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if start < 1 || t.Line > len(lines) {
		return "", errors.New("suggestion range unavailable")
	}
	selected := strings.Join(lines[start-1:t.Line], "")
	if strings.TrimSuffix(selected, "\n") != before {
		return "", errors.New("suggestion conflicts with current source")
	}
	after := replacement
	if replacement != "" && strings.HasSuffix(selected, "\n") {
		after += "\n"
	}
	result := strings.Join(lines[:start-1], "") + after + strings.Join(lines[t.Line:], "")
	if result == content {
		return "", errors.New("suggestion already applied or unchanged")
	}
	return result, nil
}
