// Package layoutprefs stores only user presentation choices, independently of
// immutable sessions and progress generations. Paths are supplied by the app.
package layoutprefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Preferences struct {
	Version           int  `json:"version"`
	Split             bool `json:"split"`
	RailWidth         int  `json:"rail_width"`
	CommitWidth       int  `json:"commit_width"`
	GroupFiles        bool `json:"group_files"`
	CollapseGenerated bool `json:"collapse_generated"`
}

func valid(p Preferences) bool {
	return p.Version == 1 && validWidth(p.RailWidth) && validWidth(p.CommitWidth)
}
func validWidth(n int) bool { return n == 0 || n >= 18 && n <= 4096 }

var errInvalid = errors.New("layout preferences unavailable or invalid; defaults used and file preserved")

func Load(path string) (Preferences, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{Version: 1}, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return Preferences{}, errInvalid
	}
	f, err := os.OpenInRoot(filepath.Dir(path), filepath.Base(path))
	if errors.Is(err, os.ErrNotExist) {
		return Preferences{Version: 1}, nil
	}
	if err != nil {
		return Preferences{}, errInvalid
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return Preferences{}, errInvalid
	}
	b, err := io.ReadAll(io.LimitReader(f, 4097))
	if err != nil || len(b) > 4096 {
		return Preferences{}, errInvalid
	}
	var p Preferences
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || !valid(p) {
		return Preferences{}, errInvalid
	}
	return p, nil
}
func Save(path string, p Preferences) error {
	if !valid(p) {
		return errInvalid
	}
	if _, err := Load(path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errInvalid
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errInvalid
	}
	b, err := json.Marshal(p)
	if err != nil {
		return errInvalid
	}
	f, err := os.CreateTemp(dir, ".layout-*")
	if err != nil {
		return errInvalid
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return errInvalid
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return errInvalid
	}
	if err = f.Close(); err != nil {
		return errInvalid
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return errInvalid
	}
	d, err := os.OpenInRoot(dir, ".")
	if err != nil {
		return errors.New("layout preference saved; directory durability unverified")
	}
	defer func() { _ = d.Close() }()
	if err = d.Sync(); err != nil {
		return errors.New("layout preference saved; directory durability unverified")
	}
	return nil
}
