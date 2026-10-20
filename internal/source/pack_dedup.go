package source

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const minDedupObjects = 2048

// Only a verified, independently owned pack can replace loose object links.
// Unsupported layouts retain ordinary mirroring. A failed optional integrity
// check authorizes no omissions; resource failures/cancellation stop the open.
func (v *View) preparePackedObjects(ctx context.Context, objects string) (map[string]bool, error) {
	packDir := filepath.Join(objects, "pack")
	st, err := os.Lstat(packDir)
	if err != nil || !st.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(packDir)
	if err != nil {
		return nil, nil
	}
	var indexEntry, packEntry os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".idx") {
			if indexEntry != nil || !safePackIndexName(name) || !entry.Type().IsRegular() {
				return nil, nil
			}
			indexEntry = entry
		}
	}
	if indexEntry == nil {
		return nil, nil
	}
	indexName := indexEntry.Name()
	packName := strings.TrimSuffix(indexName, ".idx") + ".pack"
	for _, entry := range entries {
		if entry.Name() == packName && entry.Type().IsRegular() {
			packEntry = entry
		}
	}
	if packEntry == nil {
		return nil, nil
	}
	candidate, err := os.MkdirTemp(v.dir, "pack-check-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(candidate) }()
	if err := os.Mkdir(filepath.Join(candidate, "pack"), 0700); err != nil {
		return nil, err
	}
	if err := borrowObjectEntry(ctx, objects, candidate, filepath.Join(packDir, indexName), indexEntry, nil); err != nil {
		return nil, err
	}
	index, err := readPackCandidate(filepath.Join(candidate, "pack", indexName), min(v.limits.ContentBytes, 1<<20))
	if err != nil {
		return nil, nil
	}
	plan := preflightIndex(index, v.limits)
	if plan == nil || len(plan.oids) < minDedupObjects || !enoughPackedDuplicates(ctx, objects, plan.oids) {
		return nil, ctx.Err()
	}
	if err := borrowObjectEntry(ctx, objects, candidate, filepath.Join(packDir, packName), packEntry, nil); err != nil {
		return nil, err
	}
	budget := min(int64(min(v.limits.ContentBytes, maxDedupPackBytes)), v.limits.FetchBytes) - int64(len(index))
	if budget <= 0 {
		return nil, nil
	}
	pack, err := readPackCandidate(filepath.Join(candidate, "pack", packName), int(budget))
	if err != nil {
		return nil, nil
	}
	plan = preflightPack(ctx, index, pack, v.limits)
	if plan == nil {
		return nil, ctx.Err()
	}
	// Fetch unhooks borrowed reachability; keep copies behind that same barrier
	// and outside the directory charged against the temporary fetch allowance.
	privateDir := filepath.Join(v.dir, "borrowed", "pack")
	if err := os.MkdirAll(privateDir, 0700); err != nil {
		return nil, err
	}
	privateIndex := filepath.Join(privateDir, indexName)
	privatePack := filepath.Join(privateDir, packName)
	// These are new files in the private view, never the candidate hard links.
	if err := writeNewPackFile(privateIndex, index); err != nil {
		return nil, err
	}
	if err := writeNewPackFile(privatePack, pack); err != nil {
		return nil, err
	}
	_, err = v.Git(ctx, min(v.limits.ContentBytes, 1024), "index-pack", "--verify", "--no-rev-index", "--threads=1", "--max-input-size=16777216", privatePack)
	if err != nil {
		for _, path := range []string{privateIndex, privatePack} {
			if cleanup := os.Remove(path); cleanup != nil {
				return nil, cleanup
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrLimit) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, nil // No integrity proof: keep every ordinary object link.
	}
	skip := map[string]bool{"pack/" + indexName: true, "pack/" + packName: true}
	for _, oid := range plan.oids {
		skip[oid[:2]+"/"+oid[2:]] = true
	}
	return skip, nil
}

func safePackIndexName(name string) bool {
	stem, ok := strings.CutSuffix(name, ".idx")
	if !ok || len(stem) == 0 || len(name) > 128 {
		return false
	}
	for _, c := range []byte(stem) {
		valid := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !valid {
			return false
		}
	}
	return true
}

func writeNewPackFile(path string, data []byte) error {
	//nolint:gosec // path is a validated basename below the caller-owned private pack directory; exclusive creation cannot overwrite a borrowed hard link.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func readPackCandidate(path string, limit int) ([]byte, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > int64(limit) || limit <= 0 {
		return nil, ErrLimit
	}
	//nolint:gosec // path is an entry in the caller-owned private candidate directory; symlinks were rejected above.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, ErrLimit
	}
	return data, nil
}

func enoughPackedDuplicates(ctx context.Context, objects string, oids []string) bool {
	wanted := make(map[string]bool, len(oids))
	for _, oid := range oids {
		wanted[oid] = true
	}
	directories, err := os.ReadDir(objects)
	if err != nil {
		return false
	}
	count, scanned := 0, 0
	for _, directory := range directories {
		name := directory.Name()
		if !directory.IsDir() || len(name) != 2 {
			continue
		}
		//nolint:gosec // Only enumerate a child of the validated object directory; no file contents are read.
		f, err := os.Open(filepath.Join(objects, name))
		if err != nil {
			continue
		}
		for {
			entries, readErr := f.ReadDir(256)
			for _, entry := range entries {
				scanned++
				if entry.Type().IsRegular() && wanted[name+entry.Name()] {
					count++
				}
			}
			if count >= minDedupObjects || scanned >= maxDedupObjects*2 || ctx.Err() != nil || readErr != nil {
				break
			}
		}
		_ = f.Close()
		if count >= minDedupObjects || scanned >= maxDedupObjects*2 || ctx.Err() != nil {
			break
		}
	}
	return count >= minDedupObjects && ctx.Err() == nil
}
