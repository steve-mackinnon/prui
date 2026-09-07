package main

import (
	"testing"
)

func TestOptions(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"open", "https://github.com/o/r/pull/1", "--repo", "/tmp/checkout", "--plain"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout"}, false},
		{[]string{"open", "https://github.com/o/r/pull/1"}, false},
		{[]string{"sessions"}, false}, {[]string{"open", "1", "--repo", "x", "--github-repo", "o/r", "extra"}, false},
	} {
		_, e := parseOptions(tc.args)
		if (e == nil) != tc.valid {
			t.Errorf("%v: %v", tc.args, e)
		}
	}
}
