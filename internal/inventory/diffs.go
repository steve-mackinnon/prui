package inventory

import (
	"bytes"
	"context"

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

func batchDiffs(ctx context.Context, objects Objects, files []FileChange, blobs map[string]source.BlobResult, empty string, limits source.Limits) map[blobPair]source.BlobResult {
	reader, ok := objects.(blobDiffBatchReader)
	if !ok || len(blobs) == 0 {
		return nil
	}
	remainingBytes, remainingLines := limits.ContentBytes, limits.DiffLines
	seen := map[blobPair]bool{}
	var requests []source.BlobDiffRequest
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
			requests = append(requests, source.BlobDiffRequest{OldOID: pair.old, NewOID: pair.next, Limit: reservedBytes})
		}
	}
	if len(requests) == 0 {
		return nil
	}
	results := reader.BlobDiffBatch(ctx, requests)
	diffs := make(map[blobPair]source.BlobResult, len(requests))
	for i, request := range requests {
		result := source.BlobResult{Err: source.ErrCommand}
		if len(results) == len(requests) {
			result = results[i]
		}
		diffs[blobPair{request.OldOID, request.NewOID}] = result
	}
	return diffs
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
