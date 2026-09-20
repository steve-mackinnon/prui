package tui

import "testing"

func TestEmojiSupportedUsesLocaleEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  []string
		want bool
	}{
		{"UTF-8 language", []string{"LANG=en_US.UTF-8"}, true},
		{"UTF8 ctype takes precedence", []string{"LANG=C", "LC_CTYPE=en_US.utf8"}, true},
		{"ASCII language", []string{"LANG=C"}, false},
		{"LC_ALL takes precedence", []string{"LANG=en_US.UTF-8", "LC_ALL=C"}, false},
		{"unknown keeps Unicode default", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := emojiSupported(tc.env); got != tc.want {
				t.Fatalf("emojiSupported(%v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}
