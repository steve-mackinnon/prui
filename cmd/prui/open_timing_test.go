package main

import (
	"context"
	"reflect"
	"testing"

	"prui/internal/commits"
	"prui/internal/verify"
)

func TestOpenTimingPreservesCommitCapture(t *testing.T) {
	for _, supportsCommits := range []bool{false, true} {
		name := "metadata-only"
		if supportsCommits {
			name = "commit-reader"
		}
		t.Run(name, func(t *testing.T) {
			app, seed := wiringFixture(t)
			if supportsCommits {
				app.gh = performanceGitHub{fixtureGH: app.gh.(*fixtureGH)}
			}
			metadata := seed.Inventory.Comparison.Metadata
			t.Setenv(verify.OpenTimingEnvironment, "")
			ordinary, err := app.openWithPrevious(context.Background(), string(seed.Checkout), metadata.Identity, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if supportsCommits && (ordinary.Commits == nil || ordinary.Commits.Status != commits.Captured) {
				t.Fatal("fixture did not capture commits")
			}
			t.Setenv(verify.OpenTimingEnvironment, "1")
			timed, err := app.openWithPrevious(context.Background(), string(seed.Checkout), metadata.Identity, nil, nil)
			if err != nil || !reflect.DeepEqual(ordinary.Commits, timed.Commits) {
				t.Fatalf("timing changed captured history: err=%v\nordinary=%#v\ntimed=%#v", err, ordinary.Commits, timed.Commits)
			}
		})
	}
}
