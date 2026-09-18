package main

import (
	"context"
	"encoding/json"
	"testing"

	"pr-review/internal/guide"
	"pr-review/internal/guideeval"
)

func TestEvalGuidesCLIReadsStoredSessionWithoutGitHub(t *testing.T) {
	app, saved := wiringFixture(t)
	path := app.store.Path()
	if err := app.store.Close(); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int {
		return run([]string{"eval-guides", saved.ID, "--store", path})
	})
	if code != 0 {
		t.Fatalf("exit = %d, output = %q", code, out)
	}
	var report guideeval.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output is not JSON: %q: %v", out, err)
	}
	if report.Status != guideeval.NotAvailable {
		t.Fatalf("report = %#v, want not_available", report)
	}
}

func TestEvalGuidesCLIReportsStoredGeneratedGuides(t *testing.T) {
	app, saved := wiringFixture(t)
	derived, err := app.generateGuide(context.Background(), saved, guideAnalyzerFunc(func(_ context.Context, in guide.Input) (guide.Bundle, error) {
		ids := make([]string, 0, len(in.Units))
		for _, unit := range in.Units {
			ids = append(ids, unit.ID)
		}
		return guide.Bundle{Status: guide.Generated, Items: []guide.Item{{Title: "Fixture change", Sections: []guide.Section{{Title: "Review fixture", UnitIDs: ids}}}}}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	path := app.store.Path()
	if err := app.store.Close(); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int {
		return run([]string{"eval-guides", derived.ID, "--store", path})
	})
	if code != 0 {
		t.Fatalf("exit = %d, output = %q", code, out)
	}
	var report guideeval.Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != guideeval.Passed || len(report.Results) != 1 || report.Results[0].Name != "structural" {
		t.Fatalf("report = %#v", report)
	}
}
