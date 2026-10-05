package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"prui/internal/commits"
	"prui/internal/inventory"
)

var commitModePattern = regexp.MustCompile(`^[0-7]{6}$`)
var commitChangePattern = regexp.MustCompile(`^([AMDT]|R[0-9]{1,3})$`)

// validateCommits checks frozen history independently of PR progress references.
func validateCommits(b *commits.Bundle, base, head string) bool {
	if b == nil {
		return true
	}
	if !commits.ValidateComposition(b) {
		return false
	}
	if b.BaseSHA != base || b.HeadSHA != head || len(b.Entries) > 100 || !boundedCommitText(b.Reason, 4096) {
		return false
	}
	if b.Status == commits.Unavailable {
		return b.Reason != "" && !b.Complete && len(b.Entries) == 0
	}
	if b.Status != commits.Captured || b.Reason != "" || (len(b.Entries) == 100 && b.Complete) {
		return false
	}
	size, lines := len(base)+len(head), 0
	seen := map[string]bool{}
	for _, entry := range b.Entries {
		if !shaPattern.MatchString(entry.SHA) || seen[entry.SHA] || !boundedCommitText(entry.Subject, 4096) || strings.ContainsAny(entry.Subject, "\r\n") || !boundedCommitText(entry.Author, 256) || !boundedCommitText(entry.Reason, 4096) || len(entry.Parents) > 10000 {
			return false
		}
		seen[entry.SHA] = true
		parents := map[string]bool{}
		for _, p := range entry.Parents {
			if !shaPattern.MatchString(p) || p == entry.SHA || parents[p] {
				return false
			}
			parents[p] = true
			size += len(p)
		}
		size += len(entry.SHA) + len(entry.Subject) + len(entry.Author) + len(entry.Reason)
		switch entry.Status {
		case commits.Unavailable:
			if entry.Reason == "" || entry.Diff != nil {
				return false
			}
		case commits.Captured:
			if entry.Reason != "" || entry.Diff == nil || !validateCommitDiff(entry.Diff, &size, &lines) {
				return false
			}
		default:
			return false
		}
		if size > 50<<20 || lines > 100000 {
			return false
		}
	}
	if b.Composition != nil {
		encoded, err := json.Marshal(b.Composition)
		if err != nil || size+len(encoded) > 50<<20 {
			return false
		}
	}
	return true
}

func boundedCommitText(s string, max int) bool { return len(s) <= max && utf8.ValidString(s) }

func validateCommitDiff(d *commits.Diff, size, lines *int) bool {
	if len(d.Files) > 10000 || len(d.Units) > 100000 || len(d.Problems) > 10000 || (d.Complete && len(d.Problems) > 0) {
		return false
	}
	files, owned, units, refs := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, f := range d.Files {
		if !shaPattern.MatchString(f.OldOID) || !shaPattern.MatchString(f.NewOID) || !commitModePattern.MatchString(f.OldMode) || !commitModePattern.MatchString(f.NewMode) || !commitChangePattern.MatchString(f.Status) || !utf8.ValidString(f.ID) || f.ID == "" || files[f.ID] || len(f.ID) > 256 || bytes.IndexByte(f.OldPath, 0) >= 0 || bytes.IndexByte(f.NewPath, 0) >= 0 {
			return false
		}
		files[f.ID] = true
		*size += len(f.ID) + len(f.OldPath) + len(f.NewPath) + len(f.OldOID) + len(f.NewOID) + len(f.OldMode) + len(f.NewMode) + len(f.Status)
	}
	inventoryID := ""
	for _, u := range d.Units {
		if !utf8.ValidString(u.ID) || !utf8.ValidString(u.InventoryID) || u.ID == "" || units[u.ID] || !files[u.FileChangeID] || u.InventoryID == "" || len(u.ID) > 256 || len(u.InventoryID) > 256 || !boundedCommitText(u.UnavailableReason, 4096) || u.OldRange.Start < 0 || u.OldRange.Count < 0 || u.NewRange.Start < 0 || u.NewRange.Count < 0 {
			return false
		}
		if inventoryID == "" {
			inventoryID = u.InventoryID
		}
		if u.InventoryID != inventoryID {
			return false
		}
		units[u.ID] = true
		owned[u.FileChangeID] = true
		switch u.Kind {
		case inventory.TextHunk:
			if u.PatchReference == "" || u.UnavailableReason != "" {
				return false
			}
		case inventory.FileMetadata, inventory.Binary, inventory.Gitlink:
			if u.UnavailableReason != "" || u.PatchReference != "" {
				return false
			}
		case inventory.Unavailable:
			if d.Complete || u.UnavailableReason == "" || u.PatchReference != "" {
				return false
			}
		default:
			return false
		}
		if u.PatchReference != "" {
			patch, ok := d.Patches[u.PatchReference]
			if !ok || digest(patch) != u.PatchReference {
				return false
			}
			refs[u.PatchReference] = true
		}
		*size += len(u.ID) + len(u.InventoryID) + len(u.FileChangeID) + len(u.Kind) + len(u.PatchReference) + len(u.UnavailableReason)
	}
	if len(owned) != len(files) || len(refs) != len(d.Patches) {
		return false
	}
	for ref, patch := range d.Patches {
		*size += len(ref) + len(patch)
		*lines += bytes.Count(patch, []byte{'\n'})
		if len(patch) > 0 && patch[len(patch)-1] != '\n' {
			*lines += 1
		}
	}
	for _, problem := range d.Problems {
		if problem == "" || !boundedCommitText(problem, 4096) {
			return false
		}
		*size += len(problem)
	}
	return *size <= 50<<20 && *lines <= 100000
}

