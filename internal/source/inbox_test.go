package source

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInboxAccountPagination(t *testing.T) {
	calls := 0
	g := &GH{Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		args := strings.Join(r.Args, " ")
		if !strings.Contains(args, "author:@me") || strings.Contains(args, "repo:") {
			t.Fatalf("not account search: %s", args)
		}
		count := 100
		more := true
		if calls == 2 {
			count = 23
			more = false
		}
		nodes := []map[string]any{}
		for i := 0; i < count; i++ {
			nodes = append(nodes, map[string]any{"number": (calls-1)*100 + i + 1, "title": "test", "repository": map[string]string{"nameWithOwner": fmt.Sprintf("o/r%d", i%2)}, "updatedAt": "2026-10-04T12:00:00Z", "createdAt": "2026-10-01T12:00:00Z", "headRefOid": strings.Repeat("a", 40), "state": "OPEN", "isDraft": false, "reviewRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}, "author": map[string]string{"login": "viewer"}})
		}
		return json.Marshal(map[string]any{"data": map[string]any{"viewer": map[string]string{"login": "viewer"}, "search": map[string]any{"issueCount": 123, "nodes": nodes, "pageInfo": map[string]any{"hasNextPage": more, "endCursor": fmt.Sprint(calls)}}}})
	}), Limits: Defaults(), Dir: t.TempDir()}
	result, err := g.ReadInbox(context.Background(), InboxOptions{View: "authored", State: "open", Requests: "all"})
	if err != nil || !result.Complete || len(result.Items) != 123 || calls != 2 {
		t.Fatalf("%+v %v calls=%d", result, err, calls)
	}
}
