package session

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"prui/internal/session/storage"
)

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "prui", "storage"), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(base) {
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "prui", "storage"), nil
}

func Open(path string) (*Store, error) {
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func OpenReadOnly(path string) (*Store, error) {
	db, err := storage.OpenReadOnly(context.Background(), path)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}
