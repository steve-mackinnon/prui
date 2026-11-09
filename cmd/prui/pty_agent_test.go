package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"prui/internal/agentchat"
	reviewcontext "prui/internal/context"
	"prui/internal/guide"
	"prui/internal/inventory"
	"prui/internal/session"
	"prui/internal/source"
)

// TestPTYAgentConversations exercises the compiled app through a real PTY and
// a loopback model. The fixture supplies only saved comparison data: no GitHub
// access, checkout reads, provider credential, or external network is needed.
func TestPTYAgentConversations(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("PTY agent journey supports Linux and macOS")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("PTY agent journey requires Python 3 standard library:", err)
	}
	root := t.TempDir()
	buildHome := os.Getenv("HOME")
	storePath := temporaryDefaultStore(t, root)
	store, err := session.Open(storePath)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(ptyAgentSnapshot())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	requests := map[string]int{}
	var requestError string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		manifest, err := ptyAgentManifest(r)
		if err != nil {
			mu.Lock()
			requestError = "provider request did not contain a valid bounded manifest"
			mu.Unlock()
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		kind := ptyAgentQuestionKind(manifest.Question)
		mu.Lock()
		requests[kind]++
		if !ptyAgentManifestValid(manifest, kind) {
			requestError = "provider request lost selected source or captured evidence"
		}
		mu.Unlock()
		if kind == "slow" {
			<-r.Context().Done()
			// The PTY must observe this marker before quitting; waiting for a
			// permanently silent fake would not prove that X canceled the call.
			if err := os.WriteFile(filepath.Join(root, "provider-canceled"), []byte("yes"), 0600); err != nil {
				mu.Lock()
				requestError = "provider cancellation marker could not be written"
				mu.Unlock()
			}
			ptyAgentWriteAnswer(w, agentchat.Answer{Text: "Late slow answer", References: []agentchat.Reference{}})
			return
		}
		answer := agentchat.Answer{Text: map[string]string{
			"hunk":   "Hunk answer from selected code.",
			"test":   "Captured test evidence supports target.",
			"caller": "Caller evidence was not supplied.",
			"follow": "Follow-up uses the anchored hunk.",
			"guide":  "Guide reopens the same anchored thread.",
		}[kind], References: []agentchat.Reference{}}
		switch kind {
		case "hunk", "follow", "guide":
			answer.References = []agentchat.Reference{{EvidenceID: manifest.Hunk.ID, StartLine: 1, EndLine: 1}}
		case "test":
			for _, e := range manifest.Evidence {
				if e.ID == "test-evidence" {
					answer.References = []agentchat.Reference{{EvidenceID: e.ID, StartLine: 1, EndLine: 1}}
				}
			}
		}
		ptyAgentWriteAnswer(w, answer)
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal("loopback test listener unavailable:", err)
	}
	server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: handler}}
	server.Start()
	defer server.Close()

	configDir := filepath.Join(root, "prui")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	config, err := json.Marshal(map[string]any{"guide": map[string]string{
		"provider": "openai-compatible", "model": "pty-agent", "base_url": server.URL + "/v1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "prui")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, ".")
	build.Env = append(os.Environ(), "HOME="+buildHome)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build PTY executable: %v\n%s", err, out)
	}
	cmd := exec.CommandContext(ctx, python, "testdata/pty_agent.py", binary, root, saved.ID)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		mu.Lock()
		counts, providerFailure := fmt.Sprint(requests), requestError
		mu.Unlock()
		t.Fatalf("agent PTY journey: %v; request kinds=%s; provider check=%s\n%s", err, counts, providerFailure, out)
	} else {
		t.Logf("%s", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if requestError != "" {
		t.Fatal(requestError)
	}
	for _, kind := range []string{"hunk", "test", "caller", "follow", "guide", "slow"} {
		if requests[kind] != 1 {
			t.Fatalf("%s request count = %d, want one", kind, requests[kind])
		}
	}
	if len(requests) != 6 {
		t.Fatalf("unexpected agent request kind; total kinds = %d, want six", len(requests))
	}
}

func ptyAgentSnapshot() session.Snapshot {
	const head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const base = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const sourceText = "@@ -0,0 +1,2 @@\n+func target() bool { return true }\n+// target has a captured test\n"
	patch := []byte(sourceText)
	ref := fmt.Sprintf("%x", sha256.Sum256(patch))
	file := inventory.FileChange{ID: "pkg/work.go", NewPath: []byte("pkg/work.go"), Status: "A", NewMode: "100644"}
	return session.Snapshot{
		Inventory: inventory.Inventory{
			Comparison: source.PinnedComparison{InventoryID: "pty-agent-inventory", MergeBaseSHA: base, Metadata: source.Metadata{
				Identity: source.Identity{Repository: "owner/agent", Number: 42}, BaseRepository: "owner/agent", HeadRepository: "owner/agent", BaseSHA: base, HeadSHA: head,
			}},
			Complete: true,
			Files:    []inventory.FileChange{file},
			Units: []inventory.ReviewUnit{
				{InventoryID: "pty-agent-inventory", ID: "metadata", FileChangeID: file.ID, Kind: inventory.FileMetadata},
				{InventoryID: "pty-agent-inventory", ID: "hunk", FileChangeID: file.ID, Kind: inventory.TextHunk, PatchReference: ref, NewRange: inventory.Range{Start: 1, Count: 2}},
			},
			Patches: map[string][]byte{ref: patch},
		},
		Slices:    []session.Slice{{FileID: file.ID, Units: []int{0, 1}}},
		UnitFiles: []int{0, 0},
		Context: reviewcontext.ContextBundle{ComparisonID: "pty-agent-inventory", Evidence: []reviewcontext.Evidence{{
			EvidenceID: "test-evidence", Repository: "owner/agent", CommitSHA: head,
			BlobID: "cccccccccccccccccccccccccccccccccccccccc", Path: []byte("pkg/work_test.go"),
			LineStart: 1, LineEnd: 1, Kind: reviewcontext.NearbyTest,
			Excerpt: []byte("func TestTarget() { assert(target()) }\n"),
		}}},
		Guides: &guide.Bundle{Status: guide.Generated, Provider: "openai-compatible", Model: "pty-agent", PromptVersion: guide.PromptVersion, SchemaName: guide.SchemaName,
			Items: []guide.Item{{Title: "Review source", Sections: []guide.Section{{Title: "Change", UnitIDs: []string{"metadata", "hunk"}}}}}},
	}
}

func ptyAgentManifest(r *http.Request) (agentchat.Manifest, error) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "" {
		return agentchat.Manifest{}, fmt.Errorf("unexpected provider operation")
	}
	limited, err := io.ReadAll(io.LimitReader(r.Body, 2<<20+1))
	if err != nil || len(limited) > 2<<20 {
		return agentchat.Manifest{}, fmt.Errorf("provider body exceeds transport bound")
	}
	var body any
	if json.Unmarshal(limited, &body) != nil {
		return agentchat.Manifest{}, fmt.Errorf("invalid provider body")
	}
	const marker = "Frozen request manifest (JSON data):\n"
	var manifest agentchat.Manifest
	for _, text := range ptyAgentStrings(body) {
		if _, after, ok := strings.Cut(text, marker); ok {
			if len(after) <= agentchat.MaxRequestBytes && json.Unmarshal([]byte(after), &manifest) == nil {
				return manifest, nil
			}
		}
	}
	return agentchat.Manifest{}, fmt.Errorf("manifest absent")
}

func ptyAgentStrings(v any) []string {
	switch value := v.(type) {
	case string:
		return []string{value}
	case []any:
		var out []string
		for _, item := range value {
			out = append(out, ptyAgentStrings(item)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, item := range value {
			out = append(out, ptyAgentStrings(item)...)
		}
		return out
	default:
		return nil
	}
}

func ptyAgentQuestionKind(question string) string {
	switch {
	case strings.Contains(question, "HUNK"):
		return "hunk"
	case strings.Contains(question, "TEST"):
		return "test"
	case strings.Contains(question, "CALLER"):
		return "caller"
	case strings.Contains(question, "FOLLOW"):
		return "follow"
	case strings.Contains(question, "GUIDE"):
		return "guide"
	case strings.Contains(question, "SLOW"):
		return "slow"
	default:
		return "unknown"
	}
}

func ptyAgentManifestValid(m agentchat.Manifest, kind string) bool {
	if m.ComparisonID != "pty-agent-inventory" || m.Anchor.Path != "pkg/work.go" || m.Anchor.Side != "RIGHT" || m.Anchor.Line != 1 || m.Hunk.ID == "" || m.Hunk.Path != "pkg/work.go" {
		return false
	}
	selected := false
	for _, line := range m.Hunk.Lines {
		selected = selected || line.Line == 1 && line.Selected && line.Text == "func target() bool { return true }"
	}
	if !selected || len(m.Evidence) > 3 || len(m.Question) > agentchat.MaxQuestionBytes {
		return false
	}
	wantHistory := map[string]int{"hunk": 0, "test": 1, "caller": 2, "follow": 3, "guide": 4, "slow": 5}
	if len(m.History) != wantHistory[kind] {
		return false
	}
	if kind == "test" {
		for _, e := range m.Evidence {
			if e.ID == "test-evidence" && e.Path == "pkg/work_test.go" && strings.Contains(e.Text, "TestTarget") {
				return true
			}
		}
		return false
	}
	return true
}

func ptyAgentWriteAnswer(w http.ResponseWriter, answer agentchat.Answer) {
	arguments, _ := json.Marshal(answer)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "pty-agent", "object": "chat.completion", "model": "pty-agent",
		"choices": []any{map[string]any{"index": 0, "finish_reason": "tool_calls", "message": map[string]any{
			"role": "assistant", "content": nil, "tool_calls": []any{map[string]any{
				"id": "call-agent", "type": "function", "function": map[string]any{
					"name": guide.AgentAnswerSchema, "arguments": string(arguments),
				},
			}},
		}}},
		"usage": map[string]any{"prompt_tokens": 20, "completion_tokens": 20, "total_tokens": 40},
	})
}
