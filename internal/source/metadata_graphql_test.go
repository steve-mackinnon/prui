package source

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func graphqlMetadataFixture(t *testing.T) string {
	t.Helper()
	payload := map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{
		"number": 42, "body": "Summary\n\n- committed source", "baseRefName": "release/next", "baseRefOid": strings.Repeat("a", 40), "headRefOid": strings.Repeat("b", 40), "baseRepository": map[string]any{"nameWithOwner": "owner/repo"}, "headRepository": map[string]any{"nameWithOwner": "fork/repo"},
	}}}}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGraphQLMetadataFixedQueryAndFreshCredentials(t *testing.T) {
	calls, requests := 0, 0
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		calls++
		if strings.Join(q.Args, " ") != "auth token --hostname github.com" {
			t.Fatal("unexpected CLI operation")
		}
		return []byte(strings.Repeat("x", calls)), nil
	})}
	g.readClient = &http.Client{Transport: readRoundTripper(func(q *http.Request) (*http.Response, error) {
		requests++
		if q.Method != "POST" || q.URL.String() != "https://api.github.com/graphql" || q.Header.Get("Content-Type") != "application/json" || q.Header.Get("Authorization") != "Bearer "+strings.Repeat("x", requests) {
			t.Fatal("incorrect metadata request")
		}
		var payload struct {
			Query     string
			Variables struct {
				Owner, Repo string
				Number      int
			}
		}
		if err := json.NewDecoder(q.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Variables.Owner != "owner" || payload.Variables.Repo != "repo" || payload.Variables.Number != 42 {
			t.Fatal("wrong metadata identity")
		}
		for _, field := range []string{"baseRefName", "baseRefOid", "headRefOid", "baseRepository", "headRepository", "nameWithOwner", "body"} {
			if !strings.Contains(payload.Query, field) {
				t.Fatalf("missing %s", field)
			}
		}
		if strings.Contains(payload.Query, "mutation") || strings.Contains(payload.Query, "owner/repo") {
			t.Fatal("query is not fixed read-only metadata")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(graphqlMetadataFixture(t)))}, nil
	})}
	want := Metadata{Identity: Identity{"owner/repo", 42}, TargetBranch: "release/next", BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), Description: "Summary\n\n- committed source"}
	for range 2 {
		got, err := g.Metadata(context.Background(), want.Identity)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata differs: %v", err)
		}
	}
	if calls != 2 || requests != 2 {
		t.Fatal("metadata or credential reused")
	}
}

func TestGraphQLMetadataRejectsIncompleteResponses(t *testing.T) {
	for _, field := range []string{"number", "body", "baseRefName", "baseRefOid", "headRefOid", "baseRepository", "headRepository"} {
		for _, null := range []bool{false, true} {
			t.Run(field+map[bool]string{false: " missing", true: " null"}[null], func(t *testing.T) {
				var payload map[string]any
				if err := json.Unmarshal([]byte(graphqlMetadataFixture(t)), &payload); err != nil {
					t.Fatal(err)
				}
				pr := payload["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)
				if null {
					pr[field] = nil
				} else {
					delete(pr, field)
				}
				data, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				assertGraphQLMetadataRejected(t, string(data))
			})
		}
	}
	for _, response := range []string{`{"data":null}`, `{"data":{"repository":null}}`, `{"data":{"repository":{"pullRequest":null}}}`, `{"errors":[{"message":"secret provider detail"}]}`, strings.Replace(graphqlMetadataFixture(t), `"data":`, `"errors":[{"message":"partial"}],"data":`, 1), strings.Replace(graphqlMetadataFixture(t), strings.Repeat("a", 40), strings.Repeat("A", 40), 1), strings.Replace(graphqlMetadataFixture(t), `"owner/repo"`, `"other/repo"`, 1), strings.Replace(graphqlMetadataFixture(t), `"number":42`, `"number":43`, 1), string([]byte{0xff})} {
		assertGraphQLMetadataRejected(t, response)
	}
}

func assertGraphQLMetadataRejected(t *testing.T, response string) {
	t.Helper()
	calls := 0
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		calls++
		if q.Args[0] != "auth" {
			t.Fatal("fallback after response")
		}
		return []byte("token"), nil
	}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
	})}}
	if _, err := g.Metadata(context.Background(), Identity{"owner/repo", 42}); err == nil || strings.Contains(err.Error(), "secret provider detail") {
		t.Fatal("invalid or unsafe response accepted")
	}
	if calls != 1 {
		t.Fatal("unexpected retry")
	}
}

