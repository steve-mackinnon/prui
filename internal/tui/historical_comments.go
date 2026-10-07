package tui

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"prui/internal/commits"
	"prui/internal/review"
	"prui/internal/source"
)

// historicalFilesTarget is a display projection, never an outbound anchor.
// Only captured RIGHT-side source at the same path can supply this evidence.
func historicalFilesTarget(thread source.Discussion, s *review.Session) *source.ReviewCommentTarget {
	if s == nil || thread.Retained || thread.OriginalAnchor == nil || s.Commits == nil {
		return nil
	}
	t := *thread.OriginalAnchor
	meta := s.Inventory.Comparison.Metadata
	b := s.Commits
	if t.Identity != meta.Identity || t.CommitID != thread.OriginalCommitID || t.Side != "RIGHT" || t.SubjectType != "" || source.ValidateCommentCoordinates(t) != nil || b.HeadSHA != meta.HeadSHA || b.Status != commits.Captured || b.Composition == nil || b.Composition.Status != commits.Captured {
		return nil
	}
	blob := func(sha string) ([]byte, bool) {
		for _, e := range b.Composition.Trees[sha] {
			if string(e.Path) == t.Path && (e.Mode == "100644" || e.Mode == "100755") {
				data, ok := b.Composition.Blobs[e.OID]
				return data, ok && utf8.Valid(data) && !bytes.ContainsRune(data, 0)
			}
		}
		return nil, false
	}
	old, oldOK := blob(t.CommitID)
	head, headOK := blob(meta.HeadSHA)
	if !oldOK || !headOK {
		return nil
	}
	start := t.Line
	if t.StartLine > 0 {
		start = t.StartLine
	}
	oldLines := strings.Split(strings.TrimSuffix(string(old), "\n"), "\n")
	if t.Line > len(oldLines) {
		return nil
	}
	if !bytes.Equal(old, head) {
		code := strings.Join(oldLines[start-1:t.Line], "\n")
		if strings.TrimSpace(code) == "" {
			return nil
		}
		block := "\n" + code + "\n"
		unique := func(data []byte) int {
			text := "\n" + strings.TrimSuffix(string(data), "\n") + "\n"
			at := strings.Index(text, block)
			if at < 0 || strings.Contains(text[at+1:], block) {
				return -1
			}
			return strings.Count(text[:at], "\n") + 1
		}
		if unique(old) != start {
			return nil
		}
		mapped := unique(head)
		if mapped < 0 {
			return nil
		}
		t.Line += mapped - start
		if t.StartLine > 0 {
			t.StartLine += mapped - start
		}
	}
	t.CommitID = meta.HeadSHA
	if !commits.InventoryContainsTarget(s.Inventory.Files, s.Inventory.Units, s.Inventory.Patches, t) {
		return nil
	}
	return &t
}
