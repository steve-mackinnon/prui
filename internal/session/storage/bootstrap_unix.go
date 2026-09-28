//go:build darwin || linux

package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const ownerName = ".sqlite-owner"
const lockName = ".sqlite-init.lock"
const dbName = "store.sqlite3"

type owner struct {
	Application string `json:"application"`
	Version     int    `json:"version"`
	Identity    string `json:"identity"`
	Phase       string `json:"phase"`
}

func bootstrap(ctx context.Context, root string) (string, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	if err := private(root, true); err != nil {
		return "", err
	}
	lockPath := filepath.Join(root, lockName)
	if err := privateIfExists(lockPath, false); err != nil {
		return "", err
	}
	// Opening an unrelated nonempty location must not add our lock to it.
	if err := recognize(root); err != nil {
		return "", err
	}
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer func() { _ = rootDir.Close() }()
	lock, err := openInitLock(rootDir)
	if err != nil {
		return "", err
	}
	defer func() { _ = lock.Close() }()
	for {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			break
		} else if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			return "", err
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w: %v", ErrBusy, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()
	if err := recognize(root); err != nil {
		return "", err
	}
	o, exists, err := readOwner(root)
	if err != nil {
		return "", err
	}
	if !exists {
		var raw [16]byte
		if _, err := rand.Read(raw[:]); err != nil {
			return "", err
		}
		o = owner{Application: "prui", Version: SchemaVersion, Identity: hex.EncodeToString(raw[:]), Phase: "initializing"}
		if err := writeOwner(root, o, true); err != nil {
			return "", err
		}
	}
	dbPath := filepath.Join(root, dbName)
	if err := privateIfExists(dbPath, false); err != nil {
		return "", err
	}
	_, err = os.Lstat(dbPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if o.Phase == "ready" {
			return "", fmt.Errorf("%w: established database missing", ErrCorrupt)
		}
		f, err := rootDir.OpenFile(dbName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return "", err
		}
		if err := f.Close(); err != nil {
			return "", err
		}
		if err := syncDir(root); err != nil {
			return "", err
		}
		opctx, cancel := OperationContext(ctx)
		defer cancel()
		db, err := openSQL(opctx, dbPath, false)
		if err != nil {
			return "", err
		}
		defer func() { _ = db.Close() }()
		if err := initialize(opctx, db, o.Identity); err != nil {
			return "", fmt.Errorf("%w: initialize: %v", ErrStorage, err)
		}
	case err != nil:
		return "", err
	default:
		opctx, cancel := OperationContext(ctx)
		defer cancel()
		db, err := openSQL(opctx, dbPath, false)
		if err != nil {
			return "", err
		}
		defer func() { _ = db.Close() }()
		if err := validate(opctx, db, o.Identity); err != nil {
			if o.Phase != "initializing" || !emptyUninitialized(opctx, db) {
				return "", fmt.Errorf("validate existing database (phase=%s empty=%t): %w", o.Phase, emptyUninitialized(opctx, db), err)
			}
			if err := ensureJournal(opctx, db); err != nil {
				return "", err
			}
			if err := initialize(opctx, db, o.Identity); err != nil {
				return "", Classify(err)
			}
		}
	}
	if o.Phase != "ready" {
		o.Phase = "ready"
		if err := writeOwner(root, o, false); err != nil {
			return "", err
		}
	}
	return o.Identity, nil
}

func openInitLock(rootDir *os.Root) (*os.File, error) {
	f, err := rootDir.OpenFile(lockName, os.O_RDWR, 0)
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		return f, err
	}
	f, err = rootDir.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if errors.Is(err, os.ErrExist) {
		return rootDir.OpenFile(lockName, os.O_RDWR, 0)
	}
	return f, err
}

