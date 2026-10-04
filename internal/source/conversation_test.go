package source

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestConversationPaginationAndKinds(t *testing.T) {
	calls := 0
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		endpoint := r.Args[len(r.Args)-1]
		if strings.Contains(endpoint, "/reviews?") {
			return []byte(`[{"id":2,"body":"approved","state":"APPROVED","submitted_at":"2026-10-04T02:00:00Z","user":{"login":"bob"}}]`), nil
		}
		return []byte(`[{"id":1,"body":"hello","created_at":"2026-10-04T01:00:00Z","user":{"login":"alice"}}]`), nil
	})}
	got, err := g.ListConversation(context.Background(), Identity{"owner/repo", 42})
	if err != nil || !got.Complete || len(got.Events) != 2 || calls != 2 {
		t.Fatalf("%+v %v calls=%d", got, err, calls)
	}
	if got.Events[0].Kind != "PR comment" || got.Events[1].Decision != "APPROVED" {
		t.Fatal(got)
	}
}

func TestConversationPaginationLimitsAndPartialFailure(t *testing.T) {
	for _, failAt := range []int{0, 2} {
		calls := 0
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			calls++
			if strings.Contains(r.Args[len(r.Args)-1], "/reviews?") {
				return []byte(`[]`), nil
			}
			if calls == failAt {
				return nil, errors.New("private remote diagnostic")
			}
			rows := make([]map[string]any, 100)
			for i := range rows {
				rows[i] = map[string]any{"id": (calls-1)*100 + i + 1, "body": "comment", "created_at": "2026-10-04T01:00:00Z", "user": map[string]string{"login": "alice"}}
			}
			return json.Marshal(rows)
		})}
		out, err := g.ListConversation(context.Background(), Identity{"owner/repo", 42})
		expected := 500
		if failAt != 0 {
			expected = 100
		}
		if err != nil || out.Complete || out.Reason == "" || len(out.Events) != expected {
			t.Fatalf("%+v %v", out, err)
		}
	}
}
func TestConversationRejectsInvalidAndPendingData(t *testing.T) {
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		if strings.Contains(r.Args[len(r.Args)-1], "/reviews?") {
			return []byte(`[{"id":2,"state":"PENDING"}]`), nil
		}
		return []byte(`[{"id":1,"body":"hello","created_at":"2026-10-04T01:00:00Z","user":null},{"id":1,"body":"duplicate","created_at":"2026-10-04T01:00:00Z"},{"id":3,"body":"bad","created_at":"2026-10-04T01:00:00Z","user":{"login":"bad\u001b"}}]`), nil
	})}
	out, err := g.ListConversation(context.Background(), Identity{"owner/repo", 42})
	if err != nil || out.Complete || len(out.Events) != 1 || out.Events[0].Author != "[deleted]" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestGeneralCommentUsesJSONStdinAndValidatesResponse(t *testing.T) {
	calls := 0
	body := "private body\nnext"
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		if strings.Contains(strings.Join(r.Args, " "), body) || r.Args[len(r.Args)-1] != "repos/owner/repo/issues/42/comments" {
			t.Fatal(r.Args)
		}
		var input map[string]string
		if json.Unmarshal(r.Stdin, &input) != nil || input["body"] != body {
			t.Fatal(string(r.Stdin))
		}
		return json.Marshal(map[string]any{"id": 1, "body": body, "created_at": "2026-10-04T01:00:00Z", "user": map[string]string{"login": "alice"}})
	})}
	if _, err := g.CreateGeneralComment(context.Background(), Identity{"owner/repo", 42}, body); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", string([]byte{0xff}), strings.Repeat("x", 65537)} {
		if _, err := g.CreateGeneralComment(context.Background(), Identity{"owner/repo", 42}, invalid); err == nil {
			t.Fatal("invalid body accepted")
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
func TestConversationUnavailableNeverEmptySuccess(t *testing.T) {
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { return nil, errors.New("secret") })}
	out, err := g.ListConversation(context.Background(), Identity{"owner/repo", 42})
	if err == nil || out.Complete || strings.Contains(err.Error(), "secret") {
		t.Fatalf("%+v %v", out, err)
	}
}
