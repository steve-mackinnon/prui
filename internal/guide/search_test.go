package guide

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	reviewcontext "pr-review/internal/context"
	"strings"
	"testing"
	"time"
)

func TestSearchLoopReservesFinalAndReplaysReasoning(t *testing.T) {
	n := 0
	r := newRecorder(t, func(w http.ResponseWriter, body []byte) {
		n++
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req["store"] != false || req["previous_response_id"] != nil || req["max_output_tokens"] == nil {
			t.Error("unsafe request contract")
		}
		if n > 1 && !strings.Contains(string(body), "encrypted-state") {
			t.Error("lost reasoning continuation")
		}
		if n == 4 {
			if req["tool_choice"] != "none" {
				t.Error("final request allows tools")
			}
			answer(w, oneGuide("u1"))
			return
		}
		fmt.Fprintf(w, `{"status":"completed","output":[{"type":"reasoning","id":"r%d","summary":[],"encrypted_content":"encrypted-state"},{"type":"function_call","name":"search","call_id":"c%d","arguments":"{\"pattern\":\"Login\",\"path_glob\":\"\",\"revision\":\"head\"}"}]}`, n, n)
	})
	in := Input{Search: &reviewcontext.SearchCorpus{}}
	b, err := r.analyzer(t, "").Analyze(context.Background(), in)
	if err != nil || b.Status != Generated || n != 4 {
		t.Fatalf("loop result %v %v requests=%d", b, err, n)
	}
}

func TestSearchLoopDoesNotPersistProviderEcho(t *testing.T) {
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		w.WriteHeader(400)
		fmt.Fprint(w, `{"error":{"message":"SENSITIVE_SOURCE"}}`)
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: &reviewcontext.SearchCorpus{}})
	if err != nil || b.Status != Unavailable || strings.Contains(b.Reason, "SENSITIVE_SOURCE") {
		t.Fatalf("unsafe failure %v %v", b, err)
	}
}

