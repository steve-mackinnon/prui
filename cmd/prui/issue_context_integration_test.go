package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
)

// Exercises actual GH response normalization, real Linear request/response
// handling and private SQLite persistence together, never a live provider.
func TestIssueContextProviderCacheIdentityAndOfflineIntegration(t *testing.T) {
	a, frozen := wiringFixture(t)
	before, err := json.Marshal(frozen.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	configDir := t.TempDir()
	a.guideConfigPath = filepath.Join(configDir, "config.json")
	config := `{"enabled":true,"repositories":["owner/repo"],"workspace":"work","credential_env":"PRUI_CONTEXT_ACCOUNT","auth":"api_key"}`
	if err = os.WriteFile(filepath.Join(configDir, "issue-context.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRUI_CONTEXT_ACCOUNT", "synthetic-account-A")
	githubCalls, linearCalls := 0, 0
	failGithub := false
	malformedLinear := false
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
			return map[string]any{"nodes": nodes, "pageInfo": map[string]bool{"hasNextPage": false}}
		}
		pr := map[string]any{"headRefOid": strings.Repeat("c", 40), "body": "private-description https://linear.app/work/issue/APP-2/fix", "reviewDecision": "REVIEW_REQUIRED", "reviewRequests": connection([]any{map[string]any{"requestedReviewer": map[string]string{"__typename": "User", "login": "reviewer"}}}), "latestOpinionatedReviews": connection([]any{}), "labels": connection([]any{map[string]string{"name": "bug"}}), "closingIssuesReferences": connection([]any{map[string]any{"number": 3, "title": "GitHub issue", "state": "OPEN", "url": "https://github.com/owner/repo/issues/3", "repository": map[string]string{"nameWithOwner": "owner/repo"}}})}
		b, e := json.Marshal(map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": pr}}})
		return b, e
	})}
	a.issueContextTransport = contextTransport(func(r *http.Request) (*http.Response, error) {
		linearCalls++
		if r.Header.Get("Authorization") != "synthetic-account-A" || r.URL.String() != "https://api.linear.app/graphql" {
			t.Fatal("wrong authorized account/endpoint")
		}
		body := `{"data":{"issue":{"identifier":"APP-2","title":"Account A product","description":"Saved detail","url":"https://linear.app/work/issue/APP-2/fix","state":{"name":"Done"}}}}`
		if malformedLinear {
			body = `{"errors":[{"message":"private provider error"}],"data":{"issue":null}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	got, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || !got.Complete || len(got.Requested) != 1 || len(got.Issues) != 1 || len(got.Linear) != 1 || got.LinearWorkspace != "work" || got.LinearAuth != "api_key" || got.HeadSHA == frozen.Inventory.Comparison.Metadata.HeadSHA {
		t.Fatal("provider/cache integration", got, err)
	}
	// Changed credentials/configuration never relabel historical data on local open.
	t.Setenv("PRUI_CONTEXT_ACCOUNT", "synthetic-account-B")
	if err = os.WriteFile(filepath.Join(configDir, "issue-context.json"), []byte(`{"enabled":true,"repositories":["owner/repo"],"workspace":"other","credential_env":"PRUI_CONTEXT_ACCOUNT","auth":"oauth"}`), 0600); err != nil {
		t.Fatal(err)
	}
	a.offline = true
	cached, err := a.readIssueContext(context.Background(), id, false)
	if err != nil || cached.LinearWorkspace != "work" || cached.LinearAuth != "api_key" || cached.Linear[0].Title != "Account A product" || githubCalls != 1 || linearCalls != 1 {
		t.Fatal("historical provenance relabelled", cached, err)
	}
	for _, other := range []source.Identity{{Repository: "owner/repo", Number: id.Number + 1}, {Repository: "other/repo", Number: id.Number}} {
		if _, e := a.readIssueContext(context.Background(), other, false); !errors.Is(e, session.ErrIssueContextNotFound) {
			t.Fatal("foreign PR cache reused", other, e)
		}
	}
	if _, err = a.readIssueContext(context.Background(), id, true); err == nil || githubCalls != 1 || linearCalls != 1 {
		t.Fatal("offline touched providers")
	}
	// Reopen the database without either credential or configuration.
	if err = a.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.OpenReadOnly(a.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cached, err = reopened.LoadIssueContext(context.Background(), id)
	if err != nil || cached.LinearWorkspace != "work" || cached.Linear[0].Description != "Saved detail" {
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
	// Reopen writable for denied-account and optional partial refresh cases.
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
	if err != nil || retained.CapturedAt != got.CapturedAt || retained.LinearWorkspace != "work" {
		t.Fatal("failed refresh destroyed cache", err)
	}
	failGithub = false
	malformedLinear = true
	if err = os.WriteFile(filepath.Join(configDir, "issue-context.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRUI_CONTEXT_ACCOUNT", "synthetic-account-A")
	partial, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || len(partial.Issues) != 1 || len(partial.Labels) != 1 || len(partial.Linear) != 0 || partial.LinearReason == "" || strings.Contains(partial.LinearReason, "private provider") {
		t.Fatal("optional errors lost Github", partial, err)
	}
}
