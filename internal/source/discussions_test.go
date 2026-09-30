package source

import (
	"context"
	"encoding/json"
	"testing"
)

func discussionFixture() map[string]any {
	sha := "0123456789abcdef0123456789abcdef01234567"
	return map[string]any{"id": "thread-1", "path": "a.go", "diffSide": "RIGHT", "originalLine": 2, "line": nil, "isOutdated": true, "isResolved": false, "subjectType": "LINE", "comments": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{map[string]any{"fullDatabaseId": "123", "body": "hello", "author": map[string]any{"login": "alice"}, "originalCommit": map[string]any{"oid": sha}, "commit": map[string]any{"oid": sha}, "url": "https://github.com/owner/repo/pull/42#discussion_r123", "diffHunk": "@@ -1 +1 @@", "createdAt": "2026-09-30T00:00:00Z"}}}}
}
func TestDiscussionsKeepHistoricalThread(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"reviewThreads": map[string]any{"nodes": []any{discussionFixture()}, "pageInfo": map[string]any{"hasNextPage": false}}}}}})
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		var p map[string]any
		if json.Unmarshal(r.Stdin, &p) != nil || p["query"] == nil {
			t.Fatal("missing stdin query")
		}
		return data, nil
	})}
	got, err := g.ListDiscussions(context.Background(), Identity{"owner/repo", 42})
	if err != nil || !got.Complete || len(got.Threads) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	d := got.Threads[0]
	if d.OriginalAnchor == nil || d.CurrentAnchor != nil || d.Outdated == nil || !*d.Outdated || *d.Resolved || d.Comments[0].ID != 123 {
		t.Fatalf("%+v", d)
	}
}

func TestDiscussionsPaginateRepliesAndThreads(t *testing.T) {
	calls := 0
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		var input struct {
			Query     string
			Variables map[string]any
		}
		if json.Unmarshal(r.Stdin, &input) != nil {
			t.Fatal("invalid stdin")
		}
		if calls == 1 {
			d := discussionFixture()
			d["comments"].(map[string]any)["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "reply-page"}
			return json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"reviewThreads": map[string]any{"nodes": []any{d}, "pageInfo": map[string]any{"hasNextPage": true, "endCursor": "thread-page"}}}}}})
		}
		if calls == 2 {
			if input.Variables["id"] != "thread-1" || input.Variables["cursor"] != "reply-page" {
				t.Fatal(input)
			}
			return []byte(`{"data":{"node":{"comments":{"nodes":[{"fullDatabaseId":"124","body":"reply","author":{"login":"bob"},"replyTo":{"fullDatabaseId":"123"}}],"pageInfo":{"hasNextPage":false}}}}}`), nil
		}
		if calls != 3 || input.Variables["cursor"] != "thread-page" {
			t.Fatal(input, calls)
		}
		return []byte(`{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}}`), nil
	})}
	out, err := g.ListDiscussions(context.Background(), Identity{"owner/repo", 42})
	if err != nil || !out.Complete || calls != 3 || len(out.Threads[0].Comments) != 2 || out.Threads[0].Comments[1].ParentID != 123 {
		t.Fatalf("%+v %v calls=%d", out, err, calls)
	}
}
func TestDiscussionsRetainPartialOnNestedFailure(t *testing.T) {
	calls := 0
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		calls++
		if calls > 1 {
			return nil, context.DeadlineExceeded
		}
		d := discussionFixture()
		d["comments"].(map[string]any)["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "next"}
		return json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"reviewThreads": map[string]any{"nodes": []any{d}, "pageInfo": map[string]any{"hasNextPage": false}}}}}})
	})}
	out, err := g.ListDiscussions(context.Background(), Identity{"owner/repo", 42})
	if err != nil || out.Complete || out.Reason == "" || len(out.Threads) != 1 {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestDiscussionNormalizationNeverGuessesAnchors(t *testing.T) {
	for _, mutate := range []func(*remoteDiscussion){func(d *remoteDiscussion) { n := 1; d.OriginalStartLine = &n }, func(d *remoteDiscussion) { d.SubjectType = "FILE" }, func(d *remoteDiscussion) { d.Comments.Nodes[0].OriginalCommit = nil }, func(d *remoteDiscussion) {
		d.Comments.Nodes[0].ReplyTo = &struct{ FullDatabaseID json.RawMessage }{json.RawMessage(`"99"`)}
	}} {
		b, _ := json.Marshal(discussionFixture())
		var raw remoteDiscussion
		_ = json.Unmarshal(b, &raw)
		mutate(&raw)
		d, _, e := normalizeDiscussion(&raw, Identity{"owner/repo", 42})
		if e != nil || d.OriginalAnchor != nil {
			t.Fatalf("%+v %v", d, e)
		}
	}
}
func TestDiscussionNormalizationBoundsAndURLs(t *testing.T) {
	for _, mutate := range []func(*remoteDiscussion){func(d *remoteDiscussion) { d.Path = "a\x1b.go" }, func(d *remoteDiscussion) { d.Comments.Nodes[0].Body = string(make([]byte, 65537)) }, func(d *remoteDiscussion) {
		d.Comments.Nodes[0].FullDatabaseID = json.RawMessage(`"9223372036854775808"`)
	}} {
		b, _ := json.Marshal(discussionFixture())
		var raw remoteDiscussion
		_ = json.Unmarshal(b, &raw)
		mutate(&raw)
		if _, _, e := normalizeDiscussion(&raw, Identity{"owner/repo", 42}); e == nil {
			t.Fatal("accepted malformed discussion")
		}
	}
	for _, s := range []string{"https://evil.test/a", "https://github.com@evil.test/a", "https://github.com/a\n", "http://github.com/a"} {
		if discussionURL(s) != "" {
			t.Fatal(s)
		}
	}
}
func TestHistoricalPostRetainsCanonicalOriginalAnchor(t *testing.T) {
	id := Identity{"owner/repo", 42}
	sha := "0123456789abcdef0123456789abcdef01234567"
	data, _ := json.Marshal(remoteCommentFixture(1, map[string]any{"line": nil, "original_line": 2, "original_commit_id": sha}))
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return data, nil })}
	want := ReviewComment{Body: "body", Target: ReviewCommentTarget{Identity: id, CommitID: sha, Path: "a.go", Side: "RIGHT", Line: 2}}
	got, err := g.CreateReviewComment(context.Background(), want)
	if err != nil || got.ID != 1 || got.Target.Line != 0 || got.OriginalAnchor == nil || *got.OriginalAnchor != want.Target {
		t.Fatalf("%+v %v", got, err)
	}
	want.Target.Line = 0
	if _, err := g.CreateReviewComment(context.Background(), want); err == nil {
		t.Fatal("accepted invalid outbound target")
	}
}
