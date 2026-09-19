package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"pr-review/internal/tui"
	"pr-review/internal/verify"
)

func runVerify(ctx context.Context, o options) int {
	if o.Storage != "" {
		fmt.Fprintln(os.Stderr, "verify creates its own private temporary session store; --store is not accepted")
		return 1
	}
	if err := outsideCheckout(o.Artifacts, o.Checkout); err != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(err.Error()))
		return 1
	}
	if err := os.Mkdir(o.Artifacts, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "artifact directory must be a new path:", tui.Escape(err.Error()))
		return 1
	}
	store, err := os.MkdirTemp("", "pr-review-verify-")
	if err != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(err.Error()))
		return 1
	}
	defer func() { _ = os.RemoveAll(store) }()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(err.Error()))
		return 1
	}
	journey, journeyErr := verify.RunJourney(ctx, verify.JourneyConfig{
		Executable: executable, PRURL: o.Identity.URL(), Checkout: o.Checkout,
		StoreDir: store, ArtifactDir: o.Artifacts, Runs: o.MeasureRuns,
		OpenTimeout: o.OpenTimeout,
	})
	report := journey.Report
	if report.Status == verify.NotRun {
		report.Status, report.Reason = verify.Failed, "verification did not start"
	}
	if err := writeVerifyReport(o.Artifacts, &report); err != nil {
		fmt.Fprintln(os.Stderr, "could not write verification report:", tui.Escape(err.Error()))
		return 1
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		return 1
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return 130
	}
	if journeyErr != nil || report.Status != verify.Passed {
		return 1
	}
	return 0
}

func writeVerifyReport(dir string, report *verify.Report) error {
	if report == nil {
		return errors.New("verification report is required")
	}
	report.Artifacts.Report = "report.json"
	b, err := json.Marshal(report)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, report.Artifacts.Report)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
