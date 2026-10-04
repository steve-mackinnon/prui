package issuecontext

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestLinearAuthorizedRead(t *testing.T) {
	for _, mode := range []string{"api_key", "oauth"} {
		calls := 0
		c := Linear{Config: Config{Enabled: true, Repositories: []string{"o/r"}, Workspace: "work", CredentialEnv: "TEST_KEY", Auth: mode}, Getenv: func(name string) string {
			if name != "TEST_KEY" {
				t.Fatal(name)
			}
			return "fake-key"
		}, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			want := "fake-key"
			if mode == "oauth" {
				want = "Bearer fake-key"
			}
			if r.URL.String() != "https://api.linear.app/graphql" || r.Header.Get("Authorization") != want || r.Method != "POST" {
				t.Fatal("unsafe request")
			}
			b, _ := io.ReadAll(r.Body)
			var p struct {
				Query     string
				Variables map[string]string
			}
			if json.Unmarshal(b, &p) != nil || p.Variables["id"] != "APP-2" || strings.Contains(p.Query, "mutation") || strings.Contains(string(b), "fake-key") {
				t.Fatal("bad payload")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"issue":{"identifier":"APP-2","title":"Fix","description":"Details","url":"https://linear.app/work/issue/APP-2/fix","state":{"name":"In Progress"}}}}`)), Header: http.Header{}}, nil
		})}
		got, reason := c.Read(context.Background(), "o/r", "https://linear.app/work/issue/APP-2/fix")
		if reason != "" || calls != 1 || len(got) != 1 || got[0].Status != "In Progress" || got[0].Description != "Details" {
			t.Fatalf("bad result %#v %s %d", got, reason, calls)
		}
	}
}
func TestLinearCandidatesAndAuthorization(t *testing.T) {
	body := "https://evil.test/work/issue/APP-1 https://linear.app.evil/work/issue/APP-2 https://user@linear.app/work/issue/APP-3 https://linear.app/other/issue/APP-4 https://linear.app/work/issue/APP-5?token=x https://linear.app/work/issue/APP-6/fix https://linear.app/work/issue/APP-6/fix"
	got, more := Candidates(body, "work")
	if more || len(got) != 1 || got[0] != "APP-6" {
		t.Fatalf("unsafe candidates %#v %v", got, more)
	}
	c := Linear{Config: Config{Enabled: true, Repositories: []string{"other/repo"}, Workspace: "work", CredentialEnv: "TEST_KEY", Auth: "api_key"}, Getenv: func(string) string { t.Fatal("unauthorized credential read"); return "" }}
	if _, reason := c.Read(context.Background(), "o/r", body); !strings.Contains(reason, "not authorized") {
		t.Fatal(reason)
	}
}
func TestLinearFailuresAreGeneric(t *testing.T) {
	for _, response := range []string{`{"errors":[{"message":"fake-key"}],"data":{"issue":null}}`, `{"data":{"issue":{"identifier":"APP-9"}}}`, `{"data":{"issue":{"identifier":"APP-2","title":"Fix","description":"Details","url":"https://evil.test/","state":{"name":"Done"}}}}`, strings.Repeat("x", (1<<20)+1)} {
		c := Linear{Config: Config{Enabled: true, Repositories: []string{"o/r"}, Workspace: "work", CredentialEnv: "TEST_KEY", Auth: "api_key"}, Getenv: func(string) string { return "fake-key" }, Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})}
		got, reason := c.Read(context.Background(), "o/r", "https://linear.app/work/issue/APP-2/fix")
		if len(got) != 0 || reason == "" || strings.Contains(reason, "fake-key") {
			t.Fatal("invalid response accepted or exposed", reason)
		}
	}
}

func TestLinearCancellationCandidateLimitAndRedirect(t *testing.T) {
	body := ""
	for i := 1; i <= 11; i++ {
		body += fmt.Sprintf("https://linear.app/work/issue/APP-%d/fix ", i)
	}
	ids, more := Candidates(body, "work")
	if len(ids) != 10 || !more {
		t.Fatal("candidate bound lost")
	}
	config := Config{Enabled: true, Repositories: []string{"o/r"}, Workspace: "work", CredentialEnv: "TEST", Auth: "api_key"}
	c := Linear{Config: config, Getenv: func(string) string { t.Fatal("canceled operation read credential"); return "" }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, reason := c.Read(ctx, "o/r", body); !strings.Contains(reason, "canceled") {
		t.Fatal(reason)
	}
	calls := 0
	c.Getenv = func(string) string { return "fake-key" }
	c.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://evil.test/"}}}, nil
	})
	if got, reason := c.Read(context.Background(), "o/r", "https://linear.app/work/issue/APP-1/fix"); len(got) != 0 || reason == "" || calls != 1 {
		t.Fatal("redirect followed", got, reason, calls)
	}
}

func TestLinearPartialMalformedOptionalRecord(t *testing.T) {
	c := Linear{Config: Config{Enabled: true, Repositories: []string{"o/r"}, Workspace: "work", CredentialEnv: "TEST", Auth: "api_key"}, Getenv: func(string) string { return "fake-key" }, Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		state := "Done"
		id := "APP-1"
		if strings.Contains(string(b), "APP-2") {
			state = "bad\x1b"
			id = "APP-2"
		}
		data, _ := json.Marshal(map[string]any{"data": map[string]any{"issue": map[string]any{"identifier": id, "title": "Fix", "description": "Details", "url": "https://linear.app/work/issue/" + id + "/fix", "state": map[string]any{"name": state}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	got, reason := c.Read(context.Background(), "o/r", "https://linear.app/work/issue/APP-1/fix https://linear.app/work/issue/APP-2/fix")
	if len(got) != 1 || got[0].Identifier != "APP-1" || reason == "" {
		t.Fatal("optional malformed record contaminated context", got, reason)
	}
}
