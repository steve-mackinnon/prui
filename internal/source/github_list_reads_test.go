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

const githubListReadFixture = `{"data":{"repository":{"pullRequests":{"nodes":[{"number":42,"title":"Change","body":"Summary\nDetails","baseRefName":"main","createdAt":"2026-09-23T12:00:00Z","author":{"login":"alice"},"viewerLatestReview":{"state":"COMMENTED"},"commits":{"nodes":[{"commit":{"author":{"name":"Bob","user":{"login":"bob"}},"statusCheckRollup":{"state":"SUCCESS"}}}]}}]}}}}`

func TestListSharedReadUsesSameQueryAndFreshCredentials(t *testing.T) {
	var legacyQuery string
	legacy := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		if len(q.Args) != 10 || q.Args[3] != "graphql" || len(q.Stdin) != 0 {
			t.Fatal("changed legacy list request")
		}
		legacyQuery = strings.TrimPrefix(q.Args[5], "query=")
		return []byte(githubListReadFixture), nil
	})}
	want, err := legacy.ListPullRequests(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	credentials, requests := 0, 0
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
		credentials++
		if strings.Join(q.Args, " ") != "auth token --hostname github.com" {
			t.Fatal("unexpected CLI list request")
		}
		return []byte(strings.Repeat("x", credentials)), nil
	})}
	g.readClient = &http.Client{Transport: readRoundTripper(func(q *http.Request) (*http.Response, error) {
		requests++
		if q.Method != http.MethodPost || q.URL.String() != "https://api.github.com/graphql" || q.Header.Get("Content-Type") != "application/json" || q.Header.Get("Authorization") != "Bearer "+strings.Repeat("x", requests) {
			t.Fatal("incorrect list request")
		}
		var payload struct {
			Query     string
			Variables map[string]string
		}
		if err := json.NewDecoder(q.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Query != legacyQuery || !reflect.DeepEqual(payload.Variables, map[string]string{"owner": "owner", "name": "repo"}) {
			t.Fatal("list query or variables changed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(githubListReadFixture))}, nil
	})}
	for range 2 {
		got, err := g.ListPullRequests(context.Background(), "owner/repo")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("normalized list differs from CLI", err)
		}
	}
	if credentials != 2 || requests != 2 {
		t.Fatal("list credentials or response cached")
	}
}

func TestListSharedReadFailuresDoNotRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		transport  error
	}{
		{"transport", "", 0, errors.New("private provider detail")},
		{"status", "private provider detail", 403, nil},
		{"limit", strings.Repeat("x", (1<<20)+1), 200, nil},
		{"graphql", `{"errors":[{"message":"private provider detail"}]}`, 200, nil},
		{"partial", strings.Replace(githubListReadFixture, `"data":`, `"errors":[{"message":"private provider detail"}],"data":`, 1), 200, nil},
		{"invalid text", "\xff", 200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			credentials, requests := 0, 0
			g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
				credentials++
				if q.Args[0] != "auth" {
					t.Fatal("list retried through CLI")
				}
				return []byte("token"), nil
			}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
				requests++
				if tc.transport != nil {
					return nil, tc.transport
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}}
			_, err := g.ListPullRequests(context.Background(), "owner/repo")
			if err == nil || strings.Contains(err.Error(), "private provider detail") || credentials != 1 || requests != 1 {
				t.Fatal("unsafe failure or retry", err)
			}
			if tc.name == "limit" && !errors.Is(err, ErrLimit) {
				t.Fatal("list lost resource-limit classification")
			}
		})
	}
}

func TestListSharedReadAuthenticationCompatibility(t *testing.T) {
	for _, failure := range []error{ErrCommand, ErrLimit, context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			calls := 0
			g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, q Request) ([]byte, error) {
				calls++
				if q.Args[0] == "auth" {
					return nil, failure
				}
				if len(q.Args) != 10 || q.Args[3] != "graphql" || !reflect.DeepEqual(q.Args[6:], []string{"-F", "owner=owner", "-F", "name=repo"}) || len(q.Stdin) != 0 {
					t.Fatal("changed list compatibility request")
				}
				return []byte(githubListReadFixture), nil
			}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
				t.Fatal("HTTP after token-export failure")
				return nil, ErrCommand
			})}}
			got, err := g.ListPullRequests(context.Background(), "owner/repo")
			if failure == ErrCommand {
				if err != nil || len(got) != 1 || got[0].Identity.Number != 42 || calls != 2 {
					t.Fatal("list compatibility path failed", err)
				}
			} else if !errors.Is(err, failure) || calls != 1 {
				t.Fatal("resource/cancellation failure retried", err)
			}
		})
	}
}

func TestListSharedReadRefusesInvalidIdentityAndCanceledContext(t *testing.T) {
	g := &GH{Limits: Defaults(), Runner: readRunner(func(context.Context, Request) ([]byte, error) {
		t.Fatal("CLI before identity/cancellation check")
		return nil, nil
	}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP before identity/cancellation check")
		return nil, ErrCommand
	})}}
	if _, err := g.ListPullRequests(context.Background(), "owner/repo/extra"); err == nil {
		t.Fatal("invalid repository accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.ListPullRequests(ctx, "owner/repo"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestListSharedReadDeadlineIncludesAuthentication(t *testing.T) {
	g := &GH{Limits: Defaults(), Runner: readRunner(func(ctx context.Context, q Request) ([]byte, error) {
		if q.Args[0] != "auth" {
			t.Fatal("list fallback after authentication deadline")
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP after authentication deadline")
		return nil, ErrCommand
	})}}
	g.Limits.Operation = time.Millisecond
	if _, err := g.ListPullRequests(context.Background(), "owner/repo"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
