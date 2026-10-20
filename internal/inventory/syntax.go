package inventory

import (
	"bytes"
	"context"
	"prui/internal/syntax"
	"time"
)

// hunkSyntax retains only categories for displayed source, never blob text.
func hunkSyntax(h hunk, old, new syntax.Lines) syntax.Patch {
	result := syntax.Patch{}
	a, b := h.old.Start, h.newRange.Start
	inHunk := false
	for i, raw := range bytes.Split(h.patch, []byte{'\n'}) {
		if bytes.HasPrefix(raw, []byte("@@ ")) {
			inHunk = true
			continue
		}
		if !inHunk || len(raw) == 0 {
			continue
		}
		row := syntax.Row{}
		switch raw[0] {
		case '-':
			row.Old = old[a]
			a++
		case '+':
			row.New = new[b]
			b++
		case ' ':
			row.Old = old[a]
			row.New = new[b]
			a++
			b++
		}
		if len(row.Old)+len(row.New) > 0 {
			result[i] = row
		}
	}
	return result
}

// SyntaxDuration bounds optional highlighting across a comparison.
const SyntaxDuration = 5 * time.Second

// SyntaxDiagnostics includes failures and expected unsupported languages.
// Legacy explicit-empty captures have no recorded reason; do not invent one.
// Successful token-free captures carry Ready and never become warnings.
func SyntaxDiagnostics(files []FileChange, units []ReviewUnit, tokens map[string]syntax.Patch, statuses map[string]syntax.FileStatus) map[string]syntax.FileStatus {
	result := map[string]syntax.FileStatus{}
	for id, status := range statuses {
		if status.Old != syntax.Ready || status.New != syntax.Ready {
			result[id] = status
		}
	}
	empty, populated := map[string]bool{}, map[string]bool{}
	for _, u := range units {
		if u.Kind != TextHunk {
			continue
		}
		if _, ok := statuses[u.FileChangeID]; ok {
			continue
		}
		if patch := tokens[u.ID]; patch != nil {
			if len(patch) == 0 {
				empty[u.FileChangeID] = true
			} else {
				populated[u.FileChangeID] = true
			}
		}
	}
	for _, f := range files {
		if !empty[f.ID] || populated[f.ID] {
			continue
		}
		status := syntax.FileStatus{Old: syntax.Unknown, New: syntax.Unknown}
		if !syntax.Supported(string(f.OldPath)) {
			status.Old = syntax.Unsupported
		}
		if !syntax.Supported(string(f.NewPath)) {
			status.New = syntax.Unsupported
		}
		result[f.ID] = status
	}
	return result
}

// syntaxBudget tracks optional capture work without changing source limits.
type syntaxBudget struct {
	remaining    time.Duration
	bytes, spans int
}

func (b *syntaxBudget) capture(ctx context.Context, f FileChange, old, new []byte, hunks ...hunk) (oldTokens, newTokens syntax.Lines, status syntax.FileStatus) {
	status = syntax.FileStatus{Old: syntax.Ready, New: syntax.Ready}
	switch {
	case b.remaining <= 0:
		status = syntax.FileStatus{Old: syntax.InventoryTimeLimit, New: syntax.InventoryTimeLimit}
	case b.bytes+len(old)+len(new) > 4<<20:
		status = syntax.FileStatus{Old: syntax.InventoryByteLimit, New: syntax.InventoryByteLimit}
	case b.spans >= 32000:
		status = syntax.FileStatus{Old: syntax.InventorySpanLimit, New: syntax.InventorySpanLimit}
	default:
		started := time.Now()
		syntaxContext, stop := context.WithTimeout(ctx, b.remaining)
		b.bytes += len(old) + len(new)
		oldEnd, newEnd := syntaxHunkEnds(hunks)
		oldTokens, status.Old = syntax.TokenizeThrough(syntaxContext, string(f.OldPath), old, oldEnd)
		newTokens, status.New = syntax.TokenizeThrough(syntaxContext, string(f.NewPath), new, newEnd)
		if syntaxContext.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			if status.Old == syntax.Timeout {
				status.Old = syntax.InventoryTimeLimit
			}
			if status.New == syntax.Timeout {
				status.New = syntax.InventoryTimeLimit
			}
		}
		stop()
		b.remaining -= time.Since(started)
	}
	if !syntax.Supported(string(f.OldPath)) {
		status.Old = syntax.Unsupported
	}
	if !syntax.Supported(string(f.NewPath)) {
		status.New = syntax.Unsupported
	}
	if len(old) == 0 {
		status.Old = syntax.Ready
	}
	if len(new) == 0 {
		status.New = syntax.Ready
	}
	return
}

func syntaxHunkEnds(hunks []hunk) (int, int) {
	ends := [2]int{}
	for _, h := range hunks {
		for i, r := range []Range{h.old, h.newRange} {
			// Outside the maximum lexer input, retain ordinary full capture.
			// Bounding both operands also keeps endpoint arithmetic safe.
			if r.Start < 0 || r.Count < 0 || r.Start > syntax.MaxBytes || r.Count > syntax.MaxBytes {
				return 0, 0
			}
			if r.Count > 0 {
				ends[i] = max(ends[i], r.Start+r.Count-1)
			}
		}
	}
	return ends[0], ends[1]
}

// SyntaxStorageStatuses also diagnoses legacy captures that predate statuses.
func SyntaxStorageStatuses(units []ReviewUnit, tokens map[string]syntax.Patch, statuses map[string]syntax.FileStatus) map[string]syntax.FileStatus {
	result := syntax.DroppedStatuses(statuses)
	for _, u := range units {
		if u.Kind != TextHunk {
			continue
		}
		if _, captured := tokens[u.ID]; !captured {
			continue
		}
		if _, recorded := statuses[u.FileChangeID]; recorded {
			continue
		}
		if result == nil {
			result = map[string]syntax.FileStatus{}
		}
		result[u.FileChangeID] = syntax.FileStatus{Old: syntax.StorageLimit, New: syntax.StorageLimit}
	}
	return result
}
