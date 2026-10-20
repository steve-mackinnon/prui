package main

import (
	"context"
	"encoding/json"
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
	return source.IssueContext{Identity: id, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now().UTC(), Complete: true, Labels: []string{"bug"}}, nil
}

type contextTransport func(*http.Request) (*http.Response, error)

func (f contextTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestIssueContextIgnoresRemovedLinearConfiguration(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id := source.Identity{Repository: "o/r", Number: 1}
	gh := &contextGitHub{}
	configDir := t.TempDir()
	if err = os.WriteFile(filepath.Join(configDir, "issue-context.json"), []byte(`{"enabled":true,"repositories":["o/r"],"workspace":"work","credential_env":"PRUI_TEST_LINEAR_KEY","auth":"api_key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRUI_TEST_LINEAR_KEY", "fake-secret")
	original := http.DefaultTransport
	http.DefaultTransport = contextTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request to removed integration: %s", r.URL)
		return nil, nil
	})
	defer func() { http.DefaultTransport = original }()
	a := &application{gh: gh, store: store, guideConfigPath: filepath.Join(configDir, "config.json")}
	if _, err = a.readIssueContext(context.Background(), id, false); err == nil || gh.calls != 0 {
		t.Fatal("cache open contacted provider")
	}
	got, err := a.readIssueContext(context.Background(), id, true)
	if err != nil || len(got.Labels) != 1 {
		t.Fatal(got, err)
	}
	payload, err := json.Marshal(got)
	if err != nil || strings.Contains(string(payload), "Linear") {
		t.Fatal("removed integration persisted", string(payload), err)
	}
	a.offline = true
	if _, err = a.readIssueContext(context.Background(), id, true); err == nil || gh.calls != 1 {
		t.Fatal("offline called provider")
	}
	cached, err := a.readIssueContext(context.Background(), id, false)
	if err != nil || len(cached.Labels) != 1 || gh.calls != 1 {
		t.Fatal("offline context lost", err)
	}
}
