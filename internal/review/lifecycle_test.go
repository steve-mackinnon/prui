package review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
)

type metadataReader struct {
	value source.Metadata
	err   error
}

func (m metadataReader) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return m.value, m.err
}

func TestLifecycleResumeAfterRepositoryDeletion(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "old\n")
	base := r.Commit()
	r.Write("a", "new\n")
	head := r.Commit()
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	raw, err := Open(context.Background(), r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sessions")
	store, err := session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.Create(raw.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := Mark(store, saved, saved.Slices[0].FileID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(r.Dir); err != nil {
		t.Fatal(err)
	}
	store, err = session.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	resumed, err := Resume(context.Background(), store, saved.ID, metadataReader{err: errors.New("offline")})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RevisionStatus != session.CheckFailed || !reflect.DeepEqual(resumed.Inventory, saved.Inventory) || !reflect.DeepEqual(resumed.ReviewedSliceIDs, saved.ReviewedSliceIDs) {
		t.Fatal("frozen snapshot/progress lost or offline claimed current")
	}
	for _, tc := range []struct {
		name     string
		metadata source.Metadata
		want     session.RevisionStatus
	}{
		{"same", meta, session.Current},
		{"base only", source.Metadata{Identity: meta.Identity, BaseRepository: meta.BaseRepository, HeadRepository: meta.HeadRepository, BaseSHA: head, HeadSHA: head}, session.Stale},
		{"head only", source.Metadata{Identity: meta.Identity, BaseRepository: meta.BaseRepository, HeadRepository: meta.HeadRepository, BaseSHA: base, HeadSHA: base}, session.Stale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Refresh(context.Background(), store, resumed, metadataReader{value: tc.metadata}); err != nil {
				t.Fatal(err)
			}
			if resumed.RevisionStatus != tc.want || len(resumed.ReviewedSliceIDs) != 1 {
				t.Fatal("wrong stale transition or old progress lost")
			}
		})
	}
	fresh, err := store.Create(resumed.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == saved.ID || len(fresh.ReviewedSliceIDs) != 0 {
		t.Fatal("plan carried completion")
	}
	if err := Mark(store, resumed, resumed.Slices[0].FileID, false); err != nil {
		t.Fatal(err)
	}
	if len(resumed.ReviewedSliceIDs) != 0 {
		t.Fatal("unmark failed")
	}
	if err := Mark(store, resumed, "fabricated", true); err == nil {
		t.Fatal("unknown slice accepted")
	}
}
