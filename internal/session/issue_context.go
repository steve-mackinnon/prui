package session

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"prui/internal/session/storage"
	"prui/internal/source"
	"strings"
)

var ErrIssueContextNotFound = errors.New("no saved issue context")

// This optional extension leaves version-1 frozen payloads and schema intact.
// It is created only on explicitly authorized context persistence. Read-only
// stores without it return not-found without migration or filesystem mutation.
const issueContextSchema = `CREATE TABLE IF NOT EXISTS issue_context (
 repository TEXT NOT NULL, pr_number INTEGER NOT NULL CHECK(pr_number>0),
 captured_at_ns INTEGER NOT NULL CHECK(captured_at_ns>0),
 digest TEXT NOT NULL CHECK(length(digest)=64),
 payload BLOB NOT NULL CHECK(length(payload) BETWEEN 1 AND 2097152),
 PRIMARY KEY(repository,pr_number)) STRICT`

func (s *Store) SaveIssueContext(ctx context.Context, c source.IssueContext) error {
	if s.db.ReadOnly() {
		return storage.ErrReadOnly
	}
	if !source.ValidateIssueContext(c) {
		return errors.New("invalid issue context")
	}
	b, err := json.Marshal(c)
	if err != nil || len(b) > 2<<20 {
		return errors.New("invalid issue context")
	}
	ctx, cancel := storage.OperationContext(ctx)
	defer cancel()
	db, err := s.db.SQL()
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return storage.Classify(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, issueContextSchema); err != nil {
		return storage.Classify(err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO issue_context(repository,pr_number,captured_at_ns,digest,payload) VALUES(?,?,?,?,?)
 ON CONFLICT(repository,pr_number) DO UPDATE SET captured_at_ns=excluded.captured_at_ns,digest=excluded.digest,payload=excluded.payload
 WHERE excluded.captured_at_ns>issue_context.captured_at_ns`, strings.ToLower(c.Identity.Repository), c.Identity.Number, c.CapturedAt.UnixNano(), fmt.Sprintf("%x", sha256.Sum256(b)), b)
	if err != nil {
		return storage.Classify(err)
	}
	return storage.Classify(tx.Commit())
}
func (s *Store) LoadIssueContext(ctx context.Context, id source.Identity) (source.IssueContext, error) {
	if _, err := source.ParseIdentity(fmt.Sprint(id.Number), id.Repository); err != nil {
		return source.IssueContext{}, err
	}
	ctx, cancel := storage.OperationContext(ctx)
	defer cancel()
	db, err := s.db.SQL()
	if err != nil {
		return source.IssueContext{}, err
	}
	var exists int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='issue_context'`).Scan(&exists); err != nil {
		return source.IssueContext{}, storage.Classify(err)
	}
	if exists == 0 {
		return source.IssueContext{}, ErrIssueContextNotFound
	}
	var b []byte
	var digest string
	var captured int64
	err = db.QueryRowContext(ctx, `SELECT payload,digest,captured_at_ns FROM issue_context WHERE repository=? AND pr_number=?`, strings.ToLower(id.Repository), id.Number).Scan(&b, &digest, &captured)
	if errors.Is(err, sql.ErrNoRows) {
		return source.IssueContext{}, ErrIssueContextNotFound
	}
	if err != nil {
		return source.IssueContext{}, storage.Classify(err)
	}
	var c source.IssueContext
	if len(b) > 2<<20 || fmt.Sprintf("%x", sha256.Sum256(b)) != digest || json.Unmarshal(b, &c) != nil || !source.ValidateIssueContext(c) || !strings.EqualFold(c.Identity.Repository, id.Repository) || c.Identity.Number != id.Number || c.CapturedAt.UnixNano() != captured {
		return source.IssueContext{}, errors.New("saved issue context is corrupt; preserved")
	}
	return c, nil
}
