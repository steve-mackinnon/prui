package context

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"pr-review/internal/source"
)

type Kind string

const (
	Manifest      Kind = "manifest"
	Documentation Kind = "documentation"
	NearbyTest    Kind = "nearby_test"
	TextReference Kind = "text_reference"
	Source        Kind = "source"
)

type Evidence struct {
	EvidenceID      string `json:"evidence_id"`
	Repository      string `json:"repository"`
	CommitSHA       string `json:"commit_sha"`
	BlobID          string `json:"blob_id"`
	Path            []byte `json:"path"`
	LineStart       int    `json:"line_start"`
	LineEnd         int    `json:"line_end"`
	Kind            Kind   `json:"kind"`
	Excerpt         []byte `json:"excerpt"`
	RetrievalReason string `json:"retrieval_reason"`
	Relationship    string `json:"relationship"`
}

type Omitted struct {
	Path   []byte `json:"path"`
	Reason string `json:"reason"`
}
type ContextBundle struct {
	ComparisonID     string     `json:"comparison_id"`
	Evidence         []Evidence `json:"evidence"`
	ExaminedPaths    [][]byte   `json:"examined_paths"`
	OmittedPaths     []Omitted  `json:"omitted_paths_with_reasons"`
	FileBudget       int        `json:"file_budget,omitempty"`
	ExcerptBudget    int        `json:"excerpt_budget,omitempty"`
	ByteBudget       int        `json:"byte_budget,omitempty"`
	ExhaustedBudgets []string   `json:"exhausted_budgets,omitempty"`
}

type Objects interface {
	Git(context.Context, int, ...string) ([]byte, error)
	Blob(context.Context, string, int) ([]byte, error)
}
type Limits struct {
	Files, ExcerptBytes, Bytes int
	Duration                   time.Duration
}

var Defaults = Limits{Files: 100, ExcerptBytes: 32 << 10, Bytes: 256 << 10, Duration: 5 * time.Second}

type candidate struct {
	path        []byte
	oid, commit string
	kind        Kind
	reason      string
	rank        int
	skipReason  string
}

func Retrieve(parent context.Context, objects Objects, comparison source.PinnedComparison, inv inventory.Inventory, policy privacy.Policy, limits Limits) ContextBundle {
	result := ContextBundle{ComparisonID: comparison.InventoryID, FileBudget: limits.Files, ExcerptBudget: limits.ExcerptBytes, ByteBudget: limits.Bytes}
	ctx, cancel := context.WithTimeout(parent, limits.Duration)
	defer cancel()
	usedFiles, usedBytes := 0, 0
	seen := map[string]bool{}
	for _, f := range inv.Files {
		for _, p := range [][]byte{f.OldPath, f.NewPath} {
			if len(p) > 0 {
				result.ExaminedPaths = append(result.ExaminedPaths, bytes.Clone(p))
			}
		}
	}
	for _, commit := range []string{comparison.MergeBaseSHA, comparison.Metadata.HeadSHA} {
		out, err := objects.Git(ctx, 50<<20, "ls-tree", "-r", "-z", commit, "--")
		if err != nil {
			result.OmittedPaths = append(result.OmittedPaths, Omitted{Reason: "tree unavailable at " + commit})
			continue
		}
		cs := parseTree(out, commit)
		for i := range cs {
			cs[i].rank = rank(cs[i].path, inv)
		}
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].rank < cs[j].rank })
		for _, c := range cs {
			if usedFiles >= limits.Files {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: "file budget exhausted"})
				continue
			}
			key := c.commit + "\x00" + string(c.path)
			if seen[key] {
				continue
			}
			if c.skipReason != "" {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: c.skipReason})
				seen[key] = true
				continue
			}
			if c.commit == comparison.MergeBaseSHA && !isDeletedSide(c.path, inv) {
				continue
			}
			if c.commit == comparison.Metadata.HeadSHA && isDeletedSide(c.path, inv) {
				continue
			}
			if excluded, reason := policy.ExcludedPath(c.path); excluded {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: reason})
				continue
			}
			b, err := objects.Blob(ctx, c.oid, limits.ExcerptBytes)
			if err != nil {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: "blob unavailable"})
				continue
			}
			if excluded, reason := policy.ExcludedContent(b); excluded {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: reason})
				continue
			}
			if len(b) == 0 || bytes.IndexByte(b, 0) >= 0 {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: "binary or empty content"})
				continue
			}
			remaining := limits.Bytes - usedBytes
			if remaining <= 0 {
				result.OmittedPaths = append(result.OmittedPaths, Omitted{Path: c.path, Reason: "excerpt byte budget exhausted"})
				continue
			}
			if len(b) > remaining {
				b = b[:remaining]
				result.ExhaustedBudgets = append(result.ExhaustedBudgets, "excerpt bytes")
			}
			id := sha256.Sum256(append(append([]byte{}, c.commit...), append(c.path, b...)...))
			result.Evidence = append(result.Evidence, Evidence{EvidenceID: hex.EncodeToString(id[:]), Repository: comparison.Metadata.Identity.Repository, CommitSHA: c.commit, BlobID: c.oid, Path: bytes.Clone(c.path), LineStart: 1, LineEnd: 1 + bytes.Count(b, []byte{'\n'}), Kind: c.kind, Excerpt: bytes.Clone(b), RetrievalReason: c.reason, Relationship: "observed"})
			usedFiles++
			usedBytes += len(b)
			seen[key] = true
		}
	}
	if ctx.Err() != nil && len(result.ExhaustedBudgets) == 0 {
		result.ExhaustedBudgets = append(result.ExhaustedBudgets, "retrieval time")
	}
	return result
}

func parseTree(raw []byte, commit string) []candidate {
	var out []candidate
	for _, rec := range bytes.Split(raw, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		tab := bytes.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		fields := strings.Fields(string(rec[:tab]))
		if len(fields) < 3 {
			continue
		}
		p := bytes.Clone(rec[tab+1:])
		c := candidate{path: p, oid: fields[2], commit: commit, kind: classify(p), reason: "near changed paths"}
		if fields[1] != "blob" || fields[0] == "120000" || fields[0] == "160000" {
			c.skipReason = "symlink or submodule content not read"
		}
		out = append(out, c)
	}
	return out
}
func classify(p []byte) Kind {
	s := strings.ToLower(string(p))
	base := strings.ToLower(filepath.Base(s))
	if base == "go.mod" || base == "package.json" || base == "pyproject.toml" || base == "cargo.toml" || strings.HasSuffix(base, ".lock") {
		return Manifest
	}
	if strings.Contains(base, "readme") || strings.HasSuffix(base, ".md") || strings.Contains(s, "/docs/") {
		return Documentation
	}
	if strings.HasSuffix(base, "_test.go") || strings.Contains(base, "test") || strings.Contains(base, "spec") {
		return NearbyTest
	}
	return Source
}
func rank(p []byte, inv inventory.Inventory) int {
	k := classify(p)
	n := 0
	if k == Manifest || k == Documentation || k == NearbyTest {
		n = 0
	} else {
		n = 1
	}
	for _, f := range inv.Files {
		for _, q := range [][]byte{f.OldPath, f.NewPath} {
			if len(q) > 0 && filepath.Dir(string(q)) == filepath.Dir(string(p)) {
				return n
			}
		}
	}
	return n + 1
}
func isDeletedSide(p []byte, inv inventory.Inventory) bool {
	for _, f := range inv.Files {
		if bytes.Equal(f.OldPath, p) && len(f.NewPath) == 0 {
			return true
		}
	}
	return false
}
