package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"
	"time"

	"modernc.org/sqlite"
)

const ApplicationID = 0x50525256
const SchemaVersion = 1

var (
	ErrClosed            = errors.New("SQLite store closed")
	ErrReadOnly          = errors.New("SQLite store is read-only")
	ErrUnsupportedSchema = errors.New("unsupported SQLite store schema")
	ErrInvalidStore      = errors.New("invalid or unrelated SQLite store; choose a new empty storage directory")
	ErrBusy              = errors.New("SQLite store busy or operation canceled")
	ErrCorrupt           = errors.New("SQLite store is corrupt")
	ErrStorage           = errors.New("SQLite storage failure")
)

//go:embed schema.sql
var schemaFS embed.FS

type DB struct {
	mu       sync.Mutex
	sql      *sql.DB
	path     string
	readOnly bool
}

func (d *DB) SQL() (*sql.DB, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sql == nil {
		return nil, ErrClosed
	}
	return d.sql, nil
}

func (d *DB) Path() string   { return d.path }
func (d *DB) ReadOnly() bool { return d.readOnly }

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sql == nil {
		return nil
	}
	err := d.sql.Close()
	d.sql = nil
	return err
}

func Open(ctx context.Context, root string) (*DB, error) {
	return open(ctx, root, false)
}

func OpenReadOnly(ctx context.Context, root string) (*DB, error) {
	return open(ctx, root, true)
}

func open(ctx context.Context, root string, readOnly bool) (*DB, error) {
	ctx, cancel := OperationContext(ctx)
	defer cancel()
	path, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var identity string
	if readOnly {
		identity, err = inspectReady(path)
	} else {
		identity, err = bootstrap(ctx, path)
	}
	if err != nil {
		return nil, err
	}
	conn, err := openSQL(ctx, filepath.Join(path, "store.sqlite3"), readOnly)
	if err != nil {
		return nil, err
	}
	if err := validate(ctx, conn, identity); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !readOnly {
		if err := ensureJournal(ctx, conn); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return &DB{sql: conn, path: path, readOnly: readOnly}, nil
}

func openSQL(ctx context.Context, path string, readOnly bool) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	if readOnly {
		q.Set("mode", "ro")
	} else {
		q.Set("mode", "rw")
	}
	q.Set("_busy_timeout", "5000")
	q.Set("_foreign_keys", "on")
	q.Set("_synchronous", "EXTRA")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorage, err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := configure(ctx, db, readOnly); err != nil {
		_ = db.Close()
		return nil, Classify(err)
	}
	return db, nil
}

func configure(ctx context.Context, db *sql.DB, readOnly bool) error {
	_ = readOnly
	var fk, syncLevel, busy int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&syncLevel); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil {
		return err
	}
	if fk != 1 || syncLevel != 3 || busy != 5000 {
		return fmt.Errorf("%w: connection PRAGMA verification failed", ErrStorage)
	}
	return nil
}

func ensureJournal(ctx context.Context, db *sql.DB) error {
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&journal); err != nil {
		return Classify(err)
	}
	if journal != "delete" {
		return fmt.Errorf("%w: unexpected journal mode %q", ErrStorage, journal)
	}
	return nil
}

// Classify preserves cancellation, contention, corruption, and I/O categories
// while retaining the underlying driver error for diagnosis.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrBusy, err)
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() & 0xff {
		case 5, 6:
			return fmt.Errorf("%w: %w", ErrBusy, err)
		case 11, 26:
			return fmt.Errorf("%w: %w", ErrCorrupt, err)
		}
	}
	return fmt.Errorf("%w: %w", ErrStorage, err)
}

func validate(ctx context.Context, db *sql.DB, identity string) error {
	var applicationID, version int
	if err := db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&applicationID); err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("%w: %v", ErrStorage, err)
	}
	if applicationID != ApplicationID {
		return ErrInvalidStore
	}
	if version != SchemaVersion {
		return fmt.Errorf("%w: version %d", ErrUnsupportedSchema, version)
	}
	var got string
	if err := db.QueryRowContext(ctx, "SELECT identity FROM store_info WHERE id=1").Scan(&got); err != nil {
		return fmt.Errorf("%w: identity: %v", ErrCorrupt, err)
	}
	if got != identity {
		return ErrInvalidStore
	}
	return nil
}

func initialize(ctx context.Context, db *sql.DB, identity string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO store_info(id,identity) VALUES(1,?)", identity); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA application_id=%d", ApplicationID)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func OperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 10*time.Second)
}
