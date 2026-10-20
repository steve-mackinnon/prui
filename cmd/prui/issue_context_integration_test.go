package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
)

// Exercises actual GH response normalization and private SQLite persistence
// together, never a live provider.
func TestIssueContextProviderCacheIdentityAndOfflineIntegration(t *testing.T) {
	a, frozen := wiringFixture(t)
	before, err := json.Marshal(frozen.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	githubCalls := 0
	failGithub, partialGithub := false, false
	id := frozen.Inventory.Comparison.Metadata.Identity
	a.gh = &source.GH{Executable: "fixture-gh", Limits: source.Defaults(), Runner: requestFunc(func(ctx context.Context, r source.Request) ([]byte, error) {
		githubCalls++
		if failGithub {
			return nil, errors.New("synthetic account denied")
		}
		var request struct {
			Query     string
			Variables struct {
				Owner, Name string
				Number      int
			}
		}
		if json.Unmarshal(r.Stdin, &request) != nil || !strings.Contains(request.Query, "closingIssuesReferences") || request.Variables.Owner == "" || request.Variables.Number <= 0 {
			t.Fatal("incorrect GH request")
		}
		connection := func(nodes any) any {
			return map[string]any{"nodes": nodes, "pageInfo": map[string]bool{"hasNextPage": partialGithub}}
		}
		pr := map[string]any{"headRefOid": strings.Repeat("c", 40), "reviewDecision": "REVIEW_REQUIRED", "reviewRequests": connection([]any{map[string]any{"requestedReviewer": map[string]string{"__typename": "User", "login": "reviewer"}}}), "latestOpinionatedReviews": connection([]any{}), "labels": connection([]any{map[string]string{"name": "bug"}}), "closingIssuesReferences": connection([]any{map[string]any{"number": 3, "title": "GitHub issue", "state": "OPEN", "url": "https://github.com/owner/repo/issues/3", "repository": map[string]string{"nameWithOwner": "owner/repo"}}})}
		b, e := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pr}}})
		return b, e
	})}
	got, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || !got.Complete || len(got.Requested) != 1 || len(got.Issues) != 1 || got.HeadSHA == frozen.Inventory.Comparison.Metadata.HeadSHA {
		t.Fatal("provider/cache integration", got, err)
	}
	a.offline = true
	cached, err := a.readIssueContext(context.Background(), id, false)
	if err != nil || len(cached.Issues) != 1 || cached.Issues[0].Title != "GitHub issue" || githubCalls != 1 {
		t.Fatal("saved context lost", cached, err)
	}
	for _, other := range []source.Identity{{Repository: "owner/repo", Number: id.Number + 1}, {Repository: "other/repo", Number: id.Number}} {
		if _, e := a.readIssueContext(context.Background(), other, false); !errors.Is(e, session.ErrIssueContextNotFound) {
			t.Fatal("foreign PR cache reused", other, e)
		}
	}
	if _, err = a.readIssueContext(context.Background(), id, true); err == nil || githubCalls != 1 {
		t.Fatal("offline touched providers")
	}
	// Reopen the database for offline access.
	if err = a.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.OpenReadOnly(a.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cached, err = reopened.LoadIssueContext(context.Background(), id)
	if err != nil || len(cached.Issues) != 1 || cached.Issues[0].Title != "GitHub issue" {
		t.Fatal("offline restart lost context", err)
	}
	// Original frozen source is byte-equivalent and independent of current head.
	recovered, err := reopened.Load(frozen.ID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(recovered.Snapshot)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("context changed frozen source", err)
	}
	// Reopen writable for denied-account and partial refresh cases.
	a.store, err = session.Open(reopened.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer a.store.Close()
	a.offline = false
	failGithub = true
	if _, err = a.readIssueContext(context.Background(), id, true); err == nil {
		t.Fatal("denied account looked current")
	}
	retained, err := a.readIssueContext(context.Background(), id, false)
	if err != nil || retained.CapturedAt != got.CapturedAt {
		t.Fatal("failed refresh destroyed cache", err)
	}
	failGithub = false
	partialGithub = true
	partial, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || len(partial.Issues) != 1 || len(partial.Labels) != 1 || partial.Complete || partial.Reason == "" {
		t.Fatal("partial response lost GitHub context", partial, err)
	}
}
