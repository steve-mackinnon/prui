package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"prui/internal/source"
)

func TestInboxApplicationRealPaginationPartialCacheAndAccountIsolation(t *testing.T) {
	a, saved := wiringFixture(t)
	ctx := context.Background()
	original, err := a.store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	account := "account-a"
	calls := 0
	failSecond := false
	a.gh = &source.GH{Runner: requestFunc(func(_ context.Context, r source.Request) ([]byte, error) {
		calls++
		if !strings.Contains(strings.Join(r.Args, " "), "author:@me") {
			t.Fatal("account search not wired")
		}
		if failSecond && calls == 2 {
			return nil, errors.New("private fake credential must not be cached")
		}
		count := 100
		more := true
		if calls == 2 {
			count = 23
			more = false
		}
		nodes := make([]any, 0, count)
		for i := 0; i < count; i++ {
			nodes = append(nodes, map[string]any{"number": (calls-1)*100 + i + 1, "title": "triage", "repository": map[string]any{"nameWithOwner": fmt.Sprintf("o/repository-%d", i%2)}, "author": map[string]any{"login": account}, "headRefOid": strings.Repeat("a", 40), "createdAt": "2026-10-01T12:00:00Z", "updatedAt": "2026-10-04T12:00:00Z", "state": "OPEN", "isDraft": false, "reviewDecision": nil, "reviewRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}})
		}
		return json.Marshal(map[string]any{"data": map[string]any{"viewer": map[string]any{"login": account}, "search": map[string]any{"issueCount": 123, "nodes": nodes, "pageInfo": map[string]any{"hasNextPage": more, "endCursor": "next"}}}})
	}), Limits: source.Defaults(), Dir: t.TempDir()}
	o := source.InboxOptions{View: "authored", State: "open", Requests: "all"}
	first, err := a.inbox(ctx, o, true)
	if err != nil || !first.Complete || first.Cached || len(first.Items) != 123 || calls != 2 {
		t.Fatal("real loader pagination not wired", err, len(first.Items), calls)
	}
	if err := a.markInboxRead(ctx, first.Viewer, first.Items[0]); err != nil {
		t.Fatal(err)
	}
	calls = 0
	account = "account-b"
	second, err := a.inbox(ctx, o, true)
	if err != nil || second.Viewer != "account-b" || second.Items[0].Activity != "unknown" {
		t.Fatal("account A read state leaked into B", err)
	}
	calls = 0
	account = "account-a"
	failSecond = true
	partial, err := a.inbox(ctx, o, true)
	if err != nil || partial.Complete || partial.Cached || len(partial.Items) != 100 || partial.Items[0].Activity != "read" {
		t.Fatal("partial refresh lost known evidence/read baseline", err)
	}
	serialized, _ := json.Marshal(partial)
	if strings.Contains(string(serialized), "credential") {
		t.Fatal("transport error leaked into durable metadata")
	}
	a.offline = true
	a.gh = forbiddenGitHub{}
	o.Account = "account-b"
	cached, err := a.inbox(ctx, o, false)
	if err != nil || !cached.Cached || !cached.Complete || len(cached.Items) != 123 || cached.Viewer != "account-b" {
		t.Fatal("offline account capture lost", err)
	}
	o.Account = "account-a"
	cached, err = a.inbox(ctx, o, false)
	if err != nil || !cached.Cached || cached.Complete || len(cached.Items) != 100 || cached.Items[0].Activity != "read" {
		t.Fatal("offline partial evidence mislabelled", err)
	}
	after, err := a.store.Load(saved.ID)
	if err != nil || after.SnapshotReference != original.SnapshotReference || after.Generation != original.Generation || after.Inventory.Comparison.Metadata != original.Inventory.Comparison.Metadata {
		t.Fatal("triage changed pinned review state", err)
	}
}
