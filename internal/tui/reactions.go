package tui

import (
	"os"
	"strings"
)

// reactionDisplay is presentation-only: GitHub action values stay the bounded
// documented ASCII tokens used by the source contract.
var reactionEmoji = map[string]string{
	"+1":       "👍",
	"-1":       "👎",
	"laugh":    "😄",
	"confused": "😕",
	"heart":    "❤️",
	"hooray":   "🎉",
	"rocket":   "🚀",
	"eyes":     "👀",
}

func (m *Model) reactionLabel(content string) string {
	if m.reactionEmoji {
		if emoji, ok := reactionEmoji[content]; ok {
			return emoji
		}
	}
	return content
}

// emojiSupported uses locale encoding as the narrow capability signal that a
// terminal application can safely inspect. Fonts cannot be probed reliably,
// so an unknown locale keeps the modern UTF-8 default while an explicit
// non-UTF-8 locale gets the GitHub-token fallback.
func emojiSupported(env []string) bool {
	locale := ""
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		prefix := key + "="
		for _, value := range env {
			if strings.HasPrefix(value, prefix) {
				locale = strings.TrimPrefix(value, prefix)
				break
			}
		}
		if locale != "" {
			break
		}
	}
	if locale == "" {
		return true
	}
	locale = strings.ToLower(locale)
	return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
}

func defaultEmojiSupport() bool { return emojiSupported(os.Environ()) }
