package context

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"pr-review/internal/privacy"
	"pr-review/internal/source"
)

// SearchFile contains an eligible complete pinned blob. It is local-only: tools
// return bounded Evidence excerpts rather than serializing the corpus.
type SearchFile struct {
	Revision string
	Evidence Evidence
}
type SearchResult struct {
	Status     string     `json:"status"`
	Evidence   []Evidence `json:"evidence,omitempty"`
	Incomplete bool       `json:"incomplete"`
}
type SearchCorpus struct {
	Files      []SearchFile
	Omissions  []Omitted
	Incomplete bool
}

const searchResultBytes = 16 << 10

// PrepareSearch never reads working files or resolves moving refs. The caller
// owns the object view and can close it as soon as preparation returns.
func PrepareSearch(parent context.Context, objects Objects, comparison source.PinnedComparison, policy privacy.Policy) (*SearchCorpus, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	c := &SearchCorpus{}
	entries, retained := 0, 0
	for _, rev := range []struct{ name, sha string }{{"merge_base", comparison.MergeBaseSHA}, {"head", comparison.Metadata.HeadSHA}} {
		if rev.sha == "" {
			return nil, errors.New("pinned source unavailable")
		}
		raw, err := objects.Git(ctx, 50<<20, "ls-tree", "-r", "-z", rev.sha, "--")
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		if err != nil {
			c.Incomplete = true
			c.Omissions = append(c.Omissions, Omitted{Reason: "pinned tree unavailable"})
			continue
		}
		candidates := parseTree(raw, rev.sha)
		if bytes.Count(raw, []byte{0}) != len(candidates) {
			c.Incomplete = true
			c.Omissions = append(c.Omissions, Omitted{Reason: "invalid tree entry"})
		}
		for _, f := range candidates {
			if parent.Err() != nil {
				return nil, parent.Err()
			}
			if ctx.Err() != nil || entries >= 10000 || retained >= 50<<20 {
				c.Incomplete = true
				c.Omissions = append(c.Omissions, Omitted{Reason: "source preparation budget exhausted"})
				return c, nil
			}
			entries++
			reason := f.skipReason
			if !utf8.Valid(f.path) {
				reason = "invalid text path"
			}
			if denied, why := policy.ExcludedPath(f.path); denied {
				reason = why
			}
			if reason != "" {
				c.Omissions = append(c.Omissions, Omitted{Path: bytes.Clone(f.path), Reason: reason})
				continue
			}
			b, err := objects.Blob(ctx, f.oid, 1<<20)
			if parent.Err() != nil {
				return nil, parent.Err()
			}
			if err != nil || len(b) > 1<<20 {
				c.Incomplete = true
				c.Omissions = append(c.Omissions, Omitted{Path: bytes.Clone(f.path), Reason: "blob unavailable or oversized"})
				continue
			}
			if !utf8.Valid(b) {
				reason = "invalid text content"
			}
			if denied, why := policy.ExcludedContent(b); denied {
				reason = why
			}
			if reason != "" {
				c.Omissions = append(c.Omissions, Omitted{Path: bytes.Clone(f.path), Reason: reason})
				continue
			}
			if len(b) > 50<<20-retained {
				c.Incomplete = true
				c.Omissions = append(c.Omissions, Omitted{Path: bytes.Clone(f.path), Reason: "source byte budget exhausted"})
				continue
			}
			retained += len(b)
			e := Evidence{Repository: comparison.Metadata.Identity.Repository, CommitSHA: rev.sha, BlobID: f.oid, Path: bytes.Clone(f.path), LineStart: 1, LineEnd: len(searchLines(b)), Kind: f.kind, Excerpt: bytes.Clone(b), RetrievalReason: "pinned source lookup", Relationship: "observed"}
			e.EvidenceID = searchEvidenceID(e)
			c.Files = append(c.Files, SearchFile{Revision: rev.name, Evidence: e})
		}
	}
	if ctx.Err() != nil {
		c.Incomplete = true
		c.Omissions = append(c.Omissions, Omitted{Reason: "source preparation deadline exhausted"})
	}
	sort.Slice(c.Files, func(i, j int) bool {
		a, b := c.Files[i], c.Files[j]
		if string(a.Evidence.Path) != string(b.Evidence.Path) {
			return string(a.Evidence.Path) < string(b.Evidence.Path)
		}
		return a.Revision < b.Revision
	})
	return c, nil
}

func validSearchRevision(revision string) bool { return revision == "head" || revision == "merge_base" }

