package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

type fixtureGH struct {
	value source.Metadata
	err   error
}

func (g *fixtureGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.value, g.err
}
func (g *fixtureGH) Token(context.Context) (string, error) { panic("credentials must not be used") }

// captureStdout records what a CLI run writes to stdout; plain output must stay
// free of terminal control bytes even while the interactive view is colored.
func captureStdout(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	code := f()
	os.Stdout = saved
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	out := <-done
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return out, code
}

func TestLifecycleOpenResumeNewAndOfflineCLI(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	g := &fixtureGH{value: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}}
	path := filepath.Join(t.TempDir(), "sessions")
	store, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := application{store: store, gh: g, runner: source.NewRunner(), limits: source.Defaults()}
	saved, err := app.load(context.Background(), options{Command: "open", Checkout: r.Dir, Identity: g.value.Identity}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || saved.RevisionStatus != session.Current {
		t.Fatal("open did not persist/check")
	}
	if err := review.Mark(store, saved, saved.Slices[0].FileID, true); err != nil {
		t.Fatal(err)
	}
	r.Write("a", "third\n")
	g.value.HeadSHA = r.Commit()
	resumed, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID}, func(string) {})
	if err != nil || resumed.RevisionStatus != session.Stale {
		t.Fatal("resume did not check", err)
	}
	fresh, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID, New: true}, func(string) {})
	if err != nil || fresh.ID == saved.ID || len(fresh.ReviewedSliceIDs) != 0 {
		t.Fatal("new comparison reused progress", err)
	}
	old, err := store.Load(saved.ID)
	if err != nil || len(old.ReviewedSliceIDs) != 1 {
		t.Fatal("old session destroyed", err)
	}
	g.err = errors.New("authentication unavailable")
	failed, err := app.load(context.Background(), options{Command: "resume", SessionID: saved.ID}, func(string) {})
	if err != nil || failed.RevisionStatus != session.CheckFailed {
		t.Fatal("offline failed to resume", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(r.Dir); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"sessions", "--store", path},
		{"resume", saved.ID, "--store", path, "--offline", "--plain"},
		{"delete", saved.ID, "--store", path},
	} {
		out, code := captureStdout(t, func() int { return run(args) })
		if code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if strings.ContainsAny(out, "\x1b\a") {
			t.Fatalf("%v: terminal control bytes in plain output", args)
		}
	}
	if _, err := os.Stat(filepath.Join(path, saved.ID)); !os.IsNotExist(err) {
		t.Fatal("CLI deletion incomplete")
	}
}

// provider is a recording OpenAI endpoint. It answers from the unit ids it was
// actually sent, so a passing case also proves the prompt carried them.
type provider struct {
	calls  atomic.Int64
	status atomic.Int64
}

var promptUnit = regexp.MustCompile(`unit (\S+) \| path `)

func (p *provider) start(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if code := int(p.status.Load()); code != 0 {
			w.WriteHeader(code)
			w.Write([]byte(`{"error":{"message":"provider unavailable"}}`))
			return
		}
		var sent struct {
			Input []struct{ Content string } `json:"input"`
		}
		if err := json.Unmarshal(body, &sent); err != nil || len(sent.Input) != 1 {
			t.Error("request is not one bounded user message", err)
			return
		}
		var ids []string
		for _, m := range promptUnit.FindAllStringSubmatch(sent.Input[0].Content, -1) {
			ids = append(ids, `"`+m[1]+`"`)
		}
		fmt.Fprintf(w, `{"status":"completed","output_text":"{\"guides\":[{\"title\":\"Rewrite a\",\"description\":\"Replaces the file contents.\",\"sections\":[{\"title\":\"Replace contents\",\"description\":\"One hunk.\",\"unit_ids\":[%s]}]}]}"}`,
			strings.ReplaceAll(strings.Join(ids, ","), `"`, `\"`))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestLifecycleAnalysisOptInFailureAndResume(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	g := &fixtureGH{value: source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}}
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := &provider{}
	endpoint := p.start(t)
	analyzer, err := guide.NewOpenAI(guide.OpenAIOptions{APIKey: "sk-test", Endpoint: endpoint.URL})
	if err != nil {
		t.Fatal(err)
	}
	app := application{store: store, gh: g, runner: source.NewRunner(), limits: source.Defaults()}
	open := options{Command: "open", Checkout: r.Dir, Identity: g.value.Identity}
	quiet := func(string) {}

	optedOut, err := app.load(context.Background(), open, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != 0 {
		t.Fatal("opting out still contacted the provider", p.calls.Load())
	}
	if optedOut.Guides == nil || optedOut.Guides.Status != guide.Unavailable || optedOut.Guides.Reason != "analysis not requested" {
		t.Fatal("opt-out is not stated as a decision", optedOut.Guides)
	}

	app.analyzer = analyzer
	p.status.Store(500)
	var notices []string
	broken, err := app.load(context.Background(), open, func(s string) { notices = append(notices, s) })
	if err != nil {
		t.Fatal("analysis failure broke open", err)
	}
	if broken.ID == "" || !broken.Inventory.Complete {
		t.Fatal("a failed analysis cost the raw review its session")
	}
	if broken.Guides.Status != guide.Unavailable || !strings.Contains(broken.Guides.Reason, "openai analysis failed") {
		t.Fatal("provider failure is not stated", broken.Guides)
	}
	if !strings.Contains(strings.Join(notices, "\n"), "Guide analysis unavailable") {
		t.Fatal("analysis failure was silent", notices)
	}

	p.status.Store(0)
	analyzed, err := app.load(context.Background(), open, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if analyzed.Guides.Status != guide.Generated || analyzed.Guides.Provider != "openai" || analyzed.Guides.Model != guide.DefaultModel {
		t.Fatal("guides did not reach the snapshot with their provenance", analyzed.Guides)
	}
	if len(analyzed.Guides.Items) != 1 || analyzed.Guides.Items[0].Title != "Rewrite a" {
		t.Fatal("stored guides are not the provider's grouping", analyzed.Guides.Items)
	}
	if err := guide.Validate(*analyzed.Guides, analyzed.Inventory); err != nil {
		t.Fatal("stored bundle does not validate", err)
	}
	// Analysis must not disturb the deterministic plan it interprets.
	if len(analyzed.Slices) != len(optedOut.Slices) || len(analyzed.UnitFiles) != len(optedOut.UnitFiles) {
		t.Fatal("analysis changed file ownership")
	}

	before := p.calls.Load()
	resumed, err := app.load(context.Background(), options{Command: "resume", SessionID: analyzed.ID}, quiet)
	if err != nil {
		t.Fatal(err)
	}
	if p.calls.Load() != before {
		t.Fatal("resume contacted the provider", p.calls.Load()-before)
	}
	if resumed.Guides == nil || resumed.Guides.Status != guide.Generated || len(resumed.Guides.Items) != 1 {
		t.Fatal("resume did not render the frozen guides", resumed.Guides)
	}
}
