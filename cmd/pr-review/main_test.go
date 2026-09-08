package main

import (
	"strings"
	"testing"
)

func TestOptions(t *testing.T) {
	// A developer's real credential must not decide what this suite proves.
	t.Setenv("OPENAI_API_KEY", "")
	for _, tc := range []struct {
		args  []string
		valid bool
	}{
		{[]string{"open", "https://github.com/o/r/pull/1", "--repo", "/tmp/checkout", "--plain"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r"}, true},
		{[]string{"open", "1", "--repo", "/tmp/checkout"}, false},
		{[]string{"open", "https://github.com/o/r/pull/1"}, false},
		{[]string{"sessions"}, true}, {[]string{"open", "1", "--repo", "x", "--github-repo", "o/r", "extra"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--plain"}, true},
		{[]string{"delete", "../outside"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--offline", "--new"}, false},
		{[]string{"sessions", "extra"}, false},
		{[]string{"resume"}, false},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r", "--model", "gpt-6-astra"}, false},
		{[]string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r", "--send-source-to-openai"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--send-source-to-openai"}, false},
		{[]string{"resume", "0123456789abcdef0123456789abcdef", "--model", "gpt-6-astra"}, false},
	} {
		_, e := parseOptions(tc.args)
		if (e == nil) != tc.valid {
			t.Errorf("%v: %v", tc.args, e)
		}
	}
}

func TestOptionsAnalysisOptIn(t *testing.T) {
	base := []string{"open", "1", "--repo", "/tmp/checkout", "--github-repo", "o/r"}
	args := func(extra ...string) []string { return append(append([]string(nil), base...), extra...) }

	t.Setenv("OPENAI_API_KEY", "")
	if _, err := parseOptions(args("--send-source-to-openai")); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatal("opting in without a credential is not refused early", err)
	}
	if _, err := parseOptions(args("--model", "gpt-6-astra")); err == nil || !strings.Contains(err.Error(), "--send-source-to-openai") {
		t.Fatal("a model can be chosen without acknowledging the upload", err)
	}
	o, err := parseOptions(args())
	if err != nil || o.SendSource || o.Model != "" {
		t.Fatal("the default open invocation opts into analysis", o, err)
	}

	t.Setenv("OPENAI_API_KEY", "sk-present")
	o, err = parseOptions(args("--send-source-to-openai", "--model", "gpt-6-astra"))
	if err != nil {
		t.Fatal(err)
	}
	if !o.SendSource || o.Model != "gpt-6-astra" {
		t.Fatal("opt-in flags were not parsed", o)
	}
	// An unknown id is passed through: the API rejects it and that becomes a
	// stated fallback, rather than a local allowlist going stale.
	if o, err := parseOptions(args("--send-source-to-openai", "--model", "not-a-real-model")); err != nil || o.Model != "not-a-real-model" {
		t.Fatal("the model flag is validated against a local list", err)
	}
	if !strings.Contains(usage, "--send-source-to-openai") || !strings.Contains(usage, "OPENAI_API_KEY") {
		t.Fatal("usage does not state the upload boundary")
	}
}
