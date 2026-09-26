package session

import (
	"path/filepath"
	"testing"
)

func TestSnapshotDescriptionRoundTripsAndAbsentDescriptionRemainsReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "storage")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	description := "frozen description"
	snapshot := fixture()
	snapshot.PullRequestDescription = &description
	recorded, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	withoutDescription, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load(recorded.ID)
	if err != nil || got.PullRequestDescription == nil || *got.PullRequestDescription != description {
		t.Fatalf("stored description = %#v, %v", got, err)
	}
	old, err := s.Load(withoutDescription.ID)
	if err != nil || old.PullRequestDescription != nil {
		t.Fatalf("absent snapshot description = %#v, %v", old.PullRequestDescription, err)
	}
}

func TestComparisonSnapshotCacheReSessionsFrozenSourceWhenDescriptionChanges(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snapshot := fixture()
	description := "first description"
	snapshot.PullRequestDescription = &description
	metadata := snapshot.Inventory.Comparison.Metadata
	metadata.Description = description
	if _, err := s.Create(snapshot); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadComparisonSnapshot(metadata); err != nil || got == nil || got.PullRequestDescription == nil || *got.PullRequestDescription != description {
		t.Fatalf("matching description cache result = %#v, %v", got, err)
	}
	metadata.Description = "edited description"
	if got, err := s.LoadComparisonSnapshot(metadata); err != nil || got == nil || got.PullRequestDescription == nil || *got.PullRequestDescription != metadata.Description || got.Inventory.Comparison.Metadata.Description != metadata.Description {
		t.Fatalf("changed description cache result = %#v, %v", got, err)
	}
}

func TestSnapshotRejectsInvalidFrozenDescription(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	snapshot := fixture()
	invalid := string([]byte{0xff})
	snapshot.PullRequestDescription = &invalid
	if _, err := s.Create(snapshot); err == nil {
		t.Fatal("invalid description was stored")
	}
}
