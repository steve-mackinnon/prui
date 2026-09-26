package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLifecycleInterruptedProcessPreservesCommittedState(t *testing.T) {
	if path := os.Getenv("PR_REVIEW_TEST_CRASH_STORE"); path != "" {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		record, err := store.Create(fixture())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpdateState(context.Background(), record.ID, record.Generation, record.SnapshotReference,
			StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current}); err != nil {
			t.Fatal(err)
		}
		os.Exit(23)
	}
	path := filepath.Join(t.TempDir(), "storage")
	cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleInterruptedProcessPreservesCommittedState$")
	cmd.Env = append(os.Environ(), "PR_REVIEW_TEST_CRASH_STORE="+path)
	if err := cmd.Run(); err == nil {
		t.Fatal("child did not interrupt")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	entries, err := store.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("committed session missing: %#v, %v", entries, err)
	}
	record, err := store.Load(entries[0].ID)
	if err != nil || record.Generation != 2 || len(record.ReviewedSliceIDs) != 1 {
		t.Fatalf("committed progress missing: %#v, %v", record, err)
	}
}

func TestLifecycleRejectsInvalidSnapshotAndMissingSourceRow(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.Slices[0].Units = []int{0, 0} },
		func(s *Snapshot) { s.UnitFiles[0] = -1 },
		func(s *Snapshot) { s.Inventory.Units[0].PatchReference = "missing" },
		func(s *Snapshot) { s.Inventory.Units[0].InventoryID = "other" },
	} {
		snapshot := fixture()
		mutate(&snapshot)
		if _, err := store.Create(snapshot); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	record, err := store.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	db, _ := store.db.SQL()
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM snapshots`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(record.ID); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("missing source row loaded: %v", err)
	}
}

func TestLifecycleKilledWriteTransactionRecoversOldState(t *testing.T) {
	if path := os.Getenv("PR_REVIEW_TEST_OPEN_TX"); path != "" {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		db, err := store.db.SQL()
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE sessions SET revision_status='stale',generation=generation+1`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`UPDATE snapshots SET payload=zeroblob(1048576)`); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".tx-ready", []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	path := filepath.Join(t.TempDir(), "storage")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleKilledWriteTransactionRecoversOldState$")
	cmd.Env = append(os.Environ(), "PR_REVIEW_TEST_OPEN_TX="+path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(path + ".tx-ready"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not open write transaction")
		}
		time.Sleep(20 * time.Millisecond)
	}
	journal, err := os.Stat(filepath.Join(path, "store.sqlite3-journal"))
	if err != nil || journal.Size() == 0 {
		t.Fatalf("write transaction did not journal: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("child was not killed")
	}
	if readonly, err := OpenReadOnly(path); err == nil {
		readonly.Close()
		t.Fatal("read-only open recovered hot journal")
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Load(record.ID)
	if err != nil || got.Generation != 1 || got.RevisionStatus != Unchecked {
		t.Fatalf("uncommitted state survived crash: %#v, %v", got, err)
	}
}

func TestLifecycleIndependentProcessesConflictOnGeneration(t *testing.T) {
	if path := os.Getenv("PR_REVIEW_TEST_CAS_PATH"); path != "" {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		worker := os.Getenv("PR_REVIEW_TEST_CAS_WORKER")
		if err := os.WriteFile(path+".cas-ready-"+worker, []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(path + ".cas-start"); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("start signal missing")
			}
			time.Sleep(10 * time.Millisecond)
		}
		_, err = store.UpdateState(context.Background(), os.Getenv("PR_REVIEW_TEST_CAS_ID"), 1,
			os.Getenv("PR_REVIEW_TEST_CAS_REF"), StateUpdate{ReviewedSliceIDs: []string{"file"}, RevisionStatus: Current})
		if err == nil {
			os.Exit(0)
		}
		if errors.Is(err, ErrStateConflict) {
			os.Exit(42)
		}
		t.Fatalf("unexpected competing update: %v", err)
	}
	path := filepath.Join(t.TempDir(), "storage")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands := make([]*exec.Cmd, 2)
	for i := range commands {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLifecycleIndependentProcessesConflictOnGeneration$")
		cmd.Env = append(os.Environ(), "PR_REVIEW_TEST_CAS_PATH="+path,
			"PR_REVIEW_TEST_CAS_WORKER="+string(rune('0'+i)), "PR_REVIEW_TEST_CAS_ID="+record.ID,
			"PR_REVIEW_TEST_CAS_REF="+record.SnapshotReference)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands[i] = cmd
	}
	defer func() {
		for _, cmd := range commands {
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for i := range commands {
		for {
			if _, err := os.Stat(path + ".cas-ready-" + string(rune('0'+i))); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("worker did not open store")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err := os.WriteFile(path+".cas-start", []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	success, conflict := 0, 0
	for _, cmd := range commands {
		err := cmd.Wait()
		if err == nil {
			success++
			continue
		}
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 42 {
			conflict++
			continue
		}
		t.Fatalf("worker failed: %v", err)
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("successes=%d conflicts=%d", success, conflict)
	}
	got, err := store.Load(record.ID)
	if err != nil || got.Generation != 2 {
		t.Fatalf("committed state = %#v, %v", got, err)
	}
}
