package review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"prui/internal/incremental"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/testutil"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestIncrementalPinnedPushRewriteBaseAndOffline(t *testing.T) {
	for _, mode := range []string{"push", "forcepush", "base unchanged proof", "base unrelated content", "base changed content", "rename", "delete"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			r := testutil.NewRepo(t)
			r.Write("stable", "before\n")
			r.Write("changed", "before\n")
			base := r.Commit()
			r.Write("stable", "after\n")
			r.Write("changed", "after\n")
			head := r.Commit()
			meta := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
			store, err := session.Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			raw, err := Open(ctx, r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
			if err != nil {
				t.Fatal(err)
			}
			old, err := store.Create(raw.Snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range old.Slices {
				if err := Mark(ctx, store, old, s.FileID, true); err != nil {
					t.Fatal(err)
				}
			}
			next := meta
			switch mode {
			case "forcepush":
				r.Git("checkout", "--detach", base)
				r.Write("stable", "after\n")
				r.Write("changed", "rewrite\n")
				next.HeadSHA = r.Commit()
			case "base unchanged proof":
				next.BaseSHA = r.Commit() // unchanged head tree, but merge base now equals head: no PR changes
			case "base unrelated content":
				r.Git("checkout", "--detach", base)
				r.Write("unrelated", "base only\n")
				newBase := r.Commit()
				r.Git("checkout", "--detach", head)
				r.Git("merge", "--no-commit", newBase)
				next.HeadSHA = r.Commit()
				next.BaseSHA = newBase
			case "base changed content":
				r.Git("checkout", "--detach", base)
				r.Write("stable", "new base\n")
				newBase := r.Commit()
				r.Git("checkout", "--detach", head)
				r.Git("merge", "--no-commit", "--strategy=ours", newBase)
				next.HeadSHA = r.Commit()
				next.BaseSHA = newBase
			case "rename":
				r.Git("mv", "stable", "renamed")
				next.HeadSHA = r.Commit()
			case "delete":
				if err := os.Remove(filepath.Join(r.Dir, "stable")); err != nil {
					t.Fatal(err)
				}
				next.HeadSHA = r.Commit()
			default:
				r.Write("changed", "again\n")
				next.HeadSHA = r.Commit()
			}
			// Dirty checkout content must never enter either inventory or the head diff.
			r.Write("stable", "dirty checkout SECRET\n")
			before := r.Snapshot()
			fresh, err := OpenWithConfig(ctx, r.Dir, meta.Identity, FixtureGitHub{next}, source.NewRunner(), source.Defaults(), nil, Config{Previous: old})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, r.Snapshot()) {
				t.Fatal("review mutated checkout")
			}
			b := fresh.Incremental
			if b == nil || b.Status != "captured" || !b.Diff.Complete || b.Previous.HeadSHA != head || b.Current.HeadSHA != next.HeadSHA || b.PreviousID != old.ID || b.PreviousReference != old.SnapshotReference {
				t.Fatalf("capture %#v", b)
			}
			if mode == "forcepush" && !strings.HasPrefix(b.Relation, "rewritten") {
				t.Fatal(b.Relation)
			}
			for _, patch := range b.Diff.Patches {
				if strings.Contains(string(patch), "SECRET") {
					t.Fatal("checkout evidence leaked")
				}
			}
			// Every changed or renamed stable file remains unread. Normal/force pushes
			// carry the stable file only, even when its session-specific unit IDs change.
			var carried []string
			for _, f := range fresh.Inventory.Files {
				if slices.Contains(fresh.ReviewedSliceIDs, f.ID) {
					carried = append(carried, string(f.NewPath))
				}
			}
			var want []string
			switch mode {
			case "push", "forcepush":
				want = []string{"stable"}
			case "base unrelated content":
				want = []string{"changed", "stable"}
			case "base changed content", "rename", "delete":
				want = []string{"changed"}
			}
			slices.Sort(carried)
			if !slices.Equal(carried, want) {
				t.Fatalf("carried %v want %v", carried, want)
			}
			saved, err := store.CreateWithProgress(fresh.Snapshot, fresh.ReviewedSliceIDs)
			if err != nil {
				t.Fatal(err)
			}
			original, err := store.Load(old.ID)
			if err != nil || !reflect.DeepEqual(original, old) {
				t.Fatalf("original changed %v", err)
			}
			if err := os.RemoveAll(r.Dir); err != nil {
				t.Fatal(err)
			}
			reopened, err := store.Load(saved.ID)
			if err != nil || !reflect.DeepEqual(reopened.Incremental, b) || !reflect.DeepEqual(reopened.ReviewedSliceIDs, saved.ReviewedSliceIDs) {
				t.Fatalf("offline capture/progress %v", err)
			}
			// The stored prior-head diff survives deletion of the predecessor session.
			if err := store.Delete(old.ID); err != nil {
				t.Fatal(err)
			}
			reopened, err = store.Load(saved.ID)
			if err != nil || reopened.Incremental.Diff == nil {
				t.Fatalf("deleted predecessor lost comparison %v", err)
			}
		})
	}
}

func TestIncrementalProgressRejectsStaleGenerationAndInventedRead(t *testing.T) {
	ctx := context.Background()
	r := testutil.NewRepo(t)
	r.Write("f", "old\n")
	base := r.Commit()
	r.Write("f", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	store, err := session.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	raw, err := Open(ctx, r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	old, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := OpenWithConfig(ctx, r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil, Config{Previous: old})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWithProgress(fresh.Snapshot, []string{fresh.Slices[0].FileID}); !errors.Is(err, session.ErrInvalidStateUpdate) {
		t.Fatalf("invented read %v", err)
	}
	if err := Mark(ctx, store, old, old.Slices[0].FileID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateWithProgress(fresh.Snapshot, nil); !errors.Is(err, session.ErrStateConflict) {
		t.Fatalf("stale read %v", err)
	}
	// Reconciler never treats unavailable comparison as empty-success proof.
	fresh.Incremental.Status = "unavailable"
	fresh.Incremental.Diff = nil
	fresh.Incremental.Reason = "Missing prior objects"
	if ids := incremental.Carry(old.Inventory, fresh.Inventory, old.ReviewedSliceIDs, fresh.Incremental); len(ids) != 0 {
		t.Fatal(ids)
	}
}
