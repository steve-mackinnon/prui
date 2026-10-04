package source

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDraftOutcomeChecksIdentityBodyAnchorAndParentWithoutWrites(t *testing.T) {
	id := Identity{"owner/repo", 42}
	want := ReviewComment{Target: ReviewCommentTarget{Identity: id, CommitID: "0123456789abcdef0123456789abcdef01234567", Path: "a.go", Side: "RIGHT", Line: 2}, Body: "body"}
	for _, tc := range []struct {
		name   string
		fields map[string]any
		parent int64
		found  bool
	}{
		{"exact", nil, 0, true},
		{"other author", map[string]any{"user": map[string]any{"login": "someone"}}, 0, false},
		{"other body", map[string]any{"body": "different"}, 0, false},
		{"other line", map[string]any{"line": 9}, 0, false},
		{"reply", map[string]any{"in_reply_to_id": 3}, 3, true},
		{"other parent", map[string]any{"in_reply_to_id": 4}, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
				if strings.Contains(strings.Join(r.Args, " "), "POST") || len(r.Stdin) > 0 {
					t.Fatal("reconciliation wrote")
				}
				if r.Args[len(r.Args)-1] == "user" {
					return []byte(`{"login":"reviewer"}`), nil
				}
				return json.Marshal([]any{remoteCommentFixture(7, tc.fields)})
			})}
			found, err := g.CommentDraftOutcome(context.Background(), want, tc.parent)
			if err != nil || found != tc.found {
				t.Fatalf("found=%v err=%v", found, err)
			}
		})
	}
}
func TestDraftOutcomeNeverTreatsTruncatedOrMalformedHistoryAsAbsent(t *testing.T) {
	want := ReviewComment{Target: ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Path: "a", Side: "RIGHT", Line: 1}, Body: "body"}
	for _, malformed := range []bool{false, true} {
		g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			if r.Args[len(r.Args)-1] == "user" {
				return []byte(`{"login":"reviewer"}`), nil
			}
			if malformed {
				return []byte(`[{}]`), nil
			}
			items := make([]any, 100)
			for i := range items {
				items[i] = remoteCommentFixture(int64(i+1), nil)
			}
			return json.Marshal(items)
		})}
		if _, err := g.CommentDraftOutcome(context.Background(), want, 0); err == nil {
			t.Fatal("incomplete history proved absence")
		}
	}
}
func TestDraftOutcomeReviewIncludesPendingCommentBodiesAndAnchors(t *testing.T) {
	want := PullRequestReview{Identity: Identity{"owner/repo", 42}, CommitID: "0123456789abcdef0123456789abcdef01234567", Event: "COMMENT", Body: "summary", Comments: []ReviewComment{{Target: ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: "0123456789abcdef0123456789abcdef01234567", Path: "a.go", Side: "RIGHT", Line: 2}, Body: "body"}}}
	for _, body := range []string{"body", "other"} {
		g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			endpoint := r.Args[len(r.Args)-1]
			if endpoint == "user" {
				return []byte(`{"login":"reviewer"}`), nil
			}
			if strings.Contains(endpoint, "/9/comments") {
				return json.Marshal([]any{remoteCommentFixture(7, map[string]any{"body": body})})
			}
			return []byte(`[{"id":9,"body":"summary","state":"COMMENTED","commit_id":"0123456789abcdef0123456789abcdef01234567","user":{"login":"reviewer"}}]`), nil
		})}
		found, err := g.ReviewDraftOutcome(context.Background(), want)
		if err != nil || found != (body == "body") {
			t.Fatalf("%s found=%v err=%v", body, found, err)
		}
	}
}

func TestDraftOutcomeRejectsNullAndMalformedReviewFields(t *testing.T) {
	want := PullRequestReview{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Event: "APPROVE"}
	for _, history := range []string{"null", `[{"id":9,"body":"","commit_id":"` + want.CommitID + `","user":{"login":"reviewer"}}]`, `[{"id":9,"body":null,"state":"APPROVED","commit_id":"` + want.CommitID + `","user":{"login":"reviewer"}}]`} {
		g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			if r.Args[len(r.Args)-1] == "user" {
				return []byte(`{"login":"reviewer"}`), nil
			}
			return []byte(history), nil
		})}
		if _, err := g.ReviewDraftOutcome(context.Background(), want); err == nil {
			t.Fatalf("accepted malformed history %s", history)
		}
	}
	for _, comments := range []bool{false, true} {
		g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			endpoint := r.Args[len(r.Args)-1]
			if endpoint == "user" {
				return []byte(`{"login":"reviewer"}`), nil
			}
			if comments || strings.Contains(endpoint, "/9/comments") {
				return []byte("null"), nil
			}
			return []byte(`[{"id":9,"body":"","state":"APPROVED","commit_id":"` + want.CommitID + `","user":{"login":"reviewer"}}]`), nil
		})}
		if comments {
			c := ReviewComment{Target: ReviewCommentTarget{Identity: want.Identity, CommitID: want.CommitID, Path: "a", Side: "RIGHT", Line: 1}, Body: "body"}
			if _, err := g.CommentDraftOutcome(context.Background(), c, 0); err == nil {
				t.Fatal("null comments proved absence")
			}
		} else if _, err := g.ReviewDraftOutcome(context.Background(), want); err == nil {
			t.Fatal("null review comments proved absence")
		}
	}
}
