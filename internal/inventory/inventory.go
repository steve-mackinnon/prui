package inventory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"prui/internal/source"
)

type Kind string

const (
	TextHunk     Kind = "text_hunk"
	FileMetadata Kind = "file_metadata"
	Binary       Kind = "binary"
	Gitlink      Kind = "gitlink"
	Unavailable  Kind = "unavailable"
	Version           = "raw-v1"
	Settings          = "myers;context=3;inter-hunk=0;no-indent-heuristic;renames=50%;rename-limit=10000;blob-diff;binary=NUL-first-8192"
)

type FileChange struct {
	ID                                       string
	OldPath, NewPath                         []byte
	OldOID, NewOID, OldMode, NewMode, Status string
}
type Range struct{ Start, Count int }
type ReviewUnit struct {
	InventoryID, ID, FileChangeID     string
	Kind                              Kind
	OldRange, NewRange                Range
	PatchReference, UnavailableReason string
}
type Inventory struct {
	Comparison source.PinnedComparison
	Files      []FileChange
	Units      []ReviewUnit
	Patches    map[string][]byte
	Complete   bool
	Problems   []string
}
type Objects interface {
	Git(context.Context, int, ...string) ([]byte, error)
	Blob(context.Context, string, int) ([]byte, error)
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

var rawHeader = regexp.MustCompile(`^:([0-7]{6}) ([0-7]{6}) ([0-9a-f]{40}) ([0-9a-f]{40}) ([AMDT]|R[0-9]{1,3})$`)

func parseRaw(raw []byte, limit int) ([]FileChange, error) {
	files := []FileChange{}
	for len(raw) > 0 {
		if len(files) >= limit {
			return nil, fmt.Errorf("changed-entry limit: %w", source.ErrLimit)
		}
		i := bytes.IndexByte(raw, 0)
		if i < 0 {
			return nil, errors.New("unterminated raw header")
		}
		h := rawHeader.FindSubmatch(raw[:i])
		if h == nil {
			return nil, errors.New("unsupported raw change record")
		}
		raw = raw[i+1:]
		i = bytes.IndexByte(raw, 0)
		if i <= 0 {
			return nil, errors.New("missing raw path")
		}
		old := bytes.Clone(raw[:i])
		raw = raw[i+1:]
		newPath := bytes.Clone(old)
		if h[5][0] == 'R' {
			i = bytes.IndexByte(raw, 0)
			if i <= 0 {
				return nil, errors.New("missing rename destination")
			}
			newPath = bytes.Clone(raw[:i])
			raw = raw[i+1:]
		}
		f := FileChange{OldPath: old, NewPath: newPath, OldMode: string(h[1]), NewMode: string(h[2]), OldOID: string(h[3]), NewOID: string(h[4]), Status: string(h[5])}
		if f.Status == "A" {
			f.OldPath = nil
		}
		if f.Status == "D" {
			f.NewPath = nil
		}
		b, _ := json.Marshal(f)
		f.ID = digest(b)
		files = append(files, f)
	}
	return files, nil
}

func Build(ctx context.Context, objects Objects, p source.PinnedComparison, l source.Limits) (Inventory, error) {
	return build(ctx, objects, p.MergeBaseSHA, p.Metadata.HeadSHA, &p, CommitComparison{}, l)
}

// CommitComparison compares a commit tree to its first parent (or empty tree).
type CommitComparison struct{ ParentSHA, CommitSHA string }

// BuildCommit shares raw/blob diff rules without inventing pull-request metadata.
func BuildCommit(ctx context.Context, objects Objects, c CommitComparison, l source.Limits) (Inventory, error) {
	return build(ctx, objects, c.ParentSHA, c.CommitSHA, nil, c, l)
}

func build(ctx context.Context, objects Objects, oldSHA, newSHA string, p *source.PinnedComparison, c CommitComparison, l source.Limits) (Inventory, error) {
	inv := Inventory{Complete: true, Patches: map[string][]byte{}, Files: []FileChange{}, Units: []ReviewUnit{}, Problems: []string{}}
	if l.Entries <= 0 || l.BlobBytes <= 0 || l.ContentBytes <= 0 || l.DiffLines <= 0 {
		return inv, errors.New("invalid inventory limits")
	}
	raw, e := objects.Git(ctx, 50<<20, "diff-tree", "-r", "--raw", "-z", "--no-commit-id", "--no-abbrev", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "--no-renames", "-M50%", "-l10000", oldSHA, newSHA, "--")
	if e != nil {
		return inv, fmt.Errorf("inventory unavailable (not an empty diff): %w", e)
	}
	inv.Files, e = parseRaw(raw, l.Entries)
	if e != nil {
		return inv, e
	}

	settings := fmt.Sprintf("%s;entries=%d;content=%d;blob=%d;lines=%d", Settings, l.Entries, l.ContentBytes, l.BlobBytes, l.DiffLines)
	var key []byte
	if p != nil {
		p.InventoryVersion = Version
		p.DiffSettings = settings
		p.InventoryID = ""
		key, _ = json.Marshal(p)
		p.InventoryID = digest(append(key, raw...))
		inv.Comparison = *p
	} else {
		key, _ = json.Marshal(struct {
			Comparison CommitComparison
			Settings   string
		}{c, settings})
	}
	inventoryID := digest(append(key, raw...))

	empty, e := objects.Git(ctx, 100, "hash-object", "-w", "--stdin")
	if e != nil {
		return inv, e
	}
	emptyOID := strings.TrimSpace(string(empty))
	used, lines := 0, 0
	add := func(f FileChange, k Kind, old, newRange Range, patch []byte, reason string) {
		u := ReviewUnit{InventoryID: inventoryID, FileChangeID: f.ID, Kind: k, OldRange: old, NewRange: newRange, UnavailableReason: reason}
		if len(patch) > 0 {
			u.PatchReference = digest(patch)
			inv.Patches[u.PatchReference] = bytes.Clone(patch)
			used += len(patch)
			lines += bytes.Count(patch, []byte{'\n'})
		}
		key, _ := json.Marshal(u)
		u.ID = digest(key)
		inv.Units = append(inv.Units, u)
		if k == Unavailable {
			inv.Complete = false
			inv.Problems = append(inv.Problems, fmt.Sprintf("file %s: %s", f.ID[:12], reason))
		}
	}
	for _, f := range inv.Files {
		if ctx.Err() != nil {
			return inv, ctx.Err()
		}
		add(f, FileMetadata, Range{}, Range{}, nil, "")
		if f.OldMode == "160000" || f.NewMode == "160000" {
			add(f, Gitlink, Range{}, Range{}, nil, "")
			if f.OldMode == "000000" || f.NewMode == "000000" || (f.OldMode == "160000" && f.NewMode == "160000") {
				continue
			}
		}
		if f.OldOID == f.NewOID {
			continue
		}
		oldOID, newOID := f.OldOID, f.NewOID
		if f.OldMode == "000000" || f.OldMode == "160000" {
			oldOID = emptyOID
		}
		if f.NewMode == "000000" || f.NewMode == "160000" {
			newOID = emptyOID
		}
		a, ea := objects.Blob(ctx, oldOID, l.BlobBytes)
		b, eb := objects.Blob(ctx, newOID, l.BlobBytes)
		if ctx.Err() != nil {
			return inv, ctx.Err()
		}
		if ea != nil || eb != nil {
			reason := "committed blob unavailable"
			if errors.Is(ea, source.ErrLimit) || errors.Is(eb, source.ErrLimit) {
				reason = "per-blob limit exceeded"
			}
			add(f, Unavailable, Range{}, Range{}, nil, reason)
			continue
		}
		if bytes.IndexByte(a[:min(len(a), 8192)], 0) >= 0 || bytes.IndexByte(b[:min(len(b), 8192)], 0) >= 0 {
			add(f, Binary, Range{}, Range{}, nil, "")
			continue
		}
		remaining := l.ContentBytes - used
		if remaining <= 0 || lines >= l.DiffLines {
			add(f, Unavailable, Range{}, Range{}, nil, "materialized content or diff-line limit exceeded")
			continue
		}
		patch, e := objects.Git(ctx, remaining, "diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--text", "--diff-algorithm=myers", "--no-indent-heuristic", "--unified=3", "--inter-hunk-context=0", "--full-index", oldOID, newOID, "--")
		if ctx.Err() != nil {
			return inv, ctx.Err()
		}
		if e != nil {
			reason := "actual patch unavailable"
			if errors.Is(e, source.ErrLimit) {
				reason = "materialized content limit exceeded"
			}
			add(f, Unavailable, Range{}, Range{}, nil, reason)
			continue
		}
		hunks, e := parseHunks(patch)
		if e != nil {
			add(f, Unavailable, Range{}, Range{}, nil, "unrecognized actual patch")
			continue
		}
		totalBytes, totalLines := 0, 0
		for _, h := range hunks {
			totalBytes += len(h.patch)
			totalLines += bytes.Count(h.patch, []byte{'\n'})
		}
		if totalBytes > remaining || totalLines > l.DiffLines-lines {
			add(f, Unavailable, Range{}, Range{}, nil, "materialized content or diff-line limit exceeded")
			continue
		}
		for _, h := range hunks {
			add(f, TextHunk, h.old, h.newRange, h.patch, "")
		}
	}
	return inv, nil
}

type hunk struct {
	old, newRange Range
	patch         []byte
}

var hunkHeader = regexp.MustCompile(`(?m)^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@[^\n]*\n`)

func parseHunks(patch []byte) ([]hunk, error) {
	matches := hunkHeader.FindAllSubmatchIndex(patch, -1)
	if len(matches) == 0 {
		if len(patch) == 0 {
			return nil, nil
		}
		return nil, errors.New("patch has no hunks")
	}
	result := []hunk{}
	header := patch[:matches[0][0]]
	for i, m := range matches {
		values := [4]int{}
		for j := range values {
			start, end := m[2+j*2], m[3+j*2]
			if start < 0 {
				values[j] = 1
				continue
			}
			n, e := strconv.Atoi(string(patch[start:end]))
			if e != nil {
				return nil, e
			}
			values[j] = n
		}
		end := len(patch)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		actual := append(bytes.Clone(header), patch[m[0]:end]...)
		result = append(result, hunk{Range{values[0], values[1]}, Range{values[2], values[3]}, actual})
	}
	return result, nil
}
