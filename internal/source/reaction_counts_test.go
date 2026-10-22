package source

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReactionCountsIgnoreURLAndBoundRemoteValues(t *testing.T) {
	var raw remoteReactionCounts
	if err := json.Unmarshal([]byte(`{"url":"https://github.com/untrusted","+1":5,"-1":-3,"heart":9,"unknown":10,"eyes":1000000001}`), &raw); err != nil {
		t.Fatal(err)
	}
	counts := boundedReactionCounts(raw)
	if counts != (ReactionCounts{5, 0, 0, 0, 9, 0, 0, 0}) {
		t.Fatal(counts)
	}
	event, err := normalizeConversation(remoteConversationEvent{ID: 1, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), User: &struct{ Login string }{"alice"}, Reactions: raw}, "PR comment")
	if err != nil || event.ReactionCounts != counts {
		t.Fatal(event, err)
	}
}

func TestGeneralReactionEndpointAndValidation(t *testing.T) {
	calls := 0
	g := &GH{Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		if strings.Join(r.Args, " ") != "api --hostname github.com --method POST --input - repos/owner/repo/issues/comments/7/reactions" || string(r.Stdin) != `{"content":"heart"}` {
			t.Fatal(r)
		}
		return []byte(`{"id":9,"content":"heart","user":{"login":"alice"}}`), nil
	})}
	reaction, err := g.AddGeneralCommentReaction(context.Background(), Identity{Repository: "owner/repo", Number: 42}, 7, "heart")
	if err != nil || reaction.ID != 9 {
		t.Fatal(reaction, err)
	}
	for _, content := range []string{"bad", "heart; echo nope", ""} {
		if _, err := g.AddGeneralCommentReaction(context.Background(), Identity{Repository: "owner/repo", Number: 42}, 7, content); err == nil {
			t.Fatal("invalid reaction accepted")
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
