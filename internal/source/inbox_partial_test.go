package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func inboxFixture() map[string]any {
	return map[string]any{"data": map[string]any{"viewer": map[string]any{"login": "viewer"}, "search": map[string]any{"issueCount": 1, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}, "nodes": []any{map[string]any{"number": 1, "title": "test", "author": map[string]any{"login": "viewer"}, "repository": map[string]any{"nameWithOwner": "o/r"}, "createdAt": "2026-10-01T12:00:00Z", "updatedAt": "2026-10-04T12:00:00Z", "headRefOid": strings.Repeat("a", 40), "state": "OPEN", "isDraft": false, "reviewDecision": nil, "reviews": map[string]any{"totalCount": 0}, "reviewRequests": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}}}}}}
}
func fixtureInbox(t *testing.T, f map[string]any, o InboxOptions) Inbox {
	t.Helper()
	g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) { return json.Marshal(f) }), Limits: Defaults(), Dir: t.TempDir()}
	r, err := g.ReadInbox(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestInboxMissingAndNullEvidenceNeverComplete(t *testing.T) {
	for _, field := range []string{"nodes", "pageInfo", "issueCount"} {
		for _, null := range []bool{false, true} {
			f := inboxFixture()
			search := f["data"].(map[string]any)["search"].(map[string]any)
			if null {
				search[field] = nil
			} else {
				delete(search, field)
			}
			r := fixtureInbox(t, f, InboxOptions{View: "authored"})
			if r.Complete || len(r.Problems) == 0 {
				t.Fatalf("accepted %s null=%t %+v", field, null, r)
			}
		}
	}
	for _, field := range []string{"isDraft", "repository", "headRefOid", "state", "reviewRequests"} {
		for _, null := range []bool{false, true} {
			f := inboxFixture()
			n := f["data"].(map[string]any)["search"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
			if null {
				n[field] = nil
			} else {
				delete(n, field)
			}
			r := fixtureInbox(t, f, InboxOptions{View: "authored"})
			if r.Complete {
				t.Fatalf("accepted %s null=%t", field, null)
			}
			if field == "reviewRequests" && (len(r.Items) != 1 || r.Items[0].RequestsComplete) {
				t.Fatalf("unknown requests %+v", r)
			}
		}
	}
	for _, field := range []string{"nodes", "pageInfo"} {
		f := inboxFixture()
		n := f["data"].(map[string]any)["search"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
		n["reviewRequests"].(map[string]any)[field] = nil
		r := fixtureInbox(t, f, InboxOptions{View: "authored"})
		if r.Complete || r.Items[0].RequestsComplete {
			t.Fatal(r)
		}
	}
	f := inboxFixture()
	s := f["data"].(map[string]any)["search"].(map[string]any)
	s["pageInfo"].(map[string]any)["hasNextPage"] = nil
	r := fixtureInbox(t, f, InboxOptions{View: "authored"})
	if r.Complete {
		t.Fatal(r)
	}
}
func TestInboxAuthoritativeFiltersAndUnknownDecision(t *testing.T) {
	f := inboxFixture()
	r := fixtureInbox(t, f, InboxOptions{View: "authored", Review: "none"})
	if !r.Complete || len(r.Items) != 1 || r.Items[0].ReviewDecision != "UNKNOWN" {
		t.Fatal(r)
	}
	for _, o := range []InboxOptions{{View: "authored", Review: "approved"}, {View: "authored", Review: "required"}, {View: "authored", Repository: "o/other"}, {View: "authored", Author: "other"}, {View: "authored", State: "closed"}, {View: "authored", Draft: "yes"}} {
		r := fixtureInbox(t, f, o)
		if r.Complete || len(r.Items) != 0 {
			t.Fatalf("wrong filter accepted %+v %+v", o, r)
		}
	}
	for _, decision := range []struct{ filter, value string }{{"approved", "APPROVED"}, {"required", "REVIEW_REQUIRED"}, {"changes_requested", "CHANGES_REQUESTED"}} {
		f := inboxFixture()
		n := f["data"].(map[string]any)["search"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
		n["reviewDecision"] = decision.value
		r := fixtureInbox(t, f, InboxOptions{View: "authored", Review: decision.filter})
		if !r.Complete || len(r.Items) != 1 {
			t.Fatal(r)
		}
	}
}
func TestInboxLaterPageErrorRetainsEvidence(t *testing.T) {
	f := inboxFixture()
	s := f["data"].(map[string]any)["search"].(map[string]any)
	s["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "next"}
	s["issueCount"] = 101
	calls := 0
	g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		calls++
		if calls == 2 {
			return nil, errors.New("fake credential should not render")
		}
		return json.Marshal(f)
	}), Limits: Defaults(), Dir: t.TempDir()}
	r, err := g.ReadInbox(context.Background(), InboxOptions{View: "authored"})
	if err != nil || r.Complete || len(r.Items) != 1 || calls != 2 || strings.Contains(strings.Join(r.Problems, " "), "credential") {
		t.Fatalf("%+v %v", r, err)
	}
}

func TestInboxChangingCountsAndConflictingUnionAreIncomplete(t *testing.T) {
	for _, mode := range []string{"count", "union", "coverage"} {
		calls := 0
		g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) {
			calls++
			f := inboxFixture()
			search := f["data"].(map[string]any)["search"].(map[string]any)
			if mode == "count" {
				search["issueCount"] = 3
				if calls == 1 {
					search["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "next"}
				} else {
					search["issueCount"] = 2
					search["nodes"].([]any)[0].(map[string]any)["number"] = 2
				}
			} else if calls == 2 {
				n := search["nodes"].([]any)[0].(map[string]any)
				n["headRefOid"] = strings.Repeat("b", 40)
				if mode == "coverage" {
					n["reviewRequests"] = nil
				}
			}
			return json.Marshal(f)
		}), Limits: Defaults(), Dir: t.TempDir()}
		o := InboxOptions{View: "authored"}
		if mode != "count" {
			o.View = "requested"
		}
		r, err := g.ReadInbox(context.Background(), o)
		if err != nil || r.Complete || calls != 2 || len(r.Items) != map[string]int{"count": 2, "union": 1, "coverage": 1}[mode] {
			t.Fatalf("%s: %+v %v", mode, r, err)
		}
		if mode == "coverage" && r.Items[0].RequestsComplete {
			t.Fatal("merged incomplete evidence lost partial coverage")
		}
	}
}
func TestInboxSearchPaginationIsBounded(t *testing.T) {
	calls := 0
	g := &GH{Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		calls++
		f := inboxFixture()
		s := f["data"].(map[string]any)["search"].(map[string]any)
		s["issueCount"] = 1001
		s["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": fmt.Sprint(calls)}
		s["nodes"].([]any)[0].(map[string]any)["number"] = calls
		return json.Marshal(f)
	}), Limits: Defaults(), Dir: t.TempDir()}
	r, err := g.ReadInbox(context.Background(), InboxOptions{View: "requested"})
	if err != nil || r.Complete || calls != 20 || r.Requests != 20 || len(r.Items) != 20 {
		t.Fatal(r, err, calls)
	}
}
func TestInboxParticipatedIncludesReviewedStream(t *testing.T) {
	calls := 0
	g := &GH{Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		args := strings.Join(r.Args, " ")
		want := "involves:@me"
		if calls == 2 {
			want = "reviewed-by:@me"
		}
		if !strings.Contains(args, want) {
			t.Fatal(args)
		}
		f := inboxFixture()
		f["data"].(map[string]any)["search"].(map[string]any)["nodes"].([]any)[0].(map[string]any)["number"] = calls
		return json.Marshal(f)
	}), Limits: Defaults(), Dir: t.TempDir()}
	r, err := g.ReadInbox(context.Background(), InboxOptions{View: "participated"})
	if err != nil || !r.Complete || calls != 2 || len(r.Items) != 2 {
		t.Fatal(r, err)
	}
}

func TestInboxMissingAuthorAndTitleNeverComplete(t *testing.T) {
	for _, field := range []string{"author", "title"} {
		for _, null := range []bool{false, true} {
			f := inboxFixture()
			n := f["data"].(map[string]any)["search"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
			if null {
				n[field] = nil
			} else {
				delete(n, field)
			}
			r := fixtureInbox(t, f, InboxOptions{View: "requested", Requests: "personal"})
			if r.Complete {
				t.Fatalf("accepted missing %s null=%t", field, null)
			}
			if field == "author" && (len(r.Items) != 1 || r.Items[0].PullRequest.Author != "unknown") {
				t.Fatal("useful row lost explicit unknown author", r)
			}
		}
	}
}
