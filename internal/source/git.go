package source

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type View struct {
	dir, git string
	runner   Runner
	limits   Limits
}

// PinTiming records the bounded phases of building a pinned comparison.
// It is optional diagnostic data for callers such as the verifier.
type PinTiming struct {
	ViewSetup time.Duration
	MergeBase time.Duration
	Fetch     time.Duration
}

func (v *View) Close() error { return os.RemoveAll(v.dir) }
func gitEnvironment(home, token string) []string {
	env := []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "HOME=" + home, "XDG_CONFIG_HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_ATTR_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=", "GIT_PROTOCOL_FROM_USER=0", "GIT_LFS_SKIP_SMUDGE=1"}
	settings := [][2]string{{"core.hooksPath", home + "/empty"}, {"init.templateDir", home + "/empty"}, {"core.attributesFile", "/dev/null"}, {"core.pager", ""}, {"color.ui", "false"}, {"credential.helper", ""}, {"protocol.allow", "never"}, {"protocol.https.allow", "always"}, {"http.followRedirects", "false"}, {"fetch.recurseSubmodules", "false"}, {"maintenance.auto", "false"}, {"gc.auto", "0"}, {"diff.external", ""}, {"diff.ignoreSubmodules", "none"}, {"core.quotePath", "true"}, {"fetch.writeCommitGraph", "false"}}
	if token != "" {
		settings = append(settings, [2]string{"http.https://github.com/.extraHeader", "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+token))})
	}
	env = append(env, "GIT_CONFIG_COUNT="+strconv.Itoa(len(settings)))
	for i, p := range settings {
		env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, p[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, p[1]))
	}
	return env
}

func NewView(ctx context.Context, checkout string, r Runner, l Limits) (v *View, err error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	git, err := trustedExecutable("git")
	if err != nil {
		return nil, err
	}
	objects, err := objectDirectory(checkout)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "pr-review-")
	if err != nil {
		return nil, err
	}
	v = &View{dir, git, r, l}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, p := range []string{"empty", "objects/info", "objects/pack", "refs", "borrowed"} {
		if err = os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
			return nil, err
		}
	}
	for p, s := range map[string]string{"HEAD": "ref: refs/heads/unused\n", "config": "[core]\nrepositoryformatversion = 0\nbare = true\n"} {
		if err = os.WriteFile(filepath.Join(dir, p), []byte(s), 0600); err != nil {
			return nil, err
		}
	}
	// Mirror only object data, never the checkout's alternates, config, hooks or promisor markers.
	err = filepath.WalkDir(objects, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, e := filepath.Rel(objects, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return errors.New("unsupported symlink in object storage")
		}
		if d.IsDir() {
			if rel == "info" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dir, "borrowed", rel), 0700)
		}
		if !d.Type().IsRegular() {
			return errors.New("unsupported object storage entry")
		}
		if strings.HasSuffix(rel, ".promisor") || strings.HasSuffix(rel, ".bitmap") || strings.HasSuffix(rel, ".keep") {
			return nil
		}
		return os.Link(path, filepath.Join(dir, "borrowed", rel))
	})
	if err != nil {
		return nil, err
	}
	err = os.WriteFile(filepath.Join(dir, "objects/info/alternates"), []byte(filepath.Join(dir, "borrowed")+"\n"), 0600)
	return v, err
}

func objectDirectory(checkout string) (string, error) {
	root, e := filepath.Abs(checkout)
	if e != nil {
		return "", e
	}
	gitdir := filepath.Join(root, ".git")
	st, e := os.Lstat(gitdir)
	if e != nil {
		return "", errors.New("expected existing local checkout")
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("unsupported .git symlink")
	}
	if !st.IsDir() {
		b, e := readSmall(gitdir)
		if e != nil || !strings.HasPrefix(string(b), "gitdir: ") {
			return "", errors.New("invalid gitdir file")
		}
		gitdir = strings.TrimSpace(strings.TrimPrefix(string(b), "gitdir: "))
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(root, gitdir)
		}
	}
	if b, e := readSmall(filepath.Join(gitdir, "commondir")); e == nil {
		common := strings.TrimSpace(string(b))
		if filepath.IsAbs(common) {
			gitdir = common
		} else {
			gitdir = filepath.Join(gitdir, common)
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return "", e
	}
	objects := filepath.Join(gitdir, "objects")
	st, e = os.Lstat(objects)
	if e != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("invalid object directory")
	}
	return objects, nil
}
func readSmall(path string) ([]byte, error) {
	st, e := os.Stat(path)
	if e != nil {
		return nil, e
	}
	if st.Size() > 4096 {
		return nil, errors.New("oversized repository pointer")
	}
	return os.ReadFile(path)
}