// BoundCommitPayload retains as much commit material as fits in the final source
// codec budget. Call before saving a newly captured snapshot. Main PR evidence
// is never pruned. A legacy snapshot with no bundle is unchanged.
func BoundCommitPayload(snapshot *Snapshot) error {
	return boundCommitPayload(snapshot, sqliteMaxPayloadBytes)
}

func boundCommitPayload(snapshot *Snapshot, limit int) error {
	// Presentation metadata must never displace reviewable source. Copy nested
	// commit values before dropping it so cached snapshots remain immutable.
	full, err := json.Marshal(sourcePayload(*snapshot))
	if err != nil {
		return err
	}
	if len(full) > limit {
		snapshot.Inventory.Syntax = nil
		if snapshot.Commits != nil {
			bundle := *snapshot.Commits
			bundle.Entries = append([]commits.Entry(nil), bundle.Entries...)
			for i := range bundle.Entries {
				if bundle.Entries[i].Diff != nil {
					diff := *bundle.Entries[i].Diff
					diff.Syntax = nil
					bundle.Entries[i].Diff = &diff
				}
			}
			snapshot.Commits = &bundle
		}
	}
	// Commits is the last optional source field. Encoding the baseline once plus
	// its comma/key and bundle JSON gives the exact final codec size.
	baseline := sourcePayload(*snapshot)
	baseline.Commits = nil
	mainBytes, err := json.Marshal(baseline)
	if err != nil {
		return err
	}
	if len(mainBytes) > limit {
		return errors.New("session source exceeds storage limit")
	}
	if snapshot.Commits == nil {
		return nil
	}
	bundleBytes, err := json.Marshal(snapshot.Commits)
	if err != nil {
		return err
	}
	sourceSize := len(mainBytes) + len(`,"commits":`) + len(bundleBytes)
	if sourceSize <= limit {
		return nil
	}

	if snapshot.Commits.Composition != nil {
		bundle := *snapshot.Commits
		bundle.Composition = &commits.Composition{Status: commits.Unavailable, Reason: "commit source storage limit"}
		snapshot.Commits = &bundle
		bundleBytes, err = json.Marshal(snapshot.Commits)
		if err != nil {
			return err
		}
		sourceSize = len(mainBytes) + len(`,"commits":`) + len(bundleBytes)
		if sourceSize <= limit {
			return nil
		}
	}
	// Copy entries before pruning: captured snapshots and cached source remain immutable.
	bundle := *snapshot.Commits
	bundle.Entries = append([]commits.Entry(nil), bundle.Entries...)
	snapshot.Commits = &bundle
	for i := len(bundle.Entries) - 1; i >= 0; i-- {
		entry := &bundle.Entries[i]
		if entry.Diff == nil {
			continue
		}
		oldBytes, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		entry.Diff = nil
		entry.Status = commits.Unavailable
		entry.Reason = "commit source storage limit"
		newBytes, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		// Replacing one entry changes no array separators or bundle flags.
		sourceSize += len(newBytes) - len(oldBytes)
		if sourceSize <= limit {
			return nil
		}
	}
	snapshot.Commits = &commits.Bundle{BaseSHA: bundle.BaseSHA, HeadSHA: bundle.HeadSHA, Status: commits.Unavailable, Reason: "commit source storage limit"}
	unavailableBytes, err := json.Marshal(snapshot.Commits)
	if err != nil {
		return err
	}
	if len(mainBytes)+len(`,"commits":`)+len(unavailableBytes) <= limit {
		return nil
	}
	// If even a failure notice cannot fit, preserve the readable main review.
	snapshot.Commits = nil
	return nil
}