func inspectReady(root string) (string, error) {
	if err := private(root, true); err != nil {
		return "", err
	}
	if err := recognize(root); err != nil {
		return "", err
	}
	o, exists, err := readOwner(root)
	if err != nil {
		return "", err
	}
	if !exists || o.Phase != "ready" {
		return "", ErrInvalidStore
	}
	if err := private(filepath.Join(root, dbName), false); err != nil {
		return "", fmt.Errorf("%w: database: %v", ErrCorrupt, err)
	}
	if err := private(filepath.Join(root, dbName+"-journal"), false); err == nil {
		return "", fmt.Errorf("%w: SQLite recovery may be needed; reopen with a writable command", ErrBusy)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return o.Identity, nil
}

func recognize(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	ownerSeen, dbSeen, tempSeen := false, false, false
	for _, e := range entries {
		if len(e.Name()) > len(".sqlite-owner-") && e.Name()[:len(".sqlite-owner-")] == ".sqlite-owner-" {
			tempSeen = true
			if err := privateIfExists(filepath.Join(root, e.Name()), false); err != nil {
				return err
			}
			continue
		}
		switch e.Name() {
		case ownerName:
			ownerSeen = true
		case dbName:
			dbSeen = true
		case lockName, dbName + "-journal":
		default:
			return ErrInvalidStore
		}
		if err := privateIfExists(filepath.Join(root, e.Name()), false); err != nil {
			return err
		}
	}
	if !ownerSeen && (dbSeen || tempSeen || len(entries) > 1 || (len(entries) == 1 && entries[0].Name() != lockName)) {
		return ErrInvalidStore
	}
	if tempSeen && !dbSeen {
		return ErrInvalidStore
	}
	return nil
}

func emptyUninitialized(ctx context.Context, db *sql.DB) bool {
	var applicationID, version, objects int
	if err := db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil {
		return false
	}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return false
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&objects); err != nil {
		return false
	}
	return applicationID == 0 && version == 0 && objects == 0
}

func readOwner(root string) (owner, bool, error) {
	path := filepath.Join(root, ownerName)
	if err := private(path, false); errors.Is(err, os.ErrNotExist) {
		return owner{}, false, nil
	} else if err != nil {
		return owner{}, false, err
	}
	rootDir, err := os.OpenRoot(root)
	if err != nil {
		return owner{}, false, err
	}
	defer func() { _ = rootDir.Close() }()
	f, err := rootDir.OpenFile(ownerName, os.O_RDONLY, 0)
	if err != nil {
		return owner{}, false, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, 256))
	if err != nil {
		return owner{}, false, err
	}
	var o owner
	if err := json.Unmarshal(b, &o); err != nil || o.Application != "prui" || len(o.Identity) != 32 || (o.Phase != "ready" && o.Phase != "initializing") {
		return owner{}, false, ErrInvalidStore
	}
	if o.Version != SchemaVersion {
		return owner{}, false, fmt.Errorf("%w: version %d", ErrUnsupportedSchema, o.Version)
	}
	if _, err := hex.DecodeString(o.Identity); err != nil {
		return owner{}, false, ErrInvalidStore
	}
	return o, true, nil
}

func writeOwner(root string, o owner, first bool) error {
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if first {
		rootDir, err := os.OpenRoot(root)
		if err != nil {
			return err
		}
		defer func() { _ = rootDir.Close() }()
		f, err := rootDir.OpenFile(ownerName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		if _, err := f.Write(b); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	} else {
		// Replace the small ownership record atomically after the schema commit.
		f, err := os.CreateTemp(root, ".sqlite-owner-*")
		if err != nil {
			return err
		}
		name := f.Name()
		defer func() { _ = os.Remove(name) }()
		if err := f.Chmod(0600); err != nil {
			_ = f.Close()
			return err
		}
		if _, err := f.Write(b); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Rename(name, filepath.Join(root, ownerName)); err != nil {
			return err
		}
	}
	return syncDir(root)
}

func privateIfExists(path string, directory bool) error {
	err := private(path, directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func private(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) || info.Mode().Perm()&0077 != 0 {
		return ErrInvalidStore
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Geteuid()) {
		return ErrInvalidStore
	}
	return nil
}

func syncDir(path string) error {
	rootDir, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = rootDir.Close() }()
	f, err := rootDir.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}
