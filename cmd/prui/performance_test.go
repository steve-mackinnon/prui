package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

type performanceGitHub struct{ *fixtureGH }

type performanceRunner struct {
	source.Runner
	calls map[string]int
}

func (r *performanceRunner) Run(ctx context.Context, request source.Request) ([]byte, error) {
	if len(request.Args) > 2 {
		r.calls[request.Args[2]]++
	}
	return r.Runner.Run(ctx, request)
}

func (g performanceGitHub) ListPullRequestCommits(context.Context, source.Identity) ([]source.PullRequestCommit, error) {
	return []source.PullRequestCommit{{SHA: g.value.HeadSHA, Subject: "synthetic change", Author: "fixture"}}, nil
}

// Includes pinning, inventory, commit capture, evidence, durable storage and
// freshness checks. GitHub responses are synthetic; network time is excluded.
func BenchmarkPullRequestOpen(b *testing.B) {
	for _, scenario := range []string{"text-100", "unique-text-100", "mode-1000-tree-10000"} {
		modeOnly := scenario == "mode-1000-tree-10000"
		name, changed, total := scenario, 100, 1000
		if modeOnly {
			name, changed, total = "mode-1000-tree-10000", 1000, 10000
		}
		b.Run(name, func(b *testing.B) {
			repo := testutil.NewRepo(b)
			for i := 0; i < total; i++ {
				content := "package fixture\nconst Value = 1\n"
				if scenario == "unique-text-100" {
					content = fmt.Sprintf("package fixture\nconst Value%d = 1\n", i)
				}
				repo.Write(fmt.Sprintf("pkg/%04d/file.go", i), content)
			}
			base := repo.Commit()
			for i := 0; i < changed; i++ {
				path := fmt.Sprintf("pkg/%04d/file.go", i)
				if modeOnly {
					if err := os.Chmod(filepath.Join(repo.Dir, path), 0700); err != nil {
						b.Fatal(err)
					}
				} else {
					content := "package fixture\nconst Value = 2\n"
					if scenario == "unique-text-100" {
						content = fmt.Sprintf("package fixture\nconst Value%d = 2\n", i)
					}
					repo.Write(path, content)
				}
			}
			head := repo.Commit()
			meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
			store, err := session.Open(filepath.Join(b.TempDir(), "storage"))
			if err != nil {
				b.Fatal(err)
			}
			defer store.Close()
			runner := &performanceRunner{Runner: source.NewRunner(), calls: map[string]int{}}
			app := &application{store: store, gh: performanceGitHub{&fixtureGH{value: meta}}, runner: runner, limits: source.Defaults()}
			b.Run("cold", func(b *testing.B) {
				before := map[string]int{}
				for command, count := range runner.calls {
					before[command] = count
				}
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					s, err := app.openWithPrevious(context.Background(), repo.Dir, meta.Identity, nil, nil)
					if err != nil || s == nil || !s.Inventory.Complete || len(s.Inventory.Files) != changed {
						b.Fatalf("open: %v", err)
					}
				}
				for _, command := range []string{"cat-file", "diff", "ls-tree"} {
					b.ReportMetric(float64(runner.calls[command]-before[command])/float64(b.N), command+"-calls/op")
				}
			})
			b.Run("cached", func(b *testing.B) {
				b.StopTimer()
				if exists, err := store.HasComparisonSnapshot(meta.Identity); err != nil {
					b.Fatal(err)
				} else if !exists {
					if _, err := app.openWithPrevious(context.Background(), repo.Dir, meta.Identity, nil, nil); err != nil {
						b.Fatal(err)
					}
				}
				b.StartTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					s, err := app.cachedPullRequestSnapshot(context.Background(), repo.Dir, meta.Identity, nil)
					if err != nil || s == nil || len(s.Inventory.Files) != changed || s.RevisionStatus != session.Unchecked {
						b.Fatalf("cached open: %v", err)
					}
				}
			})
		})
	}
}
