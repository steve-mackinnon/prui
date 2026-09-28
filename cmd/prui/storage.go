package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func outsideCheckout(storage, checkout string) error {
	root, err := canonicalPath(checkout)
	if err != nil {
		return err
	}
	data, err := canonicalPath(storage)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, data)
	if err != nil {
		return err
	}
	if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("session storage must be outside the reviewed checkout")
	}
	return nil
}

func canonicalPath(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	suffix := ""
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !os.IsNotExist(err) || filepath.Dir(path) == path {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(path), suffix)
		path = filepath.Dir(path)
	}
}
