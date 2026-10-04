package source

import (
	"context"
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
