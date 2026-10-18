package source

import (
	"bytes"
	"context"
	"errors"
	"slices"
)

const diffCacheBytes = 8 << 20
const diffCacheEntries = 2048

// Only the inventory's fixed blob-to-blob command is reusable. Paths, trees,
// refs, arbitrary diff options and commands with side effects never enter it.
func blobDiffKey(args []string) string {
	prefix := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--text", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--inter-hunk-context=0", "--full-index"}
	n := len(prefix)
	if len(args) != n+3 || !slices.Equal(args[:n], prefix) || args[n+2] != "--" || !shaPattern.MatchString(args[n]) || !shaPattern.MatchString(args[n+1]) {
		return ""
	}
	return args[n] + args[n+1]
}

func (v *View) blobDiff(ctx context.Context, limit int, key string, args []string) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if limit <= 0 {
		return nil, errors.New("invalid process limits")
	}
	v.blobMu.Lock()
	if v.closed {
		v.blobMu.Unlock()
		return nil, ErrCommand
	}
	if data, ok := v.diffs[key]; ok {
		if len(data) > limit {
			v.blobMu.Unlock()
			return nil, ErrLimit
		}
		copy := bytes.Clone(data)
		v.blobMu.Unlock()
		return copy, nil
	}
	v.blobMu.Unlock()
	data, err := v.gitInput(ctx, limit, nil, args...)
	if err == nil {
		v.blobMu.Lock()
		if _, exists := v.diffs[key]; !v.closed && !exists && len(v.diffs) < diffCacheEntries && len(data) <= diffCacheBytes-v.diffSize {
			if v.diffs == nil {
				v.diffs = map[string][]byte{}
			}
			v.diffs[key] = bytes.Clone(data)
			v.diffSize += len(data)
		}
		v.blobMu.Unlock()
	}
	return data, err
}
