package source

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const maxGitConfigBytes = 64 << 10

// RepositoryFromCheckout returns the GitHub repository named by a checkout's
// origin remote. It reads local Git metadata directly so repository-controlled
// Git configuration cannot run commands or load included files.
func RepositoryFromCheckout(checkout string) (string, error) {
	gitdir, common, err := gitDirectories(checkout)
	if err != nil {
		return "", err
	}
	remote, worktreeConfig, err := originFromConfig(filepath.Join(common, "config"))
	if err != nil {
		return "", err
	}
	if gitdir != common && worktreeConfig {
		worktreeRemote, _, err := originFromConfig(filepath.Join(gitdir, "config.worktree"))
		if err != nil {
			return "", err
		}
		if worktreeRemote != "" {
			remote = worktreeRemote
		}
	}
	if remote == "" {
		return "", errors.New("no origin remote configured")
	}
	return repositoryFromRemote(remote)
}

func gitDirectories(checkout string) (string, string, error) {
	root, err := filepath.Abs(checkout)
	if err != nil {
		return "", "", err
	}
	gitdir := filepath.Join(root, ".git")
	st, err := os.Lstat(gitdir)
	if err != nil {
		return "", "", errors.New("expected existing local checkout")
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("unsupported .git symlink")
	}
	if !st.IsDir() {
		if !st.Mode().IsRegular() {
			return "", "", errors.New("invalid gitdir file")
		}
		b, err := readSmall(gitdir)
		if err != nil || !strings.HasPrefix(string(b), "gitdir: ") {
			return "", "", errors.New("invalid gitdir file")
		}
		gitdir = strings.TrimSpace(strings.TrimPrefix(string(b), "gitdir: "))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(root, gitdir)
		}
	}
	gitdir, err = filepath.Abs(gitdir)
	if err != nil {
		return "", "", err
	}
	if st, err = os.Lstat(gitdir); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("invalid git directory")
	}
	common := gitdir
	if b, err := readRegularSmall(filepath.Join(gitdir, "commondir"), 4096); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitdir, common)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	common, err = filepath.Abs(common)
	if err != nil {
		return "", "", err
	}
	if st, err = os.Lstat(common); err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", "", errors.New("invalid common git directory")
	}
	return gitdir, common, nil
}

func originFromConfig(path string) (string, bool, error) {
	b, err := readRegularSmall(path, maxGitConfigBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	section := ""
	remote, worktreeConfig := "", false
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return "", false, errors.New("invalid git config section")
			}
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section == "include" || strings.HasPrefix(section, "includeif ") {
				return "", false, errors.New("git config includes are unsupported")
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return "", false, errors.New("invalid git config entry")
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch {
		case section == `remote "origin"` && key == "url":
			if remote != "" {
				return "", false, errors.New("multiple origin URLs are unsupported")
			}
			remote = value
		case section == "extensions" && key == "worktreeconfig":
			worktreeConfig = strings.EqualFold(value, "true")
		}
	}
	return remote, worktreeConfig, nil
}

func readRegularSmall(path string, limit int64) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("unsupported git config file")
	}
	//nolint:gosec // RepositoryFromCheckout supplies a bounded regular .git/config path after rejecting symlinks.
	return os.ReadFile(path)
}

func repositoryFromRemote(remote string) (string, error) {
	var path string
	switch {
	case strings.HasPrefix(remote, "git@github.com:"):
		path = strings.TrimPrefix(remote, "git@github.com:")
	case strings.HasPrefix(remote, "https://"), strings.HasPrefix(remote, "ssh://"):
		u, err := url.Parse(remote)
		if err != nil {
			return "", errors.New("unsupported GitHub origin URL")
		}
		_, hasPassword := u.User.Password()
		if hasPassword || (u.User != nil && u.User.Username() != "git") || !strings.EqualFold(u.Hostname(), "github.com") || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" {
			return "", errors.New("unsupported GitHub origin URL")
		}
		if u.Scheme == "https" && u.User != nil || u.Scheme == "ssh" && (u.User == nil || u.User.Username() != "git") {
			return "", errors.New("unsupported GitHub origin URL")
		}
		path = strings.TrimPrefix(u.EscapedPath(), "/")
	default:
		return "", errors.New("unsupported GitHub origin URL")
	}
	path = strings.TrimSuffix(path, ".git")
	if _, err := ParseIdentity("1", path); err != nil {
		return "", fmt.Errorf("invalid GitHub origin repository: %w", err)
	}
	return path, nil
}
