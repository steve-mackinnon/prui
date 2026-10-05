package source

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestInboxFiltersAndRequestSemantics(t *testing.T) {
	for _, view := range []string{"requested", "authored", "participated"} {
		for _, requests := range []string{"all", "personal", "team"} {
			o := InboxOptions{View: view, Requests: requests, Repository: "o/r", Author: "other", Review: "approved", State: "closed", Draft: "no"}
			if err := o.Validate(); err != nil {
				t.Fatal(err)
			}
			q := o.query("personal")
			for _, want := range []string{"repo:o/r", "author:other", "review:approved", "is:closed", "draft:false"} {
				if !strings.Contains(q, want) {
					t.Fatal(q)
				}
			}
			if view == "participated" && !strings.Contains(q, "involves:@me") {
				t.Fatal(q)
			}
		}
	}
	for _, o := range []InboxOptions{{View: "all"}, {View: "requested", Author: "a OR b"}, {View: "requested", Repository: "o/r is:closed"}, {View: "authored", Draft: "true"}, {View: "participated", Activity: "unread"}} {
		if o.Validate() == nil {
			t.Fatalf("accepted %+v", o)
		}
	}
}
func TestInboxPartialAndRequestUnion(t *testing.T) {
	calls := 0
	g := &GH{Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		team := strings.Contains(strings.Join(r.Args, " "), "team-review-requested-user")
		n := map[string]any{"number": 1, "title": "test", "repository": map[string]string{"nameWithOwner": "o/r"}, "updatedAt": "2026-10-04T12:00:00Z", "createdAt": "2026-10-01T12:00:00Z", "headRefOid": strings.Repeat("a", 40), "state": "OPEN", "isDraft": false, "reviewRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}}
		return json.Marshal(map[string]any{"data": map[string]any{"viewer": map[string]string{"login": "viewer"}, "search": map[string]any{"issueCount": 1, "nodes": []any{n}, "pageInfo": map[string]any{"hasNextPage": team, "endCursor": ""}}}})
	}), Limits: Defaults(), Dir: t.TempDir()}
	r, err := g.ReadInbox(context.Background(), InboxOptions{View: "requested", Requests: "all"})
	if err != nil || calls != 2 || r.Complete || len(r.Items) != 1 || !r.Items[0].PersonalRequest || !r.Items[0].TeamRequest {
		t.Fatalf("%+v %v", r, err)
	}
}
