package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"prui/internal/source"
	"strings"
	"testing"
	"time"
)

func TestIssueContextIndependentOfflineCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	id := frozen.Inventory.Comparison.Metadata.Identity
	c := source.IssueContext{Identity: id, HeadSHA: strings.Repeat("f", 40), CapturedAt: time.Now().UTC(), Complete: true, ReviewDecision: "APPROVED"}
	if err = s.SaveIssueContext(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	old := c
	old.CapturedAt = old.CapturedAt.Add(-time.Hour)
	old.ReviewDecision = "CHANGES_REQUESTED"
	if err = s.SaveIssueContext(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	r, err := s.Load(frozen.ID)
	if err != nil || r.SnapshotReference != frozen.SnapshotReference || r.Generation != frozen.Generation {
		t.Fatal("context changed frozen session", err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.LoadIssueContext(context.Background(), id)
	if err != nil || got.ReviewDecision != "APPROVED" || got.HeadSHA != c.HeadSHA {
		t.Fatal("offline or stale writer lost context", err, got)
	}
	if err = s.SaveIssueContext(context.Background(), c); err == nil {
		t.Fatal("readonly wrote")
	}
}
func TestIssueContextMissingAndCorrupt(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := source.Identity{Repository: "o/r", Number: 1}
	if _, err = s.LoadIssueContext(context.Background(), id); err != ErrIssueContextNotFound {
		t.Fatal(err)
	}
	c := source.IssueContext{Identity: id, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now().UTC(), Complete: true}
	if err = s.SaveIssueContext(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	if _, err = db.Exec(`UPDATE issue_context SET digest=?`, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LoadIssueContext(context.Background(), id); err == nil {
		t.Fatal("corruption accepted")
	}
}

func TestIssueContextLegacyLinearFieldsIgnored(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := source.IssueContext{Identity: source.Identity{Repository: "o/r", Number: 7}, HeadSHA: strings.Repeat("a", 40), CapturedAt: time.Now(), Complete: true, Labels: []string{"bug"}}
	if err = s.SaveIssueContext(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err = json.Unmarshal(payload, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["LinearWorkspace"], legacy["LinearAuth"] = "work", "api_key"
	legacy["Linear"] = []source.ContextIssue{{Identifier: "APP-1", Title: "old ticket", Status: "Done", URL: "https://linear.app/work/issue/APP-1/fix"}}
	payload, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE issue_context SET payload=?, digest=?`, payload, fmt.Sprintf("%x", sha256.Sum256(payload))); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadIssueContext(context.Background(), c.Identity)
	if err != nil || len(got.Labels) != 1 || got.Labels[0] != "bug" {
		t.Fatal("legacy cache lost GitHub context", got, err)
	}
	payload, err = json.Marshal(got)
	if err != nil || strings.Contains(string(payload), "Linear") {
		t.Fatal("legacy integration retained", string(payload), err)
	}
}
