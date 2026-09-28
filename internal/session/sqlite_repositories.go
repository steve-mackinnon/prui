package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"prui/internal/session/storage"
)

func sqliteValidRepository(entry Repository) bool {
	return entry.Repository == strings.ToLower(entry.Repository) && repositoryPattern.MatchString(entry.Repository) && filepath.IsAbs(entry.Checkout) && filepath.Clean(entry.Checkout) == entry.Checkout
}

func (s *Store) LookupRepository(repository string) (string, error) {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	conn, err := s.db.SQL()
	if err != nil {
		return "", err
	}
	repository, err = normalizeRepository(repository)
	if err != nil {
		return "", err
	}
	var checkout []byte
	err = conn.QueryRowContext(ctx, "SELECT checkout FROM repositories WHERE repository=?", repository).Scan(&checkout)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrRepositoryNotFound
	}
	if err != nil {
		return "", fmt.Errorf("lookup repository: %w", storage.Classify(err))
	}
	entry := Repository{Repository: repository, Checkout: string(checkout)}
	if !sqliteValidRepository(entry) {
		return "", errors.New("invalid remembered repository record; original retained")
	}
	return entry.Checkout, nil
}

func (s *Store) ListRepositories() ([]Repository, error) {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	conn, err := s.db.SQL()
	if err != nil {
		return nil, err
	}
	return sqliteListRepositories(ctx, conn)
}

func sqliteListRepositories(ctx context.Context, conn interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) ([]Repository, error) {
	rows, err := conn.QueryContext(ctx, "SELECT repository,checkout FROM repositories ORDER BY ordinal")
	if err != nil {
		return nil, fmt.Errorf("list repositories: %w", storage.Classify(err))
	}
	defer func() { _ = rows.Close() }()
	var result []Repository
	for rows.Next() {
		var entry Repository
		var checkout []byte
		if err := rows.Scan(&entry.Repository, &checkout); err != nil {
			return nil, fmt.Errorf("scan repository: %w", storage.Classify(err))
		}
		entry.Checkout = string(checkout)
		if !sqliteValidRepository(entry) {
			return nil, errors.New("invalid remembered repository record; original retained")
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list repositories: %w", storage.Classify(err))
	}
	return result, nil
}

func (s *Store) RememberRepository(repository, checkout string) error {
	ctx, cancel := storage.OperationContext(context.Background())
	defer cancel()
	conn, err := s.db.SQL()
	if err != nil {
		return err
	}
	if s.db.ReadOnly() {
		return storage.ErrReadOnly
	}
	repository, err = normalizeRepository(repository)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(checkout) || filepath.Clean(checkout) != checkout {
		return errors.New("repository checkout must be a canonical absolute path")
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin repository update: %w", storage.Classify(err))
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "INSERT INTO repositories(repository,checkout,ordinal) VALUES(?,?,(SELECT COALESCE(MAX(ordinal)+1,0) FROM repositories)) ON CONFLICT(repository) DO NOTHING", repository, []byte(checkout)); err != nil {
		return fmt.Errorf("reserve repository writer: %w", storage.Classify(err))
	}
	if _, err := sqliteListRepositories(ctx, tx); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE repositories SET checkout=? WHERE repository=?", []byte(checkout), repository)
	if err != nil {
		return fmt.Errorf("remember repository: %w", storage.Classify(err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit repository update: %w", storage.Classify(err))
	}
	return nil
}
