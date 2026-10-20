package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"prui/internal/commits"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
)

// Separate application work from process/package initialization and PTY I/O.
// Fixture creation, Git capture and initial schema creation are not timed.
func BenchmarkOfflineStartupComponents(b *testing.B) {
	for _, count := range []int{1, 1000} {
		b.Run(fmt.Sprintf("files-%d", count), func(b *testing.B) { benchmarkOfflineStartupComponents(b, count) })
	}
}

func benchmarkOfflineStartupComponents(b *testing.B, count int) {
	b.Helper()
	home := b.TempDir()
	b.Setenv("HOME", home)
	b.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY"} {
		b.Setenv(name, "")
	}
	repo := testutil.NewRepo(b)
	filePath := func(i int) string { return fmt.Sprintf("file-%05d.go", i) }
	for i := range count {
		repo.Write(filePath(i), "package fixture\nconst Value = 1\n")
	}
	base := repo.Commit()
	if count == 1 {
		repo.Write(filePath(0), "package fixture\nconst Value = 2\n")
	} else {
		for i := range count {
			if err := os.Chmod(filepath.Join(repo.Dir, filePath(i)), 0700); err != nil {
				b.Fatal(err)
			}
		}
	}
	head := repo.Commit()
	metadata := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	metadata.TargetBranch = "main"
	metadata.Description = "## Summary\n\nUpdate a **committed** constant.\n\n- Preserve source provenance.\n\n```go\nconst Value = 2\n```\n"
	root := filepath.Join(b.TempDir(), "storage")
	store, err := session.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	app := &application{store: store, gh: performanceGitHub{fixtureGH: &fixtureGH{value: metadata}}, runner: source.NewRunner(), limits: source.Defaults()}
	saved, err := app.openWithPrevious(context.Background(), repo.Dir, metadata.Identity, nil, nil)
	if err != nil {
		_ = store.Close()
		b.Fatal(err)
	}
	if !saved.Inventory.Complete || len(saved.Inventory.Files) != count {
		b.Fatal("incomplete startup fixture")
	}
	if saved.Commits == nil || saved.Commits.Status != commits.Captured || !saved.Commits.Complete || saved.Commits.Composition == nil || saved.Commits.Composition.Status != commits.Captured {
		b.Fatal("incomplete startup commit fixture")
	}
	wanted := fmt.Sprintf("0/%d read", count)
	if count > 1 {
		var reviewed []string
		for _, slice := range saved.Slices {
			reviewed = append(reviewed, slice.FileID)
		}
		if _, err := store.UpdateState(context.Background(), saved.ID, saved.Generation, saved.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: reviewed, RevisionStatus: session.Current}); err != nil {
			b.Fatal(err)
		}
		wanted = fmt.Sprintf("%d/%d read", count, count)
	}
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
	for _, readOnly := range []bool{false, true} {
		name := "open-writable"
		if readOnly {
			name = "open-readonly"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				var opened *session.Store
				var err error
				if readOnly {
					opened, err = session.OpenReadOnly(root)
				} else {
					opened, err = session.Open(root)
				}
				if err != nil {
					b.Fatal(err)
				}
				if err := opened.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	store, err = session.Open(root)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	b.Run("load-validate", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			record, err := store.Load(saved.ID)
			if err != nil || record == nil || !record.Inventory.Complete || len(record.Inventory.Files) != count {
				b.Fatalf("invalid offline snapshot: %v", err)
			}
		}
	})
	b.Run("load-model-first-frame", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkOfflineFrame(b, store, saved.ID, wanted)
		}
	})
}

func benchmarkOfflineFrame(b *testing.B, store *session.Store, id, wanted string) {
	b.Helper()
	app := &application{store: store, offline: true}
	m := app.model(context.Background(), options{Command: "resume", SessionID: id, Offline: true})
	defer m.Close()
	_, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	cmd := m.Init()
	for m.Loading {
		if cmd == nil {
			b.Fatal("offline model stopped before loading")
		}
		_, cmd = m.Update(cmd())
	}
	if m.Err != nil || m.Session == nil || !strings.Contains(m.View().Content, wanted) {
		b.Fatalf("offline frame unavailable: %v", m.Err)
	}
}
