package source

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestListPullRequestCommitsBoundedValidatedAndNormalized(t *testing.T) {
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/owner/repo/pulls/1/commits?per_page=100&page=1"}
		if !reflect.DeepEqual(r.Args, want) {
			t.Fatal(r.Args)
		}
		return []byte(`[{"sha":"0123456789abcdef0123456789abcdef01234567","commit":{"message":"subject\nbody","author":{"name":"Alice"}}}]`), nil
	})}
	entries, err := g.ListPullRequestCommits(context.Background(), Identity{"owner/repo", 1})
	if err != nil || len(entries) != 1 || entries[0].Subject != "subject" || entries[0].Author != "Alice" {
		t.Fatal(entries, err)
	}
}

func TestListPullRequestCommitsRejectsInvalidRecords(t *testing.T) {
	good := `{"sha":"0123456789abcdef0123456789abcdef01234567","commit":{"message":"ok","author":{"name":"Alice"}}}`
	for _, data := range []string{"null", "[" + good + "," + good + "]", strings.Replace("["+good+"]", "0123456789abcdef0123456789abcdef01234567", "BAD", 1), strings.Replace("["+good+"]", "ok", strings.Repeat("x", 4097), 1), fmt.Sprintf("[%s]", strings.Join(func() []string {
		a := make([]string, 101)
		for i := range a {
			a[i] = good
		}
		return a
	}(), ","))} {
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return []byte(data), nil })}
		if _, err := g.ListPullRequestCommits(context.Background(), Identity{"owner/repo", 1}); err == nil {
			t.Fatal("accepted invalid list")
		}
	}
}

func TestListPullRequestCommitsEmptyAndFullPage(t *testing.T) {
	for _, n := range []int{0, 100, 101} {
		records := make([]map[string]any, n)
		for i := range records {
			records[i] = map[string]any{"sha": fmt.Sprintf("%040x", i), "commit": map[string]any{"message": "ok", "author": map[string]any{"name": "Alice"}}}
		}
		data, err := json.Marshal(records)
		if err != nil {
			t.Fatal(err)
		}
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return data, nil })}
		entries, err := g.ListPullRequestCommits(context.Background(), Identity{"owner/repo", 1})
		if n > 100 {
			if err == nil {
				t.Fatal("accepted more than 100 entries")
			}
			continue
		}
		if err != nil || len(entries) != n {
			t.Fatal(entries, err)
		}
	}
}

func TestListPullRequestCommitsInvalidTextAndIdentity(t *testing.T) {
	for _, entry := range []PullRequestCommit{
		{SHA: strings.Repeat("a", 40), Author: strings.Repeat("a", 257)},
		{SHA: strings.Repeat("a", 40), Subject: "\xff"},
		{SHA: strings.Repeat("a", 40), Subject: "unexpected\nbody"},
	} {
		if ValidatePullRequestCommits([]PullRequestCommit{entry}) == nil {
			t.Fatal("accepted invalid metadata")
		}
	}
	g := GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		t.Fatal("called runner for invalid identity")
		return nil, nil
	})}
	if _, err := g.ListPullRequestCommits(context.Background(), Identity{"../repo", 1}); err == nil {
		t.Fatal("accepted invalid identity")
	}
}
