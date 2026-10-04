package commits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"prui/internal/inventory"
	"prui/internal/source"
	"sort"
	"strings"
)

// Compose builds one offline net comparison without touching the reviewed checkout.
func Compose(ctx context.Context, bundle *Bundle, selected []string, limits source.Limits) (inventory.Inventory, error) {
	empty := inventory.Inventory{Complete: true, Files: []inventory.FileChange{}, Units: []inventory.ReviewUnit{}, Patches: map[string][]byte{}}
	if ctx.Err() != nil {
		return empty, ctx.Err()
	}
	if len(selected) == 0 {
		return empty, nil
	}
	if bundle == nil || bundle.Composition == nil || bundle.Composition.Status != Captured || !ValidateComposition(bundle) {
		return empty, errors.New("selected commits: frozen composition source unavailable")
	}
	if limits.Entries <= 0 || limits.BlobBytes <= 0 || limits.ContentBytes <= 0 || limits.DiffLines <= 0 || limits.Operation <= 0 {
		return empty, errors.New("invalid composition limits")
	}
	entries := map[string]Entry{}
	for _, e := range bundle.Entries {
		entries[e.SHA] = e
	}
	chosen := map[string]bool{}
	for _, sha := range selected {
		if _, ok := entries[sha]; !ok {
			return empty, errors.New("selected commit is not captured")
		}
		chosen[sha] = true
	}
	// Find a single first-parent chain containing every selected commit.
	var ordered []Entry
	for sha := range chosen {
		chain := []Entry{}
		seen := map[string]bool{}
		cur := sha
		for {
			e, ok := entries[cur]
			if seen[cur] {
				return empty, errors.New("captured commit ancestry contains a cycle")
			}
			if !ok {
				break
			}
			seen[cur] = true
			if chosen[cur] {
				chain = append(chain, e)
			}
			if len(e.Parents) == 0 {
				break
			}
			cur = e.Parents[0]
		}
		if len(chain) == len(chosen) {
			for i := len(chain) - 1; i >= 0; i-- {
				ordered = append(ordered, chain[i])
			}
			break
		}
	}
	if len(ordered) == 0 {
		return empty, errors.New("selected commits have ambiguous first-parent ordering")
	}
	ctx, cancel := context.WithTimeout(ctx, limits.Operation)
	defer cancel()
	store, err := newNetStore(limits)
	if err != nil {
		return empty, err
	}
	defer os.RemoveAll(store.dir)
	remaining := min(limits.ContentBytes, 50<<20)
	for oid, data := range bundle.Composition.Blobs {
		if len(data) > limits.BlobBytes || len(data) > remaining {
			return empty, source.ErrLimit
		}
		remaining -= len(data)
		got, e := store.run(ctx, data, 100, "hash-object", "-w", "--stdin")
		if e != nil {
			return empty, e
		}
		if strings.TrimSpace(string(got)) != oid {
			return empty, errors.New("frozen blob identity mismatch")
		}
	}
	trees := map[string]string{}
	treeFor := func(sha string) (string, error) {
		if t, ok := trees[sha]; ok {
			return t, nil
		}
		entries, ok := bundle.Composition.Trees[sha]
		if !ok {
			return "", errors.New("frozen tree unavailable")
		}
		if len(entries) > limits.Entries {
			return "", source.ErrLimit
		}
		t, e := store.tree(ctx, entries)
		if e == nil {
			trees[sha] = t
		}
		return t, e
	}
	parent := func(e Entry) string {
		if len(e.Parents) > 0 {
			return e.Parents[0]
		}
		return EmptyTreeSHA
	}
	baseline, err := treeFor(parent(ordered[0]))
	if err != nil {
		return empty, err
	}
	if _, err = store.run(ctx, nil, 100, "read-tree", baseline); err != nil {
		return empty, err
	}
	lines := 0
	for _, e := range ordered {
		old, er := treeFor(parent(e))
		if er != nil {
			return empty, fmt.Errorf("commit %s: %w", e.SHA[:12], er)
		}
		next, er := treeFor(e.SHA)
		if er != nil {
			return empty, er
		}
		if remaining <= 0 {
			return empty, source.ErrLimit
		}
		patch, er := store.run(ctx, nil, remaining, "diff", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", old, next, "--")
		if er != nil {
			return empty, er
		}
		remaining -= len(patch)
		lines += bytes.Count(patch, []byte{'\n'})
		if lines > limits.DiffLines {
			return empty, source.ErrLimit
		}
		if len(patch) == 0 {
			continue
		}
		if _, er = store.run(ctx, patch, 1<<20, "apply", "--cached", "--3way", "--whitespace=nowarn", "-"); er != nil {
			if ctx.Err() != nil {
				return empty, ctx.Err()
			}
			if errors.Is(er, source.ErrLimit) {
				return empty, er
			}
			paths := []string{}
			unresolved, _ := store.run(ctx, nil, 1<<20, "ls-files", "--unmerged", "-z")
			seen := map[string]bool{}
			for _, record := range bytes.Split(unresolved, []byte{0}) {
				parts := bytes.SplitN(record, []byte{'\t'}, 2)
				if len(parts) == 2 && !seen[string(parts[1])] {
					seen[string(parts[1])] = true
					paths = append(paths, fmt.Sprintf("%q", parts[1]))
				}
			}
			if len(paths) == 0 && e.Diff != nil {
				for _, f := range e.Diff.Files {
					path := f.NewPath
					if len(path) == 0 {
						path = f.OldPath
					}
					paths = append(paths, fmt.Sprintf("%q", path))
				}
			}
			return empty, fmt.Errorf("commit %s: selected changes conflict at %s", e.SHA[:12], strings.Join(paths, ", "))
		}
	}
	final, err := store.run(ctx, nil, 100, "write-tree")
	if err != nil {
		return empty, err
	}
	materialLimits := limits
	materialLimits.ContentBytes = remaining
	materialLimits.DiffLines = limits.DiffLines - lines
	if materialLimits.ContentBytes <= 0 || materialLimits.DiffLines <= 0 {
		return empty, source.ErrLimit
	}
	inv, err := inventory.BuildCommit(ctx, &netObjects{store: store, remaining: remaining}, inventory.CommitComparison{ParentSHA: baseline, CommitSHA: strings.TrimSpace(string(final))}, materialLimits)
	if err != nil {
		return empty, err
	}
	if !inv.Complete {
		return empty, fmt.Errorf("selected commit diff exceeds materialization limits: %w", source.ErrLimit)
	}
	return inv, nil
}

type netStore struct {
	dir, git string
	limits   source.Limits
}

func newNetStore(l source.Limits) (*netStore, error) {
	git, e := exec.LookPath("git")
	if e != nil {
		return nil, e
	}
	git, e = filepath.Abs(git)
	if e != nil {
		return nil, e
	}
	dir, e := os.MkdirTemp("", "prui-net-")
	if e != nil {
		return nil, e
	}
	s := &netStore{dir, git, l}
	for _, p := range []string{"objects/info", "objects/pack", "refs", "empty"} {
		if e = os.MkdirAll(filepath.Join(dir, p), 0700); e != nil {
			_ = os.RemoveAll(dir)
			return nil, e
		}
	}
	for p, data := range map[string]string{"HEAD": "ref: refs/heads/unused\n", "config": "[core]\nrepositoryformatversion = 0\nbare = true\n"} {
		if e = os.WriteFile(filepath.Join(dir, p), []byte(data), 0600); e != nil {
			_ = os.RemoveAll(dir)
			return nil, e
		}
	}
	return s, nil
}
func (s *netStore) run(ctx context.Context, input []byte, limit int, args ...string) ([]byte, error) {
	env := []string{"PATH=/usr/bin:/bin", "LC_ALL=C", "HOME=" + s.dir, "XDG_CONFIG_HOME=" + s.dir, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_NO_REPLACE_OBJECTS=1", "GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_ATTR_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0", "GIT_CONFIG_COUNT=7", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=" + s.dir + "/empty", "GIT_CONFIG_KEY_1=core.attributesFile", "GIT_CONFIG_VALUE_1=/dev/null", "GIT_CONFIG_KEY_2=protocol.allow", "GIT_CONFIG_VALUE_2=never", "GIT_CONFIG_KEY_3=diff.external", "GIT_CONFIG_VALUE_3=", "GIT_CONFIG_KEY_4=maintenance.auto", "GIT_CONFIG_VALUE_4=false", "GIT_CONFIG_KEY_5=gc.auto", "GIT_CONFIG_VALUE_5=0", "GIT_CONFIG_KEY_6=init.templateDir", "GIT_CONFIG_VALUE_6=" + s.dir + "/empty"}
	return source.NewRunner().Run(ctx, source.Request{Program: s.git, Args: append([]string{"--no-pager", "--git-dir=" + s.dir}, args...), Env: env, Dir: s.dir, Stdin: input, Limit: limit, Timeout: s.limits.Operation, StorageDir: s.dir, StorageLimit: 128 << 20})
}
func (s *netStore) Git(ctx context.Context, limit int, args ...string) ([]byte, error) {
	return s.run(ctx, nil, limit, args...)
}
func (s *netStore) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	return s.Git(ctx, limit, "cat-file", "blob", oid)
}
func (s *netStore) tree(ctx context.Context, entries []TreeEntry) (string, error) {
	type node struct {
		files map[string]TreeEntry
		dirs  map[string]*node
	}
	root := &node{map[string]TreeEntry{}, map[string]*node{}}
	for _, e := range entries {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		parts := strings.Split(string(e.Path), "/")
		n := root
		for _, part := range parts[:len(parts)-1] {
			if n.dirs[part] == nil {
				n.dirs[part] = &node{map[string]TreeEntry{}, map[string]*node{}}
			}
			n = n.dirs[part]
		}
		n.files[parts[len(parts)-1]] = e
	}
	var write func(*node) (string, error)
	write = func(n *node) (string, error) {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		records := map[string]string{}
		for name, e := range n.files {
			kind := "blob"
			if e.Mode == "160000" {
				kind = "commit"
			}
			records[name] = e.Mode + " " + kind + " " + e.OID + "\t" + name + "\x00"
		}
		for name, child := range n.dirs {
			oid, e := write(child)
			if e != nil {
				return "", e
			}
			records[name] = "040000 tree " + oid + "\t" + name + "\x00"
		}
		names := make([]string, 0, len(records))
		for name := range records {
			names = append(names, name)
		}
		sort.Strings(names)
		var input strings.Builder
		for _, name := range names {
			input.WriteString(records[name])
		}
		out, e := s.run(ctx, []byte(input.String()), 100, "mktree", "-z", "--missing")
		return strings.TrimSpace(string(out)), e
	}
	return write(root)
}

// netObjects charges every inventory source read against the same operation
// budget already used to materialize frozen blobs and selected patches.
type netObjects struct {
	store     *netStore
	remaining int
}

func (o *netObjects) Git(ctx context.Context, limit int, args ...string) ([]byte, error) {
	if o.remaining <= 0 {
		return nil, source.ErrLimit
	}
	data, err := o.store.Git(ctx, min(limit, o.remaining), args...)
	if err == nil {
		o.remaining -= len(data)
	}
	return data, err
}
func (o *netObjects) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	return o.Git(ctx, limit, "cat-file", "blob", oid)
}
