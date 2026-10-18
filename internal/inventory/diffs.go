package inventory

import (
	"bytes"
	"context"
	"sort"
	"strings"

	"prui/internal/source"
)

type blobPair struct{ old, next string }

func fileBlobPair(f FileChange, empty string) blobPair {
	pair := blobPair{f.OldOID, f.NewOID}
	if f.OldMode == "000000" || f.OldMode == "160000" {
		pair.old = empty
	}
	if f.NewMode == "000000" || f.NewMode == "160000" {
		pair.next = empty
	}
	return pair
}

type blobDiffBatchReader interface {
	BlobDiffBatch(context.Context, []source.BlobDiffRequest) []source.BlobResult
}

type treeDiffBatchReader interface {
	TreeDiffBatch(context.Context, string, string, []source.TreeDiffRequest) map[int]source.BlobResult
}

func batchDiffs(ctx context.Context, objects Objects, files []FileChange, blobs map[string]source.BlobResult, empty, oldSHA, newSHA string, limits source.Limits) map[blobPair]source.BlobResult {
	reader, ok := objects.(blobDiffBatchReader)
	if !ok || len(blobs) == 0 {
		return nil
	}
	remainingBytes, remainingLines := limits.ContentBytes, limits.DiffLines
	seen := map[blobPair]bool{}
	var requests []source.BlobDiffRequest
	var treeRequests []source.TreeDiffRequest
	var changedPaths []string
	for _, f := range files {
		if f.OldOID == f.NewOID {
			continue
		}
		if f.OldMode == "160000" || f.NewMode == "160000" {
			if f.OldMode == "000000" || f.NewMode == "000000" || f.OldMode == f.NewMode {
				continue
			}
			break // A mixed gitlink/blob side still needs a serial source read.
		}
		pair := fileBlobPair(f, empty)
		old, haveOld := blobs[pair.old]
		next, haveNext := blobs[pair.next]
		if !haveOld || !haveNext {
			break // Never speculate past an earlier unknown budget consumer.
		}
		if old.Err != nil || next.Err != nil {
			continue
		}
		if binaryBlob(old.Data) || binaryBlob(next.Data) {
			continue
		}
		reservedBytes, reservedLines, fits := diffReservation(old.Data, next.Data, remainingBytes, remainingLines)
		if !fits {
			break
		}
		// Charge every file, including repeated pairs: inventory still stores
		// and accounts for each hunk independently in its original file order.
		remainingBytes -= reservedBytes
		remainingLines -= reservedLines
		if !seen[pair] {
			seen[pair] = true
			request := source.BlobDiffRequest{OldOID: pair.old, NewOID: pair.next, Limit: reservedBytes}
			requests = append(requests, request)
			path, status := string(f.NewPath), f.Status
			if status == "D" {
				path = string(f.OldPath)
			}
			if status == "A" || status == "D" {
				if changedPaths == nil {
					changedPaths = sortedChangedPaths(files)
				}
				prefix := path + "/"
				next := sort.SearchStrings(changedPaths, prefix)
				if next < len(changedPaths) && strings.HasPrefix(changedPaths[next], prefix) {
					status = "" // A pathspec would also read changed descendants.
				}
			}
			treeRequests = append(treeRequests, source.TreeDiffRequest{BlobDiffRequest: request, Path: path, Status: status})
		}
	}
	if len(requests) == 0 {
		return nil
	}
	prepared := map[int]source.BlobResult{}
	if treeReader, ok := objects.(treeDiffBatchReader); ok {
		if results := treeReader.TreeDiffBatch(ctx, oldSHA, newSHA, treeRequests); results != nil {
			prepared = results
		}
	}
	var missing []source.BlobDiffRequest
	var indices []int
	for i, request := range requests {
		if _, ok := prepared[i]; !ok {
			indices = append(indices, i)
			missing = append(missing, request)
		}
	}
	if len(missing) > 0 {
		results := reader.BlobDiffBatch(ctx, missing)
		for n, i := range indices {
			result := source.BlobResult{Err: source.ErrCommand}
			if len(results) == len(missing) {
				result = results[n]
			}
			prepared[i] = result
		}
	}
	diffs := make(map[blobPair]source.BlobResult, len(requests))
	for i, request := range requests {
		diffs[blobPair{request.OldOID, request.NewOID}] = prepared[i]
	}
	return diffs
}

func sortedChangedPaths(files []FileChange) []string {
	var paths []string
	for _, f := range files {
		for _, path := range [][]byte{f.OldPath, f.NewPath} {
			if len(path) > 0 {
				paths = append(paths, string(path))
			}
		}
	}
	sort.Strings(paths)
	return paths
}

func binaryBlob(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0
}

// Canonical blob diffs have fixed OID headers and at most 80 bytes of function
// context per hunk. Reserve 1 KiB per input record plus payload and 1 KiB fixed
// overhead: this also covers parseHunks duplicating the file header per hunk,
// line markers and both missing-newline notices. Eight output lines per input
// record plus eight fixed lines conservatively cover the same materialization.
// Check divisions before multiplying, so even 32-bit limits cannot overflow.
func diffReservation(old, next []byte, byteLimit, lineLimit int) (int, int, bool) {
	if byteLimit < 1024 || len(old) > byteLimit-1024 || len(next) > byteLimit-1024-len(old) || lineLimit < 8 {
		return 0, 0, false
	}
	records := sourceRecords(old) + sourceRecords(next)
	payload := len(old) + len(next)
	if records > (byteLimit-1024-payload)/1024 || records > (lineLimit-8)/8 {
		return 0, 0, false
	}
	return payload + 1024 + records*1024, 8 + records*8, true
}

func sourceRecords(data []byte) int {
	count := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}
	return count
}