func searchFixture() *reviewcontext.SearchCorpus {
	return &reviewcontext.SearchCorpus{Files: []reviewcontext.SearchFile{{Revision: "head", Evidence: reviewcontext.Evidence{Path: []byte("main.go"), CommitSHA: strings.Repeat("a", 40), BlobID: strings.Repeat("b", 40), Excerpt: []byte("package main\nfunc Login() {}\n")}}}}
}
func emitCall(w http.ResponseWriter, name, args string) {
	b, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "function_call", "call_id": "c1", "name": name, "arguments": args}}})
	w.Write(b)
}
func TestSearchLoopUploadsOnlyRetrievedEvidence(t *testing.T) {
	n := 0
	r := newRecorder(t, func(w http.ResponseWriter, body []byte) {
		n++
		if n == 1 {
			if strings.Contains(string(body), "package main") {
				t.Error("corpus uploaded wholesale")
			}
			emitCall(w, "search", `{"pattern":"Login","path_glob":"","revision":"head"}`)
			return
		}
		if !strings.Contains(string(body), "func Login()") {
			t.Error("retrieved text missing")
		}
		answer(w, oneGuide("u1"))
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
	if err != nil || b.Status != Generated || len(b.UploadedEvidence) != 1 || b.RetrievalVersion != SearchVersion {
		t.Fatalf("missing evidence %v %v", b, err)
	}
}
func TestSearchLoopRejectsMalformedToolsWithoutSource(t *testing.T) {
	for _, tc := range []struct{ name, args string }{
		{"shell", `{"command":"cat main.go"}`},
		{"search", `{"pattern":"Login","path_glob":"","revision":"head","extra":true}`},
		{"search", `{"pattern":"Login","revision":"head"}`},
		{"search", `{"pattern":"Login","path_glob":null,"revision":"head"}`},
		{"search", `{"pattern":"wrong","pattern":"Login","path_glob":"","revision":"head"}`},
		{"search", `{"pattern":"Login","path_glob":"","revision":"head"} {}`},
		{"read_lines", `{"path":"main.go","start":0,"end":1,"revision":"head"}`},
	} {
		t.Run(tc.name+tc.args, func(t *testing.T) {
			n := 0
			r := newRecorder(t, func(w http.ResponseWriter, body []byte) {
				n++
				if n == 1 {
					emitCall(w, tc.name, tc.args)
					return
				}
				if strings.Contains(string(body), "func Login()") {
					t.Error("invalid tool returned source")
				}
				answer(w, oneGuide("u1"))
			})
			b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
			if err != nil || b.Status != Generated || len(b.UploadedEvidence) != 0 {
				t.Fatalf("invalid call %v %v", b, err)
			}
		})
	}
}
func TestSearchLoopRechecksPrivacy(t *testing.T) {
	c := searchFixture()
	c.Files[0].Evidence.Excerpt = []byte("password = synthetic\n")
	n := 0
	r := newRecorder(t, func(w http.ResponseWriter, body []byte) {
		n++
		if n == 1 {
			emitCall(w, "read_lines", `{"path":"main.go","start":1,"end":1,"revision":"head"}`)
			return
		}
		if strings.Contains(string(body), "synthetic") {
			t.Error("sensitive tool result uploaded")
		}
		answer(w, oneGuide("u1"))
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: c})
	if err != nil || len(b.UploadedEvidence) != 0 {
		t.Fatalf("privacy failure %v %v", b, err)
	}
}
func TestSearchLoopPreservesSentEvidenceOnProviderFailure(t *testing.T) {
	n := 0
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		n++
		if n == 1 {
			emitCall(w, "search", `{"pattern":"Login","path_glob":"","revision":"head"}`)
			return
		}
		w.WriteHeader(500)
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
	if err != nil || b.Status != Unavailable || len(b.UploadedEvidence) != 1 || n != 2 {
		t.Fatalf("failure lost upload record %v %v n=%d", b, err, n)
	}
}

func TestSearchLoopDoesNotRecordUnsentEvidence(t *testing.T) {
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		b, _ := json.Marshal(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "reasoning", "encrypted_content": strings.Repeat("x", maxRequestBytes)}, map[string]any{"type": "function_call", "call_id": "c1", "name": "search", "arguments": `{"pattern":"Login","path_glob":"","revision":"head"}`}}})
		w.Write(b)
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
	if err != nil || b.Status != Unavailable || len(b.UploadedEvidence) != 0 || len(r.requests) != 1 {
		t.Fatalf("unsent evidence recorded %v %v", b, err)
	}
}
func TestSearchLoopRejectsExcessiveToolBatch(t *testing.T) {
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		var calls []any
		for i := 0; i < 9; i++ {
			calls = append(calls, map[string]any{"type": "function_call", "call_id": fmt.Sprint(i), "name": "search", "arguments": `{"pattern":"Login","path_glob":"","revision":"head"}`})
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": calls})
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
	if err != nil || b.Status != Unavailable || len(b.UploadedEvidence) != 0 || len(r.requests) != 1 {
		t.Fatalf("batch not bounded %v %v", b, err)
	}
}
func TestSearchLoopDeadlineRetainsSentEvidence(t *testing.T) {
	n := 0
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		n++
		if n == 1 {
			emitCall(w, "search", `{"pattern":"Login","path_glob":"","revision":"head"}`)
			return
		}
		time.Sleep(100 * time.Millisecond)
		answer(w, oneGuide("u1"))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	b, err := r.analyzer(t, "").Analyze(ctx, Input{Search: searchFixture()})
	if err != nil || b.Status != Unavailable || len(b.UploadedEvidence) != 1 {
		t.Fatalf("deadline lost uploaded evidence %v %v", b, err)
	}
}

func TestSearchLoopRefusalNeverContinuesTools(t *testing.T) {
	r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
		json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "refusal", "refusal": "no"}}}, map[string]any{"type": "function_call", "call_id": "c1", "name": "search", "arguments": `{"pattern":"Login","path_glob":"","revision":"head"}`}}})
	})
	b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: searchFixture()})
	if err != nil || b.Status != Unavailable || len(r.requests) != 1 || len(b.UploadedEvidence) != 0 {
		t.Fatalf("continued after refusal: %v %v requests=%d", b, err, len(r.requests))
	}
}

func TestSearchLoopRetainsRetrievalIncompleteness(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		corpusIncomplete, truncate, providerFailure bool
	}{
		{name: "complete"}, {name: "corpus", corpusIncomplete: true}, {name: "tool", truncate: true}, {name: "failed tool", truncate: true, providerFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := searchFixture()
			c.Incomplete = tc.corpusIncomplete
			if tc.truncate {
				c.Files[0].Evidence.Excerpt = []byte(strings.Repeat("Login\n", 25))
			}
			n := 0
			r := newRecorder(t, func(w http.ResponseWriter, _ []byte) {
				n++
				if n == 1 {
					emitCall(w, "search", `{"pattern":"Login","path_glob":"","revision":"head"}`)
					return
				}
				if tc.providerFailure {
					w.WriteHeader(500)
					return
				}
				answer(w, oneGuide("u1"))
			})
			b, err := r.analyzer(t, "").Analyze(context.Background(), Input{Search: c})
			want := tc.corpusIncomplete || tc.truncate || tc.providerFailure
			if err != nil || b.RetrievalIncomplete != want {
				t.Fatalf("incomplete=%v want=%v error=%v", b.RetrievalIncomplete, want, err)
			}
			payload, _ := json.Marshal(b)
			var restored Bundle
			if json.Unmarshal(payload, &restored) != nil || restored.RetrievalIncomplete != want {
				t.Fatal("scope warning did not survive serialization")
			}
		})
	}
}
