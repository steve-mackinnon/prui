package main

import "testing"

func TestOptions(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"open", "https://github.com/o/r/pull/1", "--repo", "/tmp/checkout", "--plain"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout"}, false},
		{[]string{"open", "https://github.com/o/r/pull/1"}, true},
		{[]string{"open", "1", "--github-repo", "o/r"}, true},
		{[]string{"sessions"}, true}, {[]string{"open", "1", "--repo", "x", "--github-repo", "o/r", "extra"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--plain"}, true},
		{[]string{"delete", "../outside"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--new"}, false},
		{[]string{"sessions", "extra"}, false},
		{[]string{"resume"}, false},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r", "--model", "gpt-6-astra"}, false},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r", "--send-source-to-openai"}, false},
	} {
		_, e := parseOptions(tc.args)
		if (e == nil) != tc.valid {
			t.Errorf("%v: %v", tc.args, e)
		}
	}
}
