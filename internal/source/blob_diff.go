package source

import (
	"bytes"
	"context"
	"errors"
	"slices"
)

const diffCacheBytes = 8 << 20
const diffCacheEntries = 2048

var blobDiffPrefix = []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--text", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--inter-hunk-context=0", "--full-index"}

// BlobDiffArgs is the canonical committed-blob comparison used by inventory.
// Each caller owns its argument slice; the shared prefix remains immutable.
func BlobDiffArgs(old, next string) []string {
	args := make([]string, len(blobDiffPrefix)+3)
	copy(args, blobDiffPrefix)
	copy(args[len(blobDiffPrefix):], []string{old, next, "--"})
	return args
}

// Only the inventory's fixed blob-to-blob command is reusable. Paths, trees,
// refs, arbitrary diff options and commands with side effects never enter it.
func blobDiffKey(args []string) string {
	n := len(blobDiffPrefix)
	if len(args) != n+3 || !slices.Equal(args[:n], blobDiffPrefix) || args[n+2] != "--" || !shaPattern.MatchString(args[n]) || !shaPattern.MatchString(args[n+1]) {
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
	if result, ok := v.cachedDiff(key, limit); ok {
		return result.Data, result.Err
	}
	data, err := v.gitInput(ctx, limit, nil, args...)
	if err == nil {
		v.cacheDiff(key, data)
	}
	return data, err
}

func (v *View) cachedDiff(key string, limit int) (BlobResult, bool) {
	v.blobMu.Lock()
	defer v.blobMu.Unlock()
	if v.closed {
		return BlobResult{Err: ErrCommand}, true
	}
	data, ok := v.diffs[key]
	if !ok {
		return BlobResult{}, false
	}
	if len(data) > limit {
		return BlobResult{Err: ErrLimit}, true
	}
	return BlobResult{Data: bytes.Clone(data)}, true
}

func (v *View) cacheDiff(key string, data []byte) {
	v.blobMu.Lock()
	defer v.blobMu.Unlock()
	if _, exists := v.diffs[key]; !v.closed && !exists && len(v.diffs) < diffCacheEntries && len(data) <= diffCacheBytes-v.diffSize {
		if v.diffs == nil {
			v.diffs = map[string][]byte{}
		}
		v.diffs[key] = bytes.Clone(data)
		v.diffSize += len(data)
	}
}
