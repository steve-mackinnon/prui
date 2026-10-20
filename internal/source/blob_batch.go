package source

import (
	"bytes"
	"context"
	"strconv"
	"strings"
)

// BlobResult carries either complete bytes or a read failure, never partial
// contents. It has the same per-object limit semantics as Blob.
type BlobResult struct {
	Data []byte
	Err  error
}

// BlobBatch reads a bounded subset of full blob IDs through Git's batch
// protocol. Omitted IDs were not read and may use the ordinary Blob path.
// Returned failures must not be retried automatically. Only successful reads
// enter the view's existing bounded cache; the cache lives until Close.
func (v *View) BlobBatch(ctx context.Context, oids []string, limit int) map[string]BlobResult {
	results := map[string]BlobResult{}
	v.blobMu.Lock()
	var ids []string
	seen := map[string]bool{}
	remaining := blobCacheBytes - v.blobSize
	slots := blobCacheEntries - len(v.blobs)
	for _, oid := range oids {
		if seen[oid] {
			continue
		}
		seen[oid] = true
		switch {
		case ctx.Err() != nil:
			results[oid] = BlobResult{Err: ctx.Err()}
		case limit <= 0 || !shaPattern.MatchString(oid) || v.closed:
			results[oid] = BlobResult{Err: ErrCommand}
		default:
			if data, ok := v.blobs[oid]; ok {
				if len(data) > limit {
					results[oid] = BlobResult{Err: ErrLimit}
				} else {
					results[oid] = BlobResult{Data: bytes.Clone(data)}
				}
			} else if len(ids) < slots && remaining > 0 {
				ids = append(ids, oid)
			}
		}
	}
	v.blobMu.Unlock()
	if len(ids) == 0 {
		return results
	}
	fail := func(ids []string, err error) map[string]BlobResult {
		for _, oid := range ids {
			results[oid] = BlobResult{Err: err}
		}
		return results
	}
	input := []byte(strings.Join(ids, "\n") + "\n")
	metadata, err := v.gitInput(ctx, 128*len(ids), input, "cat-file", "--batch-check")
	if err != nil {
		return fail(ids, err)
	}
	type object struct {
		oid, header string
		size        int
	}
	var selected []object
	outputLimit := 0
	for _, oid := range ids {
		line, rest, ok := bytes.Cut(metadata, []byte{'\n'})
		if !ok {
			return fail(ids, ErrCommand)
		}
		metadata = rest
		fields := strings.Split(string(line), " ")
		if len(fields) == 2 && fields[0] == oid && fields[1] == "missing" {
			results[oid] = BlobResult{Err: ErrCommand}
			continue
		}
		if len(fields) != 3 || fields[0] != oid {
			return fail(ids, ErrCommand)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || strconv.Itoa(size) != fields[2] {
			return fail(ids, ErrCommand)
		}
		switch {
		case fields[1] != "blob":
			results[oid] = BlobResult{Err: ErrCommand}
		case size > limit:
			results[oid] = BlobResult{Err: ErrLimit}
		case size <= remaining:
			selected = append(selected, object{oid: oid, header: string(line), size: size})
			remaining -= size
			outputLimit += len(line) + 1 + size + 1
		}
	}
	if len(metadata) != 0 {
		return fail(ids, ErrCommand)
	}
	if len(selected) == 0 {
		return results
	}
	ids = ids[:0]
	for _, o := range selected {
		ids = append(ids, o.oid)
	}
	input = []byte(strings.Join(ids, "\n") + "\n")
	data, err := v.gitInput(ctx, outputLimit, input, "cat-file", "--batch")
	if err != nil {
		return fail(ids, err)
	}
	// Validate all frames before exposing or retaining any of them. Object bytes
	// may contain arbitrary newlines/NULs; only the size defines their boundary.
	frames := make([][]byte, len(selected))
	for i, o := range selected {
		header := []byte(o.header + "\n")
		if !bytes.HasPrefix(data, header) {
			return fail(ids, ErrCommand)
		}
		data = data[len(header):]
		if len(data) <= o.size || data[o.size] != '\n' {
			return fail(ids, ErrCommand)
		}
		frames[i] = data[:o.size:o.size]
		data = data[o.size+1:]
	}
	if len(data) != 0 {
		return fail(ids, ErrCommand)
	}
	for i, o := range selected {
		v.cacheBlob(o.oid, frames[i])
		results[o.oid] = BlobResult{Data: frames[i]}
	}
	return results
}
