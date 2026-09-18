package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunJourneyOpensThenMarksAndResumesWithBoundedArtifacts(t *testing.T) {
	python := requirePython(t)
	root := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("testdata", "fake_pr_review.py"))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake-pr-review")
	if err := os.WriteFile(binary, fixture, 0700); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "store")
	artifacts := filepath.Join(root, "artifacts")
	if err := os.Mkdir(store, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(artifacts, 0700); err != nil {
		t.Fatal(err)
	}

	journey, err := RunJourney(context.Background(), JourneyConfig{
		Executable:  binary,
		PRURL:       "https://github.com/owner/repo/pull/42",
		Checkout:    root,
		StoreDir:    store,
		ArtifactDir: artifacts,
		Runs:        1,
		Python:      python,
		Timeout:     3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if journey.Report.Status != Passed || journey.Report.SessionID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("report = %#v", journey.Report)
	}
	for _, check := range journey.Report.Checks {
		if check.Status != Passed {
			t.Fatalf("check = %#v", check)
		}
	}
	if !strings.Contains(string(journey.Transcript), "1/2 read (local)") {
		t.Fatalf("transcript did not capture marking: %q", journey.Transcript)
	}
	if len(journey.Screens) != 3 || !strings.Contains(journey.Screens["run-1-resumed"], "[x] fake.go") {
		t.Fatalf("screens = %#v", journey.Screens)
	}
	if journey.Report.Artifacts.Transcript != "terminal.txt" || len(journey.Report.Artifacts.Screens) != 3 {
		t.Fatalf("artifacts = %#v", journey.Report.Artifacts)
	}
	if got, want := strings.Join(journey.Report.Artifacts.Screens, ","), "screens/run-1-initial.svg,screens/run-1-marked.svg,screens/run-1-resumed.svg"; got != want {
		t.Fatalf("screen artifact order = %q, want %q", got, want)
	}
	if journey.Report.Timing == nil || journey.Report.Timing.RunCount != 1 || journey.Report.Timing.Runs[0].Cache != ColdCache {
		t.Fatalf("timing = %#v", journey.Report.Timing)
	}
	if journey.Report.Timing.Runs[0].Network.PinAndInventory <= 0 || journey.Report.Timing.Runs[0].Local.FirstReviewFrame <= 0 {
		t.Fatalf("journey did not record observed timings: %#v", journey.Report.Timing.Runs[0])
	}
	if _, err := os.Stat(filepath.Join(artifacts, "terminal.txt")); err != nil {
		t.Fatal(err)
	}
}

func requirePython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("Python 3 is required for standard-library PTY tests")
	return ""
}
