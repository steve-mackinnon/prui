package session

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestLifecycleInterruptedProcess(t *testing.T) {
	if path := os.Getenv("PR_REVIEW_TEST_CRASH_STORE"); path != "" {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.Create(fixture())
		if err != nil {
			t.Fatal(err)
		}
		r.ReviewedSliceIDs = []string{"file"}
		if err := s.Save(r); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, r.ID, ".write-interrupted"), []byte(`{"schema_version":`), 0600); err != nil {
			t.Fatal(err)
		}
		// Exit without defers to simulate losing the process before replacement.
		os.Exit(23)
	}
	dir := filepath.Join(t.TempDir(), "sessions")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleInterruptedProcess$")
	cmd.Env = append(os.Environ(), "PR_REVIEW_TEST_CRASH_STORE="+dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("child did not interrupt")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal("crashed process retained lock", err)
	}
	defer s.Close()
	entries, err := s.List()
	if err != nil || len(entries) != 1 || entries[0].Err != nil {
		t.Fatal("interrupted write lost prior record", entries, err)
	}
	r := entries[0].Record
	if len(r.ReviewedSliceIDs) != 1 {
		t.Fatal("completion lost")
	}
	for ref, patch := range fixture().Inventory.Patches {
		if !bytes.Equal(r.Inventory.Patches[ref], patch) {
			t.Fatal("diff changed")
		}
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleConcurrentAccess(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := s.Load(r.ID)
	b, _ := s.Load(r.ID)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, record := range []*Record{a, b} {
		wg.Go(func() { record.ReviewedSliceIDs = []string{"file"}; results <- s.Save(record) })
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("want one committed update, got %d", success)
	}
	got, err := s.Load(r.ID)
	if err != nil || got.Generation != 2 {
		t.Fatal("concurrent write corrupted record", err)
	}
}

func TestLifecycleInvalidSnapshotAndMissingArtifacts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.Slices[0].Units = []int{0, 0} },
		func(s *Snapshot) { s.UnitFiles[0] = -1 },
		func(s *Snapshot) { s.Inventory.Units[0].PatchReference = "missing" },
		func(s *Snapshot) { s.Inventory.Units[0].InventoryID = "other" },
	} {
		snap := fixture()
		mutate(&snap)
		if _, err := s.Create(snap); err == nil {
			t.Fatal("invalid ownership/reference accepted")
		}
	}
	r, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(s.Path(), r.ID, "snapshot.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(r.ID); err == nil {
		t.Fatal("missing source silently accepted")
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
}
