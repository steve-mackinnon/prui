package guide

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"time"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
)

// Defaults bound one analysis request. They sit well under the patch retention
// limits so a large PR degrades into a stated scope rather than a huge upload.
var Defaults = Limits{Units: 400, UnitBytes: 32 << 10, Bytes: 512 << 10, Duration: 60 * time.Second}

// Unit is one frozen review unit as it will be presented to an analyzer.
type Unit struct {
	ID    string
	Path  []byte
	Kind  inventory.Kind
	Patch []byte
}

// Input is the entire request material: nothing outside it may leave the
// machine, and Withheld states what the policy or the budgets kept back.
type Input struct {
	ComparisonID string
	Units        []Unit
	Evidence     []reviewcontext.Evidence
	Omissions    []reviewcontext.Omitted
	Withheld     []Omitted // changed paths or contents excluded from this request
	Limits       Limits
	Digest       string // SHA-256 over the assembled request material
}

// InputFrom re-applies the privacy policy to the pinned inventory. The raw
// inventory deliberately keeps every changed file; this boundary decides what
// a provider may see, so an excluded path or credential-like patch is withheld
// even though it stays fully reviewable locally.
func InputFrom(inv inventory.Inventory, c reviewcontext.ContextBundle, policy privacy.Policy, limits Limits) Input {
	in := Input{ComparisonID: inv.Comparison.InventoryID, Limits: limits, Omissions: append([]reviewcontext.Omitted(nil), c.OmittedPaths...)}
	files := make(map[string]inventory.FileChange, len(inv.Files))
	for _, f := range inv.Files {
		files[f.ID] = f
	}
	withhold := func(path []byte, reason string) {
		in.Withheld = append(in.Withheld, Omitted{Path: bytes.Clone(path), Reason: reason})
	}
	used := 0
	for _, u := range inv.Units {
		f := files[u.FileChangeID]
		path := unitPath(f)
		if excluded, reason := excludedPath(policy, f); excluded {
			withhold(path, reason)
			continue
		}
		patch := inv.Patches[u.PatchReference]
		if len(patch) > 0 {
			if excluded, reason := policy.ExcludedContent(patch); excluded {
				withhold(path, reason)
				continue
			}
		}
		switch {
		case limits.Units > 0 && len(in.Units) >= limits.Units:
			withhold(path, "request unit limit reached")
			continue
		case limits.UnitBytes > 0 && len(patch) > limits.UnitBytes:
			withhold(path, "unit exceeds request limit")
			continue
		case limits.Bytes > 0 && used+len(patch) > limits.Bytes:
			withhold(path, "request byte budget exhausted")
			continue
		}
		used += len(patch)
		in.Units = append(in.Units, Unit{ID: u.ID, Path: bytes.Clone(path), Kind: u.Kind, Patch: bytes.Clone(patch)})
	}
	// Evidence was already policy-filtered during retrieval; it is checked
	// again here because this is the boundary that actually uploads it, and
	// it shares the aggregate budget with the changed units.
	for _, e := range c.Evidence {
		if excluded, reason := policy.ExcludedPath(e.Path); excluded {
			in.Omissions = append(in.Omissions, reviewcontext.Omitted{Path: bytes.Clone(e.Path), Reason: reason})
			continue
		}
		if excluded, reason := policy.ExcludedContent(e.Excerpt); excluded {
			in.Omissions = append(in.Omissions, reviewcontext.Omitted{Path: bytes.Clone(e.Path), Reason: reason})
			continue
		}
		if limits.Bytes > 0 && used+len(e.Excerpt) > limits.Bytes {
			in.Omissions = append(in.Omissions, reviewcontext.Omitted{Path: bytes.Clone(e.Path), Reason: "guide request byte budget exhausted"})
			continue
		}
		used += len(e.Excerpt)
		in.Evidence = append(in.Evidence, e)
	}
	in.Digest = in.digest()
	return in
}

// excludedPath rejects a change when either side of a rename is excluded, so a
// secret cannot be uploaded by moving it in the same commit.
func excludedPath(policy privacy.Policy, f inventory.FileChange) (bool, string) {
	for _, p := range [][]byte{f.NewPath, f.OldPath} {
		if len(p) == 0 {
			continue
		}
		if excluded, reason := policy.ExcludedPath(p); excluded {
			return true, reason
		}
	}
	return false, ""
}

func unitPath(f inventory.FileChange) []byte {
	if len(f.NewPath) > 0 {
		return f.NewPath
	}
	return f.OldPath
}

// digest identifies the exact material of this request so a stored bundle can
// be tied back to what was sent without retaining the request itself.
func (in Input) digest() string {
	h := sha256.New()
	field(h, []byte(PromptVersion), []byte(in.ComparisonID))
	field(h, fmt.Appendf(nil, "%d;%d;%d;%s", in.Limits.Units, in.Limits.UnitBytes, in.Limits.Bytes, in.Limits.Duration))
	for _, u := range in.Units {
		field(h, []byte(u.ID), u.Path, []byte(u.Kind), u.Patch)
	}
	for _, e := range in.Evidence {
		field(h, []byte(e.EvidenceID), e.Path, e.Excerpt)
	}
	for _, o := range in.Withheld {
		field(h, o.Path, []byte(o.Reason))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func field(h hash.Hash, parts ...[]byte) {
	for _, p := range parts {
		_, _ = fmt.Fprintf(h, "%d:", len(p))
		h.Write(p)
	}
}
