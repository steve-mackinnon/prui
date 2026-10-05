package source

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path"
	"strings"
)

// Attributes evaluates committed rules using Git's own parser in a disposable
// index. The isolated View has no borrowed info/attributes, config, hooks or
// worktree; read-tree and check-attr never run filters or repository commands.
// The full tree/index and attribute blobs are bounded before index creation.
func (v *View) Attributes(ctx context.Context, sha string, names []string, paths [][]byte) (map[string]map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, v.limits.Operation)
	defer cancel()
	if !shaPattern.MatchString(sha) || len(names) > 16 || len(paths) > 10000 {
		return nil, ErrLimit
	}
	tree, err := v.Git(ctx, 8<<20, "ls-tree", "-r", "-z", sha, "--")
	if err != nil {
		return nil, err
	}
	entries := bytes.Split(bytes.TrimSuffix(tree, []byte{0}), []byte{0})
	if len(entries) > 100000 {
		return nil, ErrLimit
	}
	remaining := 2 << 20
	attributeFiles := 0
	for _, entry := range entries {
		header, p, ok := bytes.Cut(entry, []byte{'\t'})
		if !ok {
			return nil, ErrCommand
		}
		if path.Base(string(p)) != ".gitattributes" {
			continue
		}
		attributeFiles++
		if attributeFiles > 256 {
			return nil, ErrLimit
		}
		fields := strings.Fields(string(header))
		if len(fields) != 3 || fields[1] != "blob" || fields[0] != "100644" && fields[0] != "100755" {
			return nil, ErrCommand
		}
		if remaining <= 0 {
			return nil, ErrLimit
		}
		data, err := v.Blob(ctx, fields[2], min(64<<10, remaining))
		if err != nil {
			return nil, err
		}
		remaining -= len(data)
	}
	dir, err := os.MkdirTemp(v.dir, "attributes-")
	if err != nil {
		return nil, ErrCommand
	}
	defer func() { _ = os.RemoveAll(dir) }()
	index := dir + "/index"
	run := func(limit int, input []byte, args ...string) ([]byte, error) {
		env := append(gitEnvironment(v.dir, ""), "GIT_INDEX_FILE="+index)
		return v.runner.Run(ctx, Request{Program: v.git, Args: append([]string{"--no-pager", "--git-dir=" + v.dir}, args...), Dir: v.dir, Env: env, Stdin: input, Limit: limit, Timeout: v.limits.Operation, StorageDir: dir, StorageLimit: 16 << 20})
	}
	if _, err = run(100, nil, "read-tree", sha); err != nil {
		return nil, err
	}
	var input []byte
	expected := map[string]bool{}
	for _, p := range paths {
		if len(p) == 0 || bytes.IndexByte(p, 0) >= 0 || len(p) > 4096 {
			return nil, ErrLimit
		}
		input = append(input, p...)
		input = append(input, 0)
		expected[string(p)] = true
	}
	if len(input) > 4<<20 {
		return nil, ErrLimit
	}
	args := append([]string{"check-attr", "--cached", "-z", "--stdin"}, names...)
	raw, err := run(8<<20, input, args...)
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(raw) == 0 || len(parts) != len(paths)*len(names)*3 {
		return nil, errors.New("attribute evidence unavailable")
	}
	result := map[string]map[string]string{}
	recognized := map[string]bool{}
	for _, n := range names {
		recognized[n] = true
	}
	for i := 0; i < len(parts); i += 3 {
		p, n, value := string(parts[i]), string(parts[i+1]), string(parts[i+2])
		if !expected[p] || !recognized[n] {
			return nil, ErrCommand
		}
		if result[p] == nil {
			result[p] = map[string]string{}
		}
		if _, exists := result[p][n]; exists {
			return nil, ErrCommand
		}
		result[p][n] = value
	}
	return result, nil
}
