package source

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func remoteCommentFixture(id int64, fields map[string]any) map[string]any {
	record := map[string]any{"id": id, "user": map[string]any{"login": "reviewer"}, "body": "body", "commit_id": "0123456789abcdef0123456789abcdef01234567", "path": "a.go", "side": "RIGHT", "line": 2}
	for key, value := range fields {
		record[key] = value
	}
	return record
}

func TestGitHubListReviewCommentsKeepsInlineThreadsInMixedPage(t *testing.T) {
	records := []map[string]any{
		remoteCommentFixture(1, map[string]any{"subject_type": "file", "line": nil, "side": nil}),
		remoteCommentFixture(2, map[string]any{"subject_type": "line", "line": nil, "original_line": 9}),
		remoteCommentFixture(3, map[string]any{"subject_type": "line"}),
		remoteCommentFixture(4, map[string]any{"in_reply_to_id": 3}),
		remoteCommentFixture(5, map[string]any{"line": nil, "side": nil}),
	}
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		calls++
		want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/owner/repo/pulls/42/comments?per_page=100&page=1"}
		if !reflect.DeepEqual(request.Args, want) {
			t.Fatal(request.Args)
		}
		return data, nil
	})}
	comments, err := g.ListReviewComments(context.Background(), Identity{Repository: "owner/repo", Number: 42})
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %#v, err = %v", comments, err)
	}
	if comments[0].ID != 3 || comments[1].ID != 4 || comments[1].ParentID != 3 || comments[0].Target.Line != 2 || calls != 1 {
		t.Fatal(comments, calls)
	}
}

func TestGitHubListReviewCommentsRejectsMalformedSkippedRecords(t *testing.T) {
	for name, fields := range map[string]map[string]any{
		"zero line":           {"line": 0},
		"negative line":       {"line": -1},
		"unknown subject":     {"subject_type": "other"},
		"invalid side":        {"line": nil, "side": "BOTH"},
		"missing inline side": {"side": nil},
		"invalid commit":      {"subject_type": "file", "line": nil, "commit_id": "bad"},
		"missing path":        {"subject_type": "file", "line": nil, "path": ""},
		"missing author":      {"subject_type": "file", "line": nil, "user": nil},
		"self reply":          {"subject_type": "file", "line": nil, "in_reply_to_id": 1},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal([]map[string]any{remoteCommentFixture(1, fields), remoteCommentFixture(2, nil)})
			if err != nil {
				t.Fatal(err)
			}
			g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return data, nil })}
			if _, err := g.ListReviewComments(context.Background(), Identity{Repository: "owner/repo", Number: 42}); err == nil {
				t.Fatal("accepted malformed record")
			}
		})
	}
}

func TestGitHubListReviewCommentsRejectsDuplicateSkippedRecords(t *testing.T) {
	data, err := json.Marshal([]map[string]any{remoteCommentFixture(1, map[string]any{"subject_type": "file", "line": nil}), remoteCommentFixture(1, nil)})
	if err != nil {
		t.Fatal(err)
	}
	g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return data, nil })}
	if _, err := g.ListReviewComments(context.Background(), Identity{Repository: "owner/repo", Number: 42}); err == nil {
		t.Fatal("accepted duplicate record")
	}
}

func TestWriteCommentResponsesRequireCurrentLineAnchor(t *testing.T) {
	for name, fields := range map[string]map[string]any{
		"file":     {"subject_type": "file"},
		"outdated": {"line": nil, "original_line": 2},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(remoteCommentFixture(1, fields))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseReviewComment(data, Identity{Repository: "owner/repo", Number: 42}); err == nil {
				t.Fatal("accepted unanchorable write response")
			}
		})
	}
}
