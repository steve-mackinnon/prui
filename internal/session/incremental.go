package session

import (
	"encoding/hex"
	"prui/internal/incremental"
	"prui/internal/source"
)

func validateIncremental(b *incremental.Bundle, m source.Metadata) bool {
	if b == nil {
		return true
	}
	p := b.Previous
	reference, err := hex.DecodeString(b.PreviousReference)
	if err != nil || len(reference) != 32 {
		return false
	}
	if !idPattern.MatchString(b.PreviousID) || b.PreviousGeneration == 0 || len(b.PreviousReference) != 64 || !source.SamePinnedRevision(b.Current, m) ||
		p.Identity != m.Identity || !shaPattern.MatchString(p.BaseSHA) || !shaPattern.MatchString(p.HeadSHA) ||
		!repositoryPattern.MatchString(p.BaseRepository) || !repositoryPattern.MatchString(p.HeadRepository) {
		return false
	}
	if b.Status == "unavailable" {
		return b.Reason != "" && boundedCommitText(b.Reason, 4096) && b.Diff == nil
	}
	if b.Status != "captured" || b.Reason != "" || b.Diff == nil || p.BaseRepository != m.BaseRepository || p.HeadRepository != m.HeadRepository {
		return false
	}
	switch b.Relation {
	case "same head", "forward push", "rewritten history (rebase or force push)":
	default:
		return false
	}
	size, lines := 0, 0
	return validateCommitDiff(b.Diff, &size, &lines) && size <= 50<<20 && lines <= 100000
}
