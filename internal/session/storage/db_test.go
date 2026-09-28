package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestOpenRoundTripWithoutCGO(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec("INSERT INTO repositories(repository,checkout,ordinal) VALUES(?,?,?)", "a/b", []byte("/tmp/check"), 0); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	dbBefore, err := os.Stat(filepath.Join(root, dbName))
	if err != nil {
		t.Fatal(err)
	}
	ownerBefore, err := os.Stat(filepath.Join(root, ownerName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL(); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed SQL: %v", err)
	}
	ro, err := OpenReadOnly(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ro.Close() }()
	conn, _ = ro.SQL()
	var checkout []byte
	if err := conn.QueryRow("SELECT checkout FROM repositories WHERE repository=?", "a/b").Scan(&checkout); err != nil {
		t.Fatal(err)
	}
	if string(checkout) != "/tmp/check" {
		t.Fatalf("checkout=%q", checkout)
	}
	if _, err := conn.Exec("INSERT INTO repositories(repository,checkout,ordinal) VALUES(?,?,?)", "c/d", []byte("/tmp/other"), 1); err == nil {
		t.Fatal("read-only database accepted write")
	}
	dbAfter, _ := os.Stat(filepath.Join(root, dbName))
	ownerAfter, _ := os.Stat(filepath.Join(root, ownerName))
	if !dbBefore.ModTime().Equal(dbAfter.ModTime()) || !ownerBefore.ModTime().Equal(ownerAfter.ModTime()) {
		t.Fatal("read-only open changed persistent files")
	}
}

func TestConcurrentFirstOpen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := Open(context.Background(), root)
			if err == nil {
				err = db.Close()
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRejectUnownedAndMissingDatabase(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".format"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), root); !errors.Is(err, ErrInvalidStore) {
		t.Fatalf("legacy store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, lockName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("legacy store was modified")
	}
	root = filepath.Join(t.TempDir(), "fresh")
	if _, err := OpenReadOnly(context.Background(), root); err == nil {
		t.Fatal("read-only open created or accepted missing store")
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only open created root")
	}
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := os.Remove(filepath.Join(root, dbName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), root); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("ready with missing database: %v", err)
	}
}

func TestRejectAmbiguousBootstrapArtifacts(t *testing.T) {
	for _, name := range []string{".sqlite-owner-orphan", dbName + "-journal"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Chmod(root, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, name), []byte("orphan"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(context.Background(), root); !errors.Is(err, ErrInvalidStore) {
				t.Fatalf("orphan artifact: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, lockName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("ambiguous store was modified")
			}
		})
	}
}

func TestURIPathRoundTrip(t *testing.T) {
	root := filepath.Join(t.TempDir(), "storage spaces 雪 ?#%")
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.SQL()
	if _, err := conn.Exec("INSERT INTO repositories(repository,checkout,ordinal) VALUES(?,?,?)", "a/b", []byte("/tmp/雪 ?#%"), 0); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = OpenReadOnly(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, _ = db.SQL()
	var checkout []byte
	if err := conn.QueryRow("SELECT checkout FROM repositories WHERE repository=?", "a/b").Scan(&checkout); err != nil {
		t.Fatal(err)
	}
	if string(checkout) != "/tmp/雪 ?#%" {
		t.Fatalf("checkout=%q", checkout)
	}
}

func TestSchemaIdentityAndSettings(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, _ := db.SQL()
	for query, want := range map[string]int{
		"PRAGMA application_id": ApplicationID,
		"PRAGMA user_version":   SchemaVersion,
		"PRAGMA foreign_keys":   1,
		"PRAGMA synchronous":    3,
		"PRAGMA busy_timeout":   5000,
	} {
		var got int
		if err := conn.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d, error %v; want %d", query, got, err, want)
		}
	}
	var journal string
	if err := conn.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil || journal != "delete" {
		t.Fatalf("journal=%q error=%v", journal, err)
	}
	if _, err := conn.Exec("INSERT INTO progress(session_id,file_id,ordinal) VALUES(?,?,?)", "missing", "file", 0); err == nil {
		t.Fatal("foreign key disabled")
	}
	var sqliteVersion string
	if err := conn.QueryRow("SELECT sqlite_version()").Scan(&sqliteVersion); err != nil {
		t.Fatal(err)
	}
	t.Logf("bundled SQLite version: %s", sqliteVersion)
}

func TestClassification(t *testing.T) {
	if !errors.Is(Classify(context.Canceled), ErrBusy) {
		t.Fatal("cancellation classification")
	}
	if !errors.Is(Classify(sql.ErrNoRows), ErrStorage) {
		t.Fatal("storage classification")
	}
}

func TestInterruptedInitializationAndForeignDatabase(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	o := owner{Application: "prui", Version: SchemaVersion, Identity: "0123456789abcdef0123456789abcdef", Phase: "initializing"}
	if err := writeOwner(root, o, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, dbName), nil, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatalf("resume empty database: %v", err)
	}
	_ = db.Close()
	got, exists, err := readOwner(root)
	if err != nil || !exists || got.Phase != "ready" {
		t.Fatalf("owner after resume: %+v %v", got, err)
	}
	root = t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeOwner(root, o, true); err != nil {
		t.Fatal(err)
	}
	foreign, err := sql.Open("sqlite", filepath.Join(root, dbName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := foreign.Exec("CREATE TABLE foreign_data(x)"); err != nil {
		t.Fatal(err)
	}
	_ = foreign.Close()
	if _, err := Open(context.Background(), root); !errors.Is(err, ErrInvalidStore) {
		t.Fatalf("foreign database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, dbName)); err != nil {
		t.Fatalf("foreign database removed: %v", err)
	}
}

func TestUnsafeLinksAndUnsupportedSchema(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ownerName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), root); !errors.Is(err, ErrInvalidStore) {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Remove(filepath.Join(root, ownerName)); err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.SQL()
	if _, err := conn.Exec("PRAGMA user_version=999"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := Open(context.Background(), root); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("newer schema: %v", err)
	}
}

func TestConnectionReplacementRetainsPragmas(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	conn, _ := db.SQL()
	conn.SetMaxIdleConns(0)
	for query, want := range map[string]int{
		"PRAGMA foreign_keys": 1,
		"PRAGMA synchronous":  3,
		"PRAGMA busy_timeout": 5000,
	} {
		var got int
		if err := conn.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("reopened %s: got %d err %v, want %d", query, got, err, want)
		}
	}
}

func TestReadOnlyRefusesJournalWithoutRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	db, err := Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	journal := filepath.Join(root, dbName+"-journal")
	if err := os.WriteFile(journal, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(context.Background(), root); !errors.Is(err, ErrBusy) {
		t.Fatalf("journal read-only: %v", err)
	}
	b, err := os.ReadFile(journal)
	if err != nil || string(b) != "interrupted" {
		t.Fatalf("journal changed: %q, %v", b, err)
	}
}
