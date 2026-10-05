package source

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func issueContextResponse(t *testing.T, more bool) []byte {
	t.Helper()
	connection := func(nodes any) any {
		return map[string]any{"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": more}}
	}
	pr := map[string]any{"headRefOid": strings.Repeat("a", 40), "body": "https://linear.app/work/issue/APP-2/fix", "reviewDecision": "CHANGES_REQUESTED",
		"reviewRequests":           connection([]any{map[string]any{"requestedReviewer": map[string]any{"__typename": "User", "login": "alice"}}, map[string]any{"requestedReviewer": map[string]any{"__typename": "Team", "slug": "review", "organization": map[string]any{"login": "org"}}}}),
		"latestOpinionatedReviews": connection([]any{map[string]any{"author": map[string]any{"login": "bob"}, "state": "CHANGES_REQUESTED"}}),
		"labels":                   connection([]any{map[string]any{"name": "bug"}}),
		"closingIssuesReferences":  connection([]any{map[string]any{"number": 2, "title": "Fix it", "state": "OPEN", "url": "https://github.com/o/r/issues/2", "repository": map[string]any{"nameWithOwner": "o/r"}}})}
	b, err := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pr}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestIssueContextBoundedGitHub(t *testing.T) {
	for _, more := range []bool{false, true} {
		g := &GH{Executable: "gh", Limits: Limits{Operation: 60 * time.Second}, Runner: listRunner(func(ctx context.Context, r Request) ([]byte, error) {
			if strings.Contains(strings.Join(r.Args, " "), "--paginate") || r.Limit > 1<<20 || r.Timeout > 60*time.Second {
				t.Fatal("unbounded request")
			}
			var p struct {
				Query     string
				Variables map[string]any
			}
			if json.Unmarshal(r.Stdin, &p) != nil || !strings.Contains(p.Query, "closingIssuesReferences(first:100)") || p.Variables["number"] != float64(1) {
				t.Fatalf("bad request %s", r.Stdin)
			}
			return issueContextResponse(t, more), nil
		})}
		got, err := g.ReadIssueContext(context.Background(), Identity{"o/r", 1})
		if err != nil {
			t.Fatal(err)
		}
		if got.Complete == more || got.ReviewDecision != "CHANGES_REQUESTED" || len(got.Requested) != 2 || got.Requested[1].Name != "org/review" || got.Reviews[0].Decision != "CHANGES_REQUESTED" || got.Issues[0].URL != "https://github.com/o/r/issues/2" || got.Body == "" {
			t.Fatalf("bad context %#v", got)
		}
	}
}
func TestIssueContextMalformedAndGraphQLErrors(t *testing.T) {
	for _, b := range [][]byte{[]byte(`{}`), []byte(`{"errors":[{"message":"secret"}],"data":null}`), []byte(`{"data":{"repository":{"pullRequest":{"headRefOid":"oops"}}}}`)} {
		g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) { return b, nil })}
		if _, err := g.ReadIssueContext(context.Background(), Identity{"o/r", 1}); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid response accepted or leaked", err)
		}
	}
}

func TestIssueContextPartialMissingNodesAndGraphQLError(t *testing.T) {
	for _, nodes := range []string{"null", "missing", "error"} {
		var p map[string]any
		if err := json.Unmarshal(issueContextResponse(t, false), &p); err != nil {
			t.Fatal(err)
		}
		pr := p["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)
		conn := pr["labels"].(map[string]any)
		switch nodes {
		case "null":
			conn["nodes"] = nil
		case "missing":
			delete(conn, "nodes")
		case "error":
			p["errors"] = []any{map[string]any{"message": "private diagnostic"}}
		}
		b, _ := json.Marshal(p)
		g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) { return b, nil })}
		got, err := g.ReadIssueContext(context.Background(), Identity{"o/r", 1})
		if err != nil || got.Complete || len(got.Issues) != 1 || len(got.Requested) != 2 || got.Reason == "" || strings.Contains(got.Reason, "private") {
			t.Fatal("partial lost honest state", got, err)
		}
	}
}
