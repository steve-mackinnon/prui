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

func TestMalformedRangeHistoryNeverProvesAbsence(t *testing.T) {
	target := ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Path: "a", Side: "RIGHT", Line: 5, StartLine: 2, StartSide: "RIGHT"}
	want := ReviewComment{Target: target, Body: "body"}
	for _, bad := range []map[string]any{{"start_line": -1}, {"start_side": ""}, {"start_line": 5}, {"original_start_line": -1}, {"start_line": nil, "start_side": "RIGHT"}} {
		record := map[string]any{"id": 3, "body": "body", "commit_id": target.CommitID, "path": "a", "side": "RIGHT", "line": 5, "start_line": 2, "start_side": "RIGHT", "user": map[string]string{"login": "alice"}}
		for k, v := range bad {
			record[k] = v
		}
		gh := &GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, q Request) ([]byte, error) {
			endpoint := q.Args[len(q.Args)-1]
			if endpoint == "user" {
				return []byte(`{"login":"alice"}`), nil
			}
			if strings.Contains(endpoint, "reviews?") {
				return []byte(`[{"id":9,"body":"summary","state":"COMMENTED","commit_id":"` + target.CommitID + `","user":{"login":"alice"}}]`), nil
			}
			return json.Marshal([]any{record})
		})}
		if matched, err := gh.CommentDraftOutcome(context.Background(), want, 0); err == nil || matched {
			t.Fatalf("malformed comment history proved absence: %v", bad)
		}
		if matched, err := gh.ReviewDraftOutcome(context.Background(), PullRequestReview{Identity: target.Identity, CommitID: target.CommitID, Event: "COMMENT", Body: "summary", Comments: []ReviewComment{want}}); err == nil || matched {
			t.Fatalf("malformed review history proved absence: %v", bad)
		}
	}
}

func TestDiscussionRefreshPreservesExtendedTargets(t *testing.T) {
	for _, file := range []bool{false, true} {
		rawMap := discussionFixture()
		rawMap["line"] = 5
		rawMap["originalLine"] = 5
		rawMap["startLine"] = 2
		rawMap["originalStartLine"] = 2
		rawMap["startDiffSide"] = "RIGHT"
		if file {
			rawMap["subjectType"] = "FILE"
			rawMap["line"] = nil
			rawMap["originalLine"] = nil
			rawMap["startLine"] = nil
			rawMap["originalStartLine"] = nil
			rawMap["startDiffSide"] = nil
		}
		data, _ := json.Marshal(rawMap)
		var raw remoteDiscussion
		if json.Unmarshal(data, &raw) != nil {
			t.Fatal("fixture")
		}
		d, _, err := normalizeDiscussion(&raw, Identity{"owner/repo", 42})
		if err != nil || d.CurrentAnchor == nil || d.OriginalAnchor == nil {
			t.Fatal("lost refreshed target", d, err)
		}
		if file {
			if d.CurrentAnchor.SubjectType != "file" || d.CurrentAnchor.Line != 0 || d.CurrentAnchor.Side != "" {
				t.Fatal("file invented coordinates")
			}
		} else {
			if d.CurrentAnchor.StartLine != 2 || d.CurrentAnchor.StartSide != "RIGHT" || d.CurrentAnchor.Line != 5 {
				t.Fatal("range flattened")
			}
		}
		if d.Comments[0].Target != *d.CurrentAnchor || d.Comments[0].OriginalAnchor == nil || *d.Comments[0].OriginalAnchor != *d.OriginalAnchor {
			t.Fatal("comment anchor lost")
		}
	}
	if !strings.Contains(discussionsQuery, "startDiffSide") {
		t.Fatal("query omits range-side provenance")
	}
}

func TestMalformedFileHistoryNeverProvesAbsence(t *testing.T) {
	target := ReviewCommentTarget{Identity: Identity{"owner/repo", 42}, CommitID: strings.Repeat("a", 40), Path: "a", SubjectType: "file"}
	want := ReviewComment{Target: target, Body: "body"}
	for _, key := range []string{"line", "original_line", "start_line", "original_start_line", "start_side"} {
		record := map[string]any{"id": 3, "body": "body", "commit_id": target.CommitID, "path": "a", "subject_type": "file", "user": map[string]string{"login": "alice"}}
		record[key] = 5
		if key == "start_side" {
			record[key] = "RIGHT"
		}
		gh := &GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, q Request) ([]byte, error) {
			if q.Args[len(q.Args)-1] == "user" {
				return []byte(`{"login":"alice"}`), nil
			}
			return json.Marshal([]any{record})
		})}
		if matched, err := gh.CommentDraftOutcome(context.Background(), want, 0); err == nil || matched {
			t.Fatalf("malformed file %s unlocked retry", key)
		}
		if _, _, err := parseRemoteReviewComment(mustCommentJSON(t, record), target.Identity); err == nil {
			t.Fatalf("malformed file %s accepted", key)
		}
	}
	// A file request cannot be dispatched as an immutable batch review at all.
	gh := &GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		t.Fatal("unsupported file review reached history")
		return nil, nil
	})}
	if _, err := gh.ReviewDraftOutcome(context.Background(), PullRequestReview{Identity: target.Identity, CommitID: target.CommitID, Event: "COMMENT", Body: "summary", Comments: []ReviewComment{want}}); err == nil {
		t.Fatal("file review outcome accepted")
	}
}
func mustCommentJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
