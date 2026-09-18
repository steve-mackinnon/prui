package verify

import (
	"context"
	"encoding/json"
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
		Runs:        2,
		Python:      python,
		Timeout:     3 * time.Second,
		Environment: append(os.Environ(), "TERM=dumb", "PR_REVIEW_EXPECT_CWD="+root),
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
	if len(journey.Screens) != 6 || !strings.Contains(journey.Screens["run-1-resumed"], "[x] fake.go") || !strings.Contains(journey.Screens["run-2-resumed"], "[x] fake.go") {
		t.Fatalf("screens = %#v", journey.Screens)
	}
	if journey.Report.Artifacts.Transcript != "terminal.txt" || len(journey.Report.Artifacts.Screens) != 6 {
		t.Fatalf("artifacts = %#v", journey.Report.Artifacts)
	}
	if got, want := strings.Join(journey.Report.Artifacts.Screens, ","), "screens/run-1-initial.svg,screens/run-1-marked.svg,screens/run-1-resumed.svg,screens/run-2-initial.svg,screens/run-2-marked.svg,screens/run-2-resumed.svg"; got != want {
		t.Fatalf("screen artifact order = %q, want %q", got, want)
	}
	if journey.Report.Timing == nil || journey.Report.Timing.RunCount != 2 || journey.Report.Timing.Runs[0].Cache != ColdCache || journey.Report.Timing.Runs[1].Cache != WarmCache {
		t.Fatalf("timing = %#v", journey.Report.Timing)
	}
	if journey.Report.Timing.Runs[0].Network.GitHubMetadata <= 0 || journey.Report.Timing.Runs[0].Network.PinAndInventory <= 0 || journey.Report.Timing.Runs[0].Local.ProcessStart <= 0 || journey.Report.Timing.Runs[0].Local.FirstReviewFrame <= journey.Report.Timing.Runs[0].Local.ProcessStart {
		t.Fatalf("journey did not record observed timings: %#v", journey.Report.Timing.Runs[0])
	}
	warm := journey.Report.Timing.Runs[1].Network
	if warm.GitHubMetadata != 0 || warm.PinAndInventory != 0 || len(warm.Stages) != 0 {
		t.Fatalf("warm run repeated network work: %#v", warm)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "terminal.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestRunOpenUsesItsConfiguredTimeoutInsteadOfThePTYTimeout(t *testing.T) {
	root := t.TempDir()
	fixture, err := os.ReadFile(filepath.Join("testdata", "fake_pr_review.py"))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "fake-pr-review")
	if err := os.WriteFile(binary, fixture, 0700); err != nil {
		t.Fatal(err)
	}

	opened, err := runOpen(context.Background(), JourneyConfig{
		Executable:  binary,
		PRURL:       "https://github.com/owner/repo/pull/42",
		Checkout:    root,
		StoreDir:    root,
		Timeout:     10 * time.Millisecond,
		OpenTimeout: 200 * time.Millisecond,
		Environment: append(os.Environ(), "PR_REVIEW_FAKE_OPEN_DELAY_SECONDS=0.05", "PR_REVIEW_EXPECT_CWD="+root),
	})
	if err != nil {
		t.Fatalf("runOpen returned error with its configured timeout: %v", err)
	}
	if sessionIDInPlain.FindStringSubmatch(string(opened)) == nil {
		t.Fatalf("open output did not include a saved session: %q", opened)
	}
}

func TestRunJourneyReportsSanitizedOpenFailureStageAndElapsedTime(t *testing.T) {
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
		OpenTimeout: 10 * time.Millisecond,
		Environment: append(os.Environ(), "PR_REVIEW_FAKE_OPEN_DELAY_SECONDS=0.05", "PR_REVIEW_EXPECT_CWD="+root),
	})
	if err == nil {
		t.Fatal("RunJourney succeeded despite the open timeout")
	}
	if journey.Report.Failure == nil || journey.Report.Failure.Stage != "open" || journey.Report.Failure.ElapsedMS <= 0 {
		t.Fatalf("failure diagnostics = %#v", journey.Report.Failure)
	}
	if got := journey.Report.Reason; got != "could not open pull request" {
		t.Fatalf("reason = %q", got)
	}
	reportJSON, marshalErr := json.Marshal(journey.Report)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !strings.Contains(string(reportJSON), `"failure":{"stage":"open","elapsed_ms":`) || strings.Contains(string(reportJSON), "PR_REVIEW_FAKE_OPEN_DELAY_SECONDS") {
		t.Fatalf("report JSON did not contain sanitized diagnostics: %s", reportJSON)
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
