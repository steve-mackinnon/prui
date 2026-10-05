package source

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPublishedEditsUseIdentityAndJSONStdin(t *testing.T) {
	for _, general := range []bool{false, true} {
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			if strings.Contains(strings.Join(r.Args, " "), "private edit") {
				t.Fatal("body in argv")
			}
			var p map[string]string
			if json.Unmarshal(r.Stdin, &p) != nil || p["body"] != "private edit" {
				t.Fatal("missing JSON body")
			}
			path := "pulls/comments/123"
			if general {
				path = "issues/comments/123"
			}
			if !strings.HasSuffix(r.Args[len(r.Args)-1], path) || !strings.Contains(strings.Join(r.Args, " "), "PATCH") {
				t.Fatal(r.Args)
			}
			return []byte(`{"id":123,"body":"private edit","created_at":"2026-10-04T00:00:00Z","user":{"login":"alice"}}`), nil
		})}
		got, err := g.EditPublishedComment(context.Background(), Identity{"owner/repo", 42}, 123, general, "private edit")
		if err != nil || got.ID != 123 || got.Body != "private edit" || got.Author != "alice" {
			t.Fatalf("%+v %v", got, err)
		}
	}
}
func TestPublishedMutationsUnknownAndInvalidInput(t *testing.T) {
	calls := 0
	g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { calls++; return nil, errors.New("private diagnostic") })}
	_, err := g.EditPublishedComment(context.Background(), Identity{"owner/repo", 42}, 1, false, "draft")
	if !errors.Is(err, ErrCommentDeliveryUnknown) || strings.Contains(err.Error(), "private diagnostic") {
		t.Fatal(err)
	}
	if _, err = g.EditPublishedComment(context.Background(), Identity{"owner/repo", 42}, 0, false, "draft"); err == nil || calls != 1 {
		t.Fatal("invalid edit reached client")
	}
	_, err = g.SetThreadResolved(context.Background(), Identity{"owner/repo", 42}, "thread", true)
	if !errors.Is(err, ErrCommentDeliveryUnknown) || calls != 2 {
		t.Fatal(err, calls)
	}
}
func TestThreadResolutionCanonicalPermissions(t *testing.T) {
	for _, resolved := range []bool{true, false} {
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
			var p struct {
				Query     string
				Variables map[string]any
			}
			if json.Unmarshal(r.Stdin, &p) != nil || p.Variables["id"] != "thread" {
				t.Fatal("missing stdin thread")
			}
			name := "resolveReviewThread"
			if !resolved {
				name = "unresolveReviewThread"
			}
			if !strings.Contains(p.Query, name) {
				t.Fatal(p.Query)
			}
			return json.Marshal(map[string]any{"data": map[string]any{"action": map[string]any{"thread": map[string]any{"id": "thread", "isResolved": resolved, "viewerCanResolve": !resolved, "viewerCanUnresolve": resolved}}}})
		})}
		got, err := g.SetThreadResolved(context.Background(), Identity{"owner/repo", 42}, "thread", resolved)
		if err != nil || got.Resolved == nil || *got.Resolved != resolved || got.CanResolve == nil || *got.CanResolve == resolved {
			t.Fatalf("%+v %v", got, err)
		}
	}
}

func TestPublishedMalformedCanonicalResponsesStayUncertain(t *testing.T) {
	for _, response := range []string{
		`{"id":124,"body":"edited","user":{"login":"alice"}}`,
		`{"id":123,"body":"different","user":{"login":"alice"}}`,
		`{"id":123,"body":"edited","user":{"login":"\u001bsecret"}}`,
		`{"data":{"action":{"thread":{"id":"other","isResolved":true,"viewerCanResolve":false,"viewerCanUnresolve":true}}}}`,
		`{"data":{"action":{"thread":{"id":"thread","isResolved":true}}}}`,
		`{"errors":[{"message":"private"}],"data":{"action":{"thread":{"id":"thread","isResolved":true,"viewerCanResolve":false,"viewerCanUnresolve":true}}}}`,
	} {
		calls := 0
		g := GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { calls++; return []byte(response), nil })}
		var err error
		if strings.Contains(response, `"id":123`) || strings.Contains(response, `"id":124`) {
			_, err = g.EditPublishedComment(context.Background(), Identity{"owner/repo", 42}, 123, false, "edited")
		} else {
			_, err = g.SetThreadResolved(context.Background(), Identity{"owner/repo", 42}, "thread", true)
		}
		if !errors.Is(err, ErrCommentDeliveryUnknown) || calls != 1 || strings.Contains(err.Error(), "private") {
			t.Fatal("malformed result accepted or retried", err, calls)
		}
	}
}