func TestGraphQLMetadataAuthenticationFallbackRetainsREST(t *testing.T) {
	calls := 0
	rest := `{"number":42,"body":null,"base":{"sha":"` + strings.Repeat("a", 40) + `","ref":"main","repo":{"full_name":"owner/repo"}},"head":{"sha":"` + strings.Repeat("b", 40) + `","repo":{"full_name":"fork/repo"}}}`
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		calls++
		if q.Args[0] == "auth" {
			return nil, ErrCommand
		}
		if strings.Join(q.Args, " ") != "api --hostname github.com --method GET repos/owner/repo/pulls/42" || len(q.Stdin) != 0 {
			t.Fatal("changed CLI compatibility request")
		}
		return []byte(rest), nil
	}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP after unavailable token export")
		return nil, ErrCommand
	})}}
	got, err := g.Metadata(context.Background(), Identity{"owner/repo", 42})
	if err != nil || got.HeadRepository != "fork/repo" || got.Description != "" || calls != 2 {
		t.Fatalf("compatibility read failed: %v", err)
	}
}

func TestGraphQLMetadataTransportBoundsAndNoRetry(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body          string
		failure, want error
	}{
		{"permission", 403, "secret provider detail", nil, ErrCommand},
		{"oversized", 200, strings.Repeat("x", (1<<20)+1), nil, ErrLimit},
		{"transport", 0, "", errors.New("secret transport detail"), ErrCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, requests := 0, 0
			g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
				calls++
				if q.Args[0] != "auth" {
					t.Fatal("CLI retry after dispatch")
				}
				return []byte("token"), nil
			}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
				requests++
				if tc.failure != nil {
					return nil, tc.failure
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			_, err := g.Metadata(context.Background(), Identity{"owner/repo", 42})
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "secret") || calls != 1 || requests != 1 {
				t.Fatalf("unexpected failure: %v, CLI %d HTTP %d", err, calls, requests)
			}
		})
	}
}

func TestGraphQLMetadataCancellationAndAuthenticationLimits(t *testing.T) {
	for _, failure := range []error{ErrLimit, context.Canceled, context.DeadlineExceeded} {
		calls := 0
		g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
			calls++
			if q.Args[0] != "auth" {
				t.Fatal("fallback after authentication limit")
			}
			return nil, failure
		}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
			t.Fatal("HTTP after authentication limit")
			return nil, ErrCommand
		})}}
		if _, err := g.Metadata(context.Background(), Identity{"owner/repo", 42}); !errors.Is(err, failure) || calls != 1 {
			t.Fatalf("authentication error = %v, calls %d", err, calls)
		}
	}
	g := &GH{Limits: Defaults(), Runner: readRunner(func(context.Context, Request) ([]byte, error) {
		t.Fatal("credentials after cancellation")
		return nil, nil
	}), readClient: newGitHubReadClient()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Metadata(ctx, Identity{"owner/repo", 42}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := g.Metadata(context.Background(), Identity{"../repo", 42}); err == nil {
		t.Fatal("invalid identity accepted")
	}
}

func TestGraphQLMetadataDeadlineIncludesAuthentication(t *testing.T) {
	g := &GH{Limits: Defaults(), Runner: readRunner(func(ctx context.Context, _ Request) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }), readClient: newGitHubReadClient()}
	g.Limits.Operation = time.Millisecond
	if _, err := g.Metadata(context.Background(), Identity{"owner/repo", 42}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestGraphQLMetadataInterruptedBodyClosesWithoutRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &interruptedReadBody{cancel: cancel}
	calls := 0
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		calls++
		if q.Args[0] != "auth" {
			t.Fatal("retry after interrupted response")
		}
		return []byte("token"), nil
	}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) { return &http.Response{StatusCode: 200, Body: body}, nil })}}
	if _, err := g.Metadata(ctx, Identity{"owner/repo", 42}); !errors.Is(err, context.Canceled) || !body.closed || calls != 1 {
		t.Fatalf("interrupted response = %v, closed %v, calls %d", err, body.closed, calls)
	}
}
