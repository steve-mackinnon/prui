package testutil

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Repo struct {
	T         testing.TB
	Dir, Home string
}

func NewRepo(t testing.TB) *Repo {
	t.Helper()
	r := &Repo{t, t.TempDir(), t.TempDir()}
	r.Git("init", "--initial-branch=main", "--template=")
	return r
}
func (r *Repo) Git(args ...string) string {
	return r.GitInput("", args...)
}
func (r *Repo) GitInput(input string, args ...string) string {
	r.T.Helper()
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(input)
	cmd.Dir = r.Dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + r.Home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid", "GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid", "LC_ALL=C"}
	b, e := cmd.CombinedOutput()
	if e != nil {
		r.T.Fatalf("fixture git %v: %v: %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func (r *Repo) Write(path, content string) {
	r.T.Helper()
	p := filepath.Join(r.Dir, path)
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		r.T.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(content), 0600); e != nil {
		r.T.Fatal(e)
	}
}
func (r *Repo) Commit() string {
	r.T.Helper()
	r.Git("add", "--all")
	r.Git("commit", "--allow-empty", "-m", "synthetic fixture")
	return r.Git("rev-parse", "HEAD")
}
func (r *Repo) Snapshot() map[string]string {
	r.T.Helper()
	result := map[string]string{}
	e := filepath.WalkDir(r.Dir, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(r.Dir, p)
		if e != nil {
			return e
		}
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		var b []byte
		if d.Type()&os.ModeSymlink != 0 {
			target, e := os.Readlink(p)
			if e != nil {
				return e
			}
			b = []byte(target)
		} else {
			b, e = os.ReadFile(p)
			if e != nil {
				return e
			}
		}
		result[rel] = fmt.Sprintf("%s:%x", st.Mode(), sha256.Sum256(b))
		return nil
	})
	if e != nil {
		r.T.Fatal(e)
	}
	return result
}
