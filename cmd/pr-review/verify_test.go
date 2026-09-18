package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pr-review/internal/verify"
)

func TestWriteVerifyReportAddsItsRelativeArtifactPath(t *testing.T) {
	dir := t.TempDir()
	r := verify.NewReport()
	r.Status = verify.Passed
	if err := writeVerifyReport(dir, &r); err != nil {
		t.Fatal(err)
	}
	if r.Artifacts.Report != "report.json" {
		t.Fatalf("report artifact = %q", r.Artifacts.Report)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got verify.Report
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != verify.Passed || got.Artifacts.Report != "report.json" {
		t.Fatalf("stored report = %#v", got)
	}
}
