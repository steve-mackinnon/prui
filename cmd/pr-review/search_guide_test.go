package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"pr-review/internal/guide"
	"reflect"
	"strings"
	"testing"
)

func TestPreparedGuideUsesSavedPinsAndNoCredentialsBeforeConsent(t *testing.T) {
	app, original := wiringFixture(t)
	app.newAnalyzer = func() (guide.Analyzer, error) { t.Fatal("preparation accessed provider"); return nil, nil }
	if err := os.WriteFile(filepath.Join(string(original.Checkout), "a"), []byte("dirty unrelated source"), 0600); err != nil {
		t.Fatal(err)
	}
	prep, err := app.prepareGuide(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if prep.Input.Search == nil {
		t.Fatal("pinned corpus unavailable", prep.UnavailableReason)
	}
	for _, f := range prep.Files() {
		if strings.Contains(string(f.Evidence.Excerpt), "dirty") {
			t.Fatal("working source leaked")
		}
	}
	if _, err := app.requestPreparedGuide(context.Background(), original, prep); err == nil {
		t.Fatal("unconfirmed request accepted")
	}
}

func TestPreparedGuideOfflineBeforeAnySourceOrCredentials(t *testing.T) {
	app := &application{offline: true, newAnalyzer: func() (guide.Analyzer, error) { panic("credentials") }}
	if _, err := app.prepareGuide(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
	if _, err := app.requestPreparedGuide(context.Background(), nil, nil); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
}

func TestPreparedGuideMissingCheckoutOffersSavedEvidence(t *testing.T) {
	app, original := wiringFixture(t)
	original.Checkout = []byte(filepath.Join(t.TempDir(), "missing"))
	prep, err := app.prepareGuide(context.Background(), original)
	if err != nil || prep.Input.Search != nil || prep.UnavailableReason == "" {
		t.Fatal(prep, err)
	}
	if len(prep.Input.Units) == 0 {
		t.Fatal("saved source lost")
	}
}

func TestPreparedGuidePersistsOnlySentSearchEvidenceAndPreservesSource(t *testing.T) {
	app, original := wiringFixture(t)
	originalBytes, _ := json.Marshal(original.Snapshot)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["store"] != false {
			t.Error("request may store provider state")
		}
		if requests == 1 {
			_, _ = io.WriteString(w, `{"status":"completed","output":[{"type":"function_call","call_id":"read1","name":"read_lines","arguments":"{\"path\":\"a\",\"start\":1,\"end\":1,\"revision\":\"head\"}"}]}`)
			return
		}
		wire, _ := json.Marshal(body)
		if !bytes.Contains(wire, []byte("new\\n")) {
			t.Error("pinned result missing from continuation")
		}
		ids := []string{}
		for _, u := range original.Inventory.Units {
			ids = append(ids, u.ID)
		}
		structured, _ := json.Marshal(map[string]any{"guides": []any{map[string]any{"title": "Updated behavior", "description": "Change the fixture.", "sections": []any{map[string]any{"title": "Implementation", "description": "Read the change.", "unit_ids": ids}}}}})
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "completed", "output_text": string(structured)})
	}))
	defer server.Close()
	app.newAnalyzer = func() (guide.Analyzer, error) {
		return guide.NewOpenAI(guide.OpenAIOptions{APIKey: "fake-key", Endpoint: server.URL})
	}
	prep, err := app.prepareGuide(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	prep.Confirm()
	derived, err := app.requestPreparedGuide(context.Background(), original, prep)
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || derived.Guides.Status != guide.Generated {
		t.Fatal("search loop incomplete", requests, derived.Guides)
	}
	if len(derived.Guides.RetrievedEvidence) != 1 || string(derived.Guides.RetrievedEvidence[0].Excerpt) != "new\n" {
		t.Fatal("sent evidence not retained", derived.Guides.RetrievedEvidence)
	}
	if len(derived.Guides.EvidenceIDs) != 1 || derived.Guides.InputDigest == "" || derived.Guides.RetrievalVersion != guide.SearchVersion {
		t.Fatal("missing provenance")
	}
	if derived.ID == original.ID || derived.DerivedFrom != original.ID || len(derived.ReviewedSliceIDs) != 0 {
		t.Fatal("immutable derivation lost")
	}
	reloaded, err := app.store.Load(derived.ID)
	if err != nil || len(reloaded.Guides.RetrievedEvidence) != 1 {
		t.Fatal("ledger not durable", err)
	}
	after, _ := json.Marshal(original.Snapshot)
	if !bytes.Equal(originalBytes, after) {
		t.Fatal("original changed")
	}
	if !reflect.DeepEqual(derived.Context, original.Context) {
		t.Fatal("frozen source evidence changed")
	}
}
