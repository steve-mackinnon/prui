package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
	"time"
)

type contextGitHub struct {
	source.GitHub
	calls int
}

func (g *contextGitHub) ReadIssueContext(_ context.Context, id source.Identity) (source.IssueContext, error) {
	g.calls++
	return source.IssueContext{Identity: id, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now().UTC(), Complete: true, Body: "private-pr-body-marker https://linear.app/work/issue/APP-2/fix"}, nil
}

type contextTransport func(*http.Request) (*http.Response, error)

func (f contextTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestIssueContextOfflineAndConfiguredLinear(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := source.Identity{Repository: "o/r", Number: 1}
	gh := &contextGitHub{}
	configDir := t.TempDir()
	path := filepath.Join(configDir, "config.json")
	if err = os.WriteFile(filepath.Join(configDir, "issue-context.json"), []byte(`{"enabled":true,"repositories":["o/r"],"workspace":"work","credential_env":"PRUI_TEST_LINEAR_KEY","auth":"api_key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRUI_TEST_LINEAR_KEY", "fake-secret")
	calls := 0
	a := &application{gh: gh, store: store, guideConfigPath: path, issueContextTransport: contextTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Authorization") != "fake-secret" {
			t.Fatal("missing auth")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":{"issue":{"identifier":"APP-2","title":"Product context","description":"Details","url":"https://linear.app/work/issue/APP-2/fix","state":{"name":"Done"}}}}`))}, nil
	})}
	if _, err = a.readIssueContext(context.Background(), id, false); err == nil || gh.calls != 0 || calls != 0 {
		t.Fatal("cache open contacted provider")
	}
	got, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || len(got.Linear) != 1 || got.Linear[0].Title != "Product context" || got.Body != "" {
		t.Fatal(got, err)
	}
	a.offline = true
	if _, err = a.readIssueContext(context.Background(), id, true); err == nil || gh.calls != 1 || calls != 1 {
		t.Fatal("offline called provider")
	}
	if err = os.Remove(filepath.Join(configDir, "issue-context.json")); err != nil {
		t.Fatal(err)
	}
	cached, err := a.readIssueContext(context.Background(), id, false)
	if err != nil || len(cached.Linear) != 1 || calls != 1 || gh.calls != 1 {
		t.Fatal("offline context lost", err)
	}
	db, err := os.ReadFile(filepath.Join(store.Path(), "store.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(db), "fake-secret") || strings.Contains(string(db), "private-pr-body-marker") {
		t.Fatal("credential persisted")
	}
}

func TestIssueContextOptionalDeadlinePreservesGitHub(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(filepath.Join(dir, "issue-context.json"), []byte(`{"enabled":true,"repositories":["o/r"],"workspace":"work","credential_env":"PRUI_TEST_LINEAR_KEY","auth":"api_key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRUI_TEST_LINEAR_KEY", "fake-key")
	a := &application{gh: &contextGitHub{}, guideConfigPath: path, issueContextTransport: contextTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	got, err := a.readIssueContext(ctx, source.Identity{Repository: "o/r", Number: 1}, true)
	if err == nil || got.HeadSHA == "" || got.Body != "" || got.LinearReason == "" || !source.ValidateIssueContext(got) {
		t.Fatal("optional deadline discarded Github", got, err)
	}
}