func (v *View) Git(ctx context.Context, limit int, args ...string) ([]byte, error) {
	return v.runner.Run(ctx, Request{Program: v.git, Args: append([]string{"--no-pager", "--git-dir=" + v.dir}, args...), Dir: v.dir, Env: gitEnvironment(v.dir, ""), Limit: limit, Timeout: v.limits.Operation})
}
func (v *View) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	if !shaPattern.MatchString(oid) {
		return nil, errors.New("invalid object ID")
	}
	b, e := v.Git(ctx, 100, "cat-file", "-s", oid)
	if e != nil {
		return nil, e
	}
	n, e := strconv.Atoi(strings.TrimSpace(string(b)))
	if e != nil || n > limit {
		return nil, ErrLimit
	}
	return v.Git(ctx, limit, "cat-file", "blob", oid)
}
func (v *View) mergeBase(ctx context.Context, m Metadata) (string, error) {
	for _, s := range []string{m.BaseSHA, m.HeadSHA} {
		if !shaPattern.MatchString(s) {
			return "", errors.New("invalid pinned SHA")
		}
		if _, e := v.Git(ctx, 100, "cat-file", "-e", s+"^{commit}"); e != nil {
			return "", e
		}
	}
	b, e := v.Git(ctx, 4096, "merge-base", "--all", m.BaseSHA, m.HeadSHA)
	if e != nil {
		return "", e
	}
	bases := strings.Fields(string(b))
	if len(bases) != 1 {
		return "", ErrAmbiguous
	}
	for _, s := range []string{m.BaseSHA, m.HeadSHA} {
		if _, e = v.Git(ctx, 100, "merge-base", "--is-ancestor", bases[0], s); e != nil {
			return "", e
		}
	}
	objects, e := v.Git(ctx, 50<<20, "rev-list", "--objects", "--no-object-names", "--missing=print", "--no-walk", m.BaseSHA, m.HeadSHA, bases[0], "--")
	if e != nil {
		return "", e
	}
	for _, oid := range strings.Fields(string(objects)) {
		if strings.HasPrefix(oid, "?") {
			return "", ErrCommand
		}
	}
	return bases[0], nil
}

var ErrAmbiguous = errors.New("comparison requires exactly one merge base")

func (v *View) fetch(ctx context.Context, m Metadata, gh GitHub, notify func(string)) error {
	notify("Fetching pinned GitHub objects into temporary isolated storage (cancel to stop)")
	token, e := gh.Token(ctx)
	if e != nil {
		return e
	}
	// Remove borrowed reachability before fetching: shallow tips must not hide missing ancestry.
	if e = os.Remove(filepath.Join(v.dir, "objects/info/alternates")); e != nil && !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	targets := [][2]string{{m.BaseRepository, m.BaseSHA}, {m.HeadRepository, m.HeadSHA}}
	for _, s := range targets {
		if !repositoryPattern.MatchString(s[0]) || !shaPattern.MatchString(s[1]) {
			return errors.New("invalid fetch target")
		}
	}
	checkStorage := func() error {
		size, err := storageSize(filepath.Join(v.dir, "objects"))
		if err != nil {
			return err
		}
		if size > v.limits.FetchBytes {
			return ErrLimit
		}
		return nil
	}
	if m.BaseRepository == m.HeadRepository {
		args := []string{"--no-pager", "--git-dir=" + v.dir, "fetch", "--no-tags", "--no-write-fetch-head", "--no-auto-maintenance", "--recurse-submodules=no", "https://github.com/" + m.BaseRepository + ".git", m.BaseSHA}
		if m.HeadSHA != m.BaseSHA {
			args = append(args, m.HeadSHA)
		}
		if _, e = v.runner.Run(ctx, Request{Program: v.git, Args: args, Dir: v.dir, Env: gitEnvironment(v.dir, token), Limit: 1 << 20, Timeout: v.limits.Operation, StorageDir: filepath.Join(v.dir, "objects"), StorageLimit: v.limits.FetchBytes}); e != nil {
			return fmt.Errorf("pinned fetch failed (check GitHub access): %w", e)
		}
		if e = checkStorage(); e != nil {
			return e
		}
	} else {
		for _, s := range targets {
			_, e = v.runner.Run(ctx, Request{Program: v.git, Args: []string{"--no-pager", "--git-dir=" + v.dir, "fetch", "--no-tags", "--no-write-fetch-head", "--no-auto-maintenance", "--recurse-submodules=no", "https://github.com/" + s[0] + ".git", s[1]}, Dir: v.dir, Env: gitEnvironment(v.dir, token), Limit: 1 << 20, Timeout: v.limits.Operation, StorageDir: filepath.Join(v.dir, "objects"), StorageLimit: v.limits.FetchBytes})
			if e != nil {
				return fmt.Errorf("pinned fetch failed (check GitHub access): %w", e)
			}
			if e = checkStorage(); e != nil {
				return e
			}
		}
	}
	return nil
}

