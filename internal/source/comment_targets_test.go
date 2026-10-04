package source

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestExtendedCommentTargets(t *testing.T) {
	base := ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Path: "a", Side: "LEFT", Line: 5, StartLine: 2, StartSide: "LEFT"}
	if ValidateReviewCommentTarget(base) != nil {
		t.Fatal("valid range rejected")
	}
	for _, mutate := range []func(*ReviewCommentTarget){func(t *ReviewCommentTarget) { t.StartLine = 6 }, func(t *ReviewCommentTarget) { t.StartSide = "RIGHT" }, func(t *ReviewCommentTarget) { t.SubjectType = "file" }, func(t *ReviewCommentTarget) { t.StartLine = 0 }} {
		bad := base
		mutate(&bad)
		if ValidateReviewCommentTarget(bad) == nil {
			t.Fatalf("invalid target accepted: %+v", bad)
		}
	}
	file := base
	file.SubjectType = "file"
	file.Line = 0
	file.Side = ""
	file.StartLine = 0
	file.StartSide = ""
	if ValidateReviewCommentTarget(file) != nil {
		t.Fatal("file target rejected")
	}
	var requests []Request
	runner := listRunner(func(_ context.Context, q Request) ([]byte, error) {
		requests = append(requests, q)
		return []byte(`{"id":3,"body":"body","commit_id":"` + file.CommitID + `","path":"a","subject_type":"file","user":{"login":"alice"}}`), nil
	})
	gh := &GH{Runner: runner, Executable: "gh", Limits: Defaults()}
	if _, err := gh.CreateReviewComment(context.Background(), ReviewComment{Target: file, Body: "body"}); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if json.Unmarshal(requests[0].Stdin, &payload) != nil {
		t.Fatal("payload")
	}
	for _, key := range []string{"line", "side", "start_line", "start_side"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("file invented %s", key)
		}
	}
	if err := gh.CreatePullRequestReview(context.Background(), PullRequestReview{Identity: file.Identity, CommitID: file.CommitID, Event: "COMMENT", Body: "summary", Comments: []ReviewComment{{Target: file, Body: "body"}}}); err == nil {
		t.Fatal("unsupported queued file accepted")
	}
	if len(requests) != 1 {
		t.Fatal("unsupported target wrote remotely")
	}
}

func TestRangePayloadAndReconciliationKeepBothEndpoints(t *testing.T) {
	for _, side := range []string{"LEFT", "RIGHT"} {
		t.Run(side, func(t *testing.T) {
			target := ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Path: "a", Side: side, Line: 5, StartLine: 2, StartSide: side}
			want := ReviewComment{Target: target, Body: "body"}
			calls := 0
			record := map[string]any{"id": 3, "body": "body", "commit_id": target.CommitID, "original_commit_id": target.CommitID, "path": "a", "side": side, "start_side": side, "line": 5, "start_line": 2, "original_line": 5, "original_start_line": 2, "user": map[string]string{"login": "alice"}}
			gh := &GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, q Request) ([]byte, error) {
				calls++
				if calls <= 2 {
					var payload struct {
						commentPayload
						Comments []commentPayload
					}
					if json.Unmarshal(q.Stdin, &payload) != nil {
						t.Fatal("payload")
					}
					actual := payload.commentPayload
					if calls == 2 {
						if len(payload.Comments) != 1 {
							t.Fatal("comments")
						}
						actual = payload.Comments[0]
					}
					if actual.StartLine != 2 || actual.Line != 5 || actual.Side != side || actual.StartSide != side {
						t.Fatalf("flattened payload: %+v", actual)
					}
					return json.Marshal(record)
				}
				if strings.HasSuffix(q.Args[len(q.Args)-1], "user") {
					return []byte(`{"login":"alice"}`), nil
				}
				return json.Marshal([]any{record})
			})}
			got, err := gh.CreateReviewComment(context.Background(), want)
			if err != nil || got.Target != target {
				t.Fatal("range response lost endpoints", got, err)
			}
			if err := gh.CreatePullRequestReview(context.Background(), PullRequestReview{Identity: target.Identity, CommitID: target.CommitID, Event: "COMMENT", Body: "summary", Comments: []ReviewComment{want}}); err != nil {
				t.Fatal(err)
			}
			matched, err := gh.CommentDraftOutcome(context.Background(), want, 0)
			if err != nil || !matched {
				t.Fatal("range failed reconciliation", err)
			}
			wrong := want
			wrong.Target.StartLine = 1
			matched, err = gh.CommentDraftOutcome(context.Background(), wrong, 0)
			if err != nil || matched {
				t.Fatal("different range reconciled", err)
			}
		})
	}
}
