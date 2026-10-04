package incremental

import (
	"bytes"
	"prui/internal/commits"
	"prui/internal/inventory"
	"prui/internal/source"
	"reflect"
	"strings"
)

func sameRepositories(a, b source.Metadata) bool {
	return a.Identity == b.Identity && a.BaseRepository == b.BaseRepository && a.HeadRepository == b.HeadRepository
}

// unchanged proves both sides of the entire PR file change, including modes,
// paths and every raw unit. Equal displayed text or a path is never proof.
func unchanged(a, b inventory.Inventory, x, y inventory.FileChange) bool {
	if !a.Complete || !b.Complete || !sameRepositories(a.Comparison.Metadata, b.Comparison.Metadata) ||
		strings.HasPrefix(x.Status, "R") || strings.HasPrefix(y.Status, "R") ||
		a.Comparison.DiffSettings != b.Comparison.DiffSettings || a.Comparison.InventoryVersion != b.Comparison.InventoryVersion ||
		x.Status != y.Status || x.OldOID != y.OldOID || x.NewOID != y.NewOID || x.OldMode != y.OldMode || x.NewMode != y.NewMode ||
		!bytes.Equal(x.OldPath, y.OldPath) || !bytes.Equal(x.NewPath, y.NewPath) {
		return false
	}
	units := func(inv inventory.Inventory, f inventory.FileChange) []inventory.ReviewUnit {
		var out []inventory.ReviewUnit
		for _, u := range inv.Units {
			if u.FileChangeID == f.ID {
				u.ID = ""
				u.InventoryID = ""
				u.FileChangeID = ""
				out = append(out, u)
			}
		}
		return out
	}
	xs, ys := units(a, x), units(b, y)
	if len(xs) == 0 || !reflect.DeepEqual(xs, ys) {
		return false
	}
	for _, u := range xs {
		if u.Kind == inventory.Unavailable {
			return false
		}
		if u.PatchReference != "" {
			xp, xok := a.Patches[u.PatchReference]
			yp, yok := b.Patches[u.PatchReference]
			if !xok || !yok || !bytes.Equal(xp, yp) {
				return false
			}
		}
	}
	return true
}

// Carry returns only destination slice IDs with verified unchanged content.
// The captured prior identity must match both snapshots exactly.
func Carry(old, next inventory.Inventory, reviewed []string, b *Bundle) []string {
	var out []string
	if b == nil || b.Status != "captured" || b.Diff == nil || !b.Diff.Complete ||
		!source.SamePinnedRevision(b.Previous, old.Comparison.Metadata) || !source.SamePinnedRevision(b.Current, next.Comparison.Metadata) {
		return out
	}
	// File IDs index candidates only; the complete proof is still mandatory.
	read := make(map[string]bool, len(reviewed))
	for _, id := range reviewed {
		read[id] = true
	}
	files := make(map[string]inventory.FileChange, len(old.Files))
	for _, f := range old.Files {
		if read[f.ID] {
			files[f.ID] = f
		}
	}
	indexUnits := func(inv inventory.Inventory) map[string][]inventory.ReviewUnit {
		m := make(map[string][]inventory.ReviewUnit, len(inv.Files))
		for _, u := range inv.Units {
			m[u.FileChangeID] = append(m[u.FileChangeID], u)
		}
		return m
	}
	oldUnits, nextUnits := indexUnits(old), indexUnits(next)
	for _, y := range next.Files {
		if x, ok := files[y.ID]; ok {
			a, c := old, next
			a.Units = oldUnits[x.ID]
			c.Units = nextUnits[y.ID]
			if unchanged(a, c, x, y) {
				out = append(out, y.ID)
			}
		}
	}
	return out
}

type AnchorKind string

const (
	AnchorUnchanged   AnchorKind = "unchanged; explicit recreation required"
	AnchorChanged     AnchorKind = "changed; select a new target"
	AnchorRenamed     AnchorKind = "renamed; select a new target"
	AnchorRemoved     AnchorKind = "deleted or absent from new PR diff"
	AnchorUnavailable AnchorKind = "unavailable; original retained"
	AnchorHistorical  AnchorKind = "historical or reply; inspect original snapshot"
	AnchorUncertain   AnchorKind = "delivery uncertain; check outcome in original snapshot"
)

// AnchorOutcome is payload-agnostic so future suggestions can retain their
// opaque payload unchanged. Proposed is advisory and grants no write authority.
type AnchorOutcome struct {
	Kind     AnchorKind
	Original source.ReviewCommentTarget
	Proposed *source.ReviewCommentTarget
}

func Assess(old, next inventory.Inventory, t source.ReviewCommentTarget, uncertain bool) AnchorOutcome {
	o := AnchorOutcome{Kind: AnchorUnavailable, Original: t}
	if uncertain {
		o.Kind = AnchorUncertain
		return o
	}
	if t.CommitID != old.Comparison.Metadata.HeadSHA {
		o.Kind = AnchorHistorical
		return o
	}
	if t.Identity != old.Comparison.Metadata.Identity || !sameRepositories(old.Comparison.Metadata, next.Comparison.Metadata) || !old.Complete || !next.Complete ||
		!commits.InventoryContainsTarget(old.Files, old.Units, old.Patches, t) {
		return o
	}
	for _, x := range old.Files {
		path := x.NewPath
		if t.Side == "LEFT" || x.Status == "D" {
			path = x.OldPath
		}
		if string(path) != t.Path {
			continue
		}
		// Match the full unchanged file identity before considering side paths:
		// a LEFT path may also be the source path of a separate rename record.
		candidate := t
		candidate.CommitID = next.Comparison.Metadata.HeadSHA
		for _, y := range next.Files {
			if unchanged(old, next, x, y) && commits.InventoryContainsTarget(next.Files, next.Units, next.Patches, candidate) {
				o.Kind = AnchorUnchanged
				o.Proposed = &candidate
				return o
			}
		}
		for _, y := range next.Files {
			newPath := y.NewPath
			if t.Side == "LEFT" || y.Status == "D" {
				newPath = y.OldPath
			}
			if string(newPath) == t.Path {
				if t.Side == "LEFT" && strings.HasPrefix(y.Status, "R") && !bytes.Equal(x.NewPath, y.NewPath) {
					continue
				}
				o.Kind = AnchorChanged
				switch {
				case y.Status == "D" && x.Status != "D" && t.Side != "LEFT":
					o.Kind = AnchorRemoved
				case strings.HasPrefix(y.Status, "R") && !bytes.Equal(x.NewPath, y.NewPath):
					o.Kind = AnchorRenamed
				}
				return o
			}
		}
		// Exact paths take precedence over rename evidence. Equal blobs alone
		// can be copies or independent identical files, never proof of a rename.
		for _, y := range next.Files {
			if strings.HasPrefix(y.Status, "R") && bytes.Equal(y.OldPath, path) && !bytes.Equal(y.NewPath, path) {
				o.Kind = AnchorRenamed
				return o
			}
		}
		o.Kind = AnchorRemoved
		return o
	}
	return o
}
