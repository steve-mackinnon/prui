package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryFromCheckoutAcceptsGitHubOrigin(t *testing.T) {
	for _, remote := range []string{
		"https://github.com/Owner/repo.git",
		"ssh://git@github.com/Owner/repo.git",
		"git@github.com:Owner/repo.git",
	} {
		t.Run(remote, func(t *testing.T) {
			checkout := checkoutWithOrigin(t, remote)
			got, err := RepositoryFromCheckout(checkout)
			if err != nil || got != "Owner/repo" {
				t.Fatalf("RepositoryFromCheckout() = %q, %v", got, err)
			}
		})
	}
}

func TestRepositoryFromCheckoutRejectsUnsafeOrUnsupportedOrigin(t *testing.T) {
	for _, remote := range []string{
		"https://github.example/owner/repo.git",
		"https://token@github.com/owner/repo.git",
		"ssh://git:token@github.com/owner/repo.git",
		"https://github.com/owner/repo.git?query=yes",
		"file:///tmp/repo",
	} {
		t.Run(remote, func(t *testing.T) {
			if _, err := RepositoryFromCheckout(checkoutWithOrigin(t, remote)); err == nil {
				t.Fatal("accepted unsupported origin")
			}
		})
	}
}

func TestRepositoryFromCheckoutRejectsIncludesAndDuplicateOrigins(t *testing.T) {
	checkout := t.TempDir()
	gitdir := filepath.Join(checkout, ".git")
	if err := os.Mkdir(gitdir, 0700); err != nil {
		t.Fatal(err)
	}
	config := "[include]\npath = ignored\n[remote \"origin\"]\nurl = https://github.com/owner/repo.git\n"
	if err := os.WriteFile(filepath.Join(gitdir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RepositoryFromCheckout(checkout); err == nil {
		t.Fatal("accepted include")
	}
	config = "[remote \"origin\"]\nurl = https://github.com/owner/one.git\nurl = https://github.com/owner/two.git\n"
	if err := os.WriteFile(filepath.Join(gitdir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RepositoryFromCheckout(checkout); err == nil {
		t.Fatal("accepted duplicate origin")
	}
}

func TestRepositoryFromCheckoutSupportsLinkedWorktreeConfiguration(t *testing.T) {
	checkout := t.TempDir()
	common := filepath.Join(t.TempDir(), "common")
	gitdir := filepath.Join(t.TempDir(), "worktree")
	for _, path := range []string{common, gitdir} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(checkout, ".git"), []byte("gitdir: "+gitdir+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "commondir"), []byte(common+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	commonConfig := "[extensions]\nworktreeConfig = true\n[remote \"origin\"]\nurl = https://github.com/owner/common.git\n"
	if err := os.WriteFile(filepath.Join(common, "config"), []byte(commonConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitdir, "config.worktree"), []byte("[remote \"origin\"]\nurl = git@github.com:owner/worktree.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := RepositoryFromCheckout(checkout)
	if err != nil || got != "owner/worktree" {
		t.Fatalf("RepositoryFromCheckout() = %q, %v", got, err)
	}
}

func TestRepositoryFromCheckoutRejectsSymlinkedConfig(t *testing.T) {
	checkout := t.TempDir()
	gitdir := filepath.Join(checkout, ".git")
	if err := os.Mkdir(gitdir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(target, []byte("[remote \"origin\"]\nurl = https://github.com/owner/repo.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(gitdir, "config")); err != nil {
		t.Fatal(err)
	}
	if _, err := RepositoryFromCheckout(checkout); err == nil {
		t.Fatal("accepted symlinked config")
	}
}

func checkoutWithOrigin(t *testing.T, remote string) string {
	t.Helper()
	checkout := t.TempDir()
	gitdir := filepath.Join(checkout, ".git")
	if err := os.Mkdir(gitdir, 0700); err != nil {
		t.Fatal(err)
	}
	config := "[remote \"origin\"]\nurl = " + remote + "\n"
	if err := os.WriteFile(filepath.Join(gitdir, "config"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	return checkout
}