type PinnedComparison struct {
	Metadata                                                  Metadata
	MergeBaseSHA, DiffSettings, InventoryVersion, InventoryID string
}

func Pin(ctx context.Context, checkout string, id Identity, gh GitHub, r Runner, l Limits, notify func(string)) (*View, PinnedComparison, error) {
	return PinWithTiming(ctx, checkout, id, gh, r, l, notify, nil)
}

// PinWithTiming performs Pin and optionally accumulates its internal stage durations.
func PinWithTiming(ctx context.Context, checkout string, id Identity, gh GitHub, r Runner, l Limits, notify func(string), timing *PinTiming) (*View, PinnedComparison, error) {
	if notify == nil {
		notify = func(string) {}
	}
	m, e := gh.Metadata(ctx, id)
	if e != nil {
		return nil, PinnedComparison{}, e
	}
	for attempt := 0; attempt < 2; attempt++ {
		started := time.Now()
		v, e := NewView(ctx, checkout, r, l)
		if timing != nil {
			timing.ViewSetup += time.Since(started)
		}
		if e != nil {
			return nil, PinnedComparison{}, e
		}
		started = time.Now()
		base, pinErr := v.mergeBase(ctx, m)
		if timing != nil {
			timing.MergeBase += time.Since(started)
		}
		if pinErr == nil {
			return v, PinnedComparison{Metadata: m, MergeBaseSHA: base}, nil
		}
		if errors.Is(pinErr, ErrAmbiguous) || errors.Is(pinErr, ErrLimit) || ctx.Err() != nil {
			_ = v.Close()
			return nil, PinnedComparison{}, pinErr
		}
		started = time.Now()
		fetchErr := v.fetch(ctx, m, gh, notify)
		if timing != nil {
			timing.Fetch += time.Since(started)
		}
		if errors.Is(fetchErr, ErrLimit) || errors.Is(fetchErr, ErrAuthentication) || ctx.Err() != nil {
			_ = v.Close()
			return nil, PinnedComparison{}, fetchErr
		}
		latest, readErr := gh.Metadata(ctx, id)
		if readErr != nil {
			_ = v.Close()
			return nil, PinnedComparison{}, readErr
		}
		if latest != m {
			_ = v.Close()
			if attempt == 1 {
				return nil, PinnedComparison{}, errors.New("PR revisions changed repeatedly during pin/fetch")
			}
			m = latest
			continue
		}
		if fetchErr != nil {
			_ = v.Close()
			return nil, PinnedComparison{}, fetchErr
		}
		started = time.Now()
		base, e = v.mergeBase(ctx, m)
		if timing != nil {
			timing.MergeBase += time.Since(started)
		}
		if e != nil {
			_ = v.Close()
			return nil, PinnedComparison{}, fmt.Errorf("unresolved pinned ancestry: %w", e)
		}
		return v, PinnedComparison{Metadata: m, MergeBaseSHA: base}, nil
	}
	return nil, PinnedComparison{}, errors.New("unable to pin comparison")
}