// Search performs case-sensitive literal line matching. pathGlob uses Go's
// slash-separated path.Match syntax; an empty glob allows all eligible paths.
func (c *SearchCorpus) Search(ctx context.Context, pattern, pathGlob, revision string) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if pattern == "" || len(pattern) > 256 || !utf8.ValidString(pattern) || strings.ContainsAny(pattern, "\n\r\x00") || !validSearchRevision(revision) {
		return SearchResult{}, errors.New("invalid search arguments")
	}
	if _, err := path.Match(pathGlob, ""); err != nil {
		return SearchResult{}, errors.New("invalid search arguments")
	}
	result := SearchResult{Status: "no_matches", Incomplete: c.Incomplete}
	files := append([]SearchFile(nil), c.Files...)
	sort.SliceStable(files, func(i, j int) bool { return bytes.Compare(files[i].Evidence.Path, files[j].Evidence.Path) < 0 })
	matches := 0
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return SearchResult{}, err
		}
		if f.Revision != revision {
			continue
		}
		if pathGlob != "" {
			ok, _ := path.Match(pathGlob, string(f.Evidence.Path))
			if !ok {
				continue
			}
		}
		lines := searchLines(f.Evidence.Excerpt)
		for i, line := range lines {
			if err := ctx.Err(); err != nil {
				return SearchResult{}, err
			}
			if !bytes.Contains(line, []byte(pattern)) {
				continue
			}
			if matches >= 20 {
				if len(result.Evidence) == 0 {
					result.Status = "unavailable"
				}
				result.Incomplete = true
				return result, nil
			}
			matches++
			start, end := max(1, i-1), min(len(lines), i+3)
			e := searchExcerpt(f.Evidence, lines, start, end)
			result.Status = "ok"
			if !appendSearchEvidence(&result, e, i+1) {
				result.Incomplete = true
			}
		}
	}
	if matches > 0 && len(result.Evidence) == 0 {
		result.Status = "unavailable"
	}
	return result, nil
}

func (c *SearchCorpus) ReadLines(ctx context.Context, p string, start, end int, revision string) (SearchResult, error) {
	if err := ctx.Err(); err != nil {
		return SearchResult{}, err
	}
	if p == "" || !utf8.ValidString(p) || start < 1 || end < start || end-start >= 200 || !validSearchRevision(revision) {
		return SearchResult{}, errors.New("invalid read arguments")
	}
	result := SearchResult{Status: "unavailable", Incomplete: c.Incomplete}
	for _, f := range c.Files {
		if err := ctx.Err(); err != nil {
			return SearchResult{}, err
		}
		if f.Revision != revision || string(f.Evidence.Path) != p {
			continue
		}
		lines := searchLines(f.Evidence.Excerpt)
		if start > len(lines) {
			return result, nil
		}
		if end > len(lines) {
			end = len(lines)
		}
		result.Status = "ok"
		if !appendSearchEvidence(&result, searchExcerpt(f.Evidence, lines, start, end), start) {
			result.Incomplete = true
			result.Status = "unavailable"
		}
		return result, nil
	}
	return result, nil
}

func searchLines(b []byte) [][]byte {
	if len(b) == 0 {
		return nil
	}
	lines := bytes.SplitAfter(b, []byte{'\n'})
	if len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	return lines
}
func searchExcerpt(e Evidence, lines [][]byte, start, end int) Evidence {
	e.Path = bytes.Clone(e.Path)
	e.LineStart = start
	e.LineEnd = end
	e.Excerpt = bytes.Join(lines[start-1:end], nil)
	e.RetrievalReason = "pinned source lookup"
	e.Relationship = "observed"
	e.EvidenceID = searchEvidenceID(e)
	return e
}
func searchEvidenceID(e Evidence) string {
	// JSON encodes unambiguous field boundaries; include exact provenance and bytes.
	raw, _ := json.Marshal(struct {
		Repository, Commit, Blob string
		Path                     []byte
		Start, End               int
		Text                     []byte
	}{e.Repository, e.CommitSHA, e.BlobID, e.Path, e.LineStart, e.LineEnd, e.Excerpt})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func appendSearchEvidence(result *SearchResult, e Evidence, requiredLine int) bool {
	for {
		trial := *result
		trial.Evidence = append(append([]Evidence(nil), result.Evidence...), e)
		raw, _ := json.Marshal(trial)
		if len(raw) <= searchResultBytes {
			result.Evidence = trial.Evidence
			return true
		}
		result.Incomplete = true
		if e.LineEnd <= requiredLine {
			return false
		}
		lines := searchLines(e.Excerpt)
		e.Excerpt = bytes.Join(lines[:len(lines)-1], nil)
		e.LineEnd--
		e.EvidenceID = searchEvidenceID(e)
	}
}
