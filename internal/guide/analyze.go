package guide

import (
	"context"
	"errors"
	"fmt"
	"strings"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
)

const (
	// PromptVersion and SchemaName are provenance: a stored bundle states the
	// contract that produced it, so guides from different prompts stay legible.
	PromptVersion = "guides-v2"
	SchemaName    = "pr_review_guides"

	maxItems     = 64
	maxSections  = 64
	maxTextBytes = 64 << 10
)

// Analyzer turns bounded request material into guide candidates. It is the
// only outbound boundary of this package; everything else is local.
type Analyzer interface {
	Analyze(context.Context, Input) (Bundle, error)
}

// Analyze is the whole failure policy in one place: a missing analyzer, an
// error, a timeout, an oversize answer, or an answer that cannot be reconciled
// with this inventory all produce an explained fallback rather than an error,
// because raw file review must never depend on analysis succeeding.
func Analyze(parent context.Context, a Analyzer, inv inventory.Inventory, in Input) (result Bundle) {
	if a == nil {
		return Fallback("analysis not requested")
	}
	ctx := parent
	if in.Limits.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, in.Limits.Duration)
		defer cancel()
	}
	candidate, err := a.Analyze(ctx, in)
	if in.Search != nil {
		defer func() {
			result.UploadedEvidence = candidate.UploadedEvidence
			result.RetrievedEvidence = append([]reviewcontext.Evidence(nil), candidate.UploadedEvidence...)
			result.RetrievalVersion = SearchVersion
			result.RetrievalIncomplete = candidate.RetrievalIncomplete || in.Search.Incomplete
			result.RetrievalOmissions = append([]reviewcontext.Omitted(nil), in.Search.Omissions...)
			material := in
			material.Evidence = append(append([]reviewcontext.Evidence(nil), in.Evidence...), candidate.UploadedEvidence...)
			result.InputDigest = material.digest()
			result.Limits = in.Limits
			result.WithheldPaths = in.Withheld
			result.EvidenceIDs = nil
			for _, e := range material.Evidence {
				result.EvidenceIDs = append(result.EvidenceIDs, e.EvidenceID)
			}
		}()
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Fallback(fmt.Sprintf("analysis timed out after %s", in.Limits.Duration))
		}
		return Fallback("analysis failed: " + stated(err.Error()))
	}
	if candidate.Status != Generated {
		return Fallback(stated(candidate.Reason))
	}
	if err := bounded(candidate); err != nil {
		return Fallback(err.Error())
	}
	b := candidate
	b.Items = attachFileMetadata(retained(candidate.Items, inv), inv)
	if len(b.Items) == 0 && len(inv.Units) > 0 {
		return Fallback("analysis produced no usable guides")
	}
	b.Items = append(b.Items, ungrouped(b.Items, inv)...)
	b.InputDigest, b.Limits, b.WithheldPaths = in.Digest, in.Limits, in.Withheld
	b.EvidenceIDs = nil
	for _, e := range in.Evidence {
		b.EvidenceIDs = append(b.EvidenceIDs, e.EvidenceID)
	}
	if b.PromptVersion == "" {
		b.PromptVersion = PromptVersion
	}
	if err := Validate(b, inv); err != nil {
		return Fallback("analysis discarded: " + stated(err.Error()))
	}
	return b
}

// attachFileMetadata keeps a file's structural record with the first guide
// section that already groups one of that file's other units. File metadata is
// required for a complete review, but has no patch text for an analyzer to
// reason about; requiring the model to name it otherwise produces a noisy
// ungrouped-only guide for every normally grouped file.
func attachFileMetadata(items []Item, inv inventory.Inventory) []Item {
	metadata := make(map[string]string, len(inv.Files))
	units := make(map[string]inventory.ReviewUnit, len(inv.Units))
	used := map[string]bool{}
	for _, u := range inv.Units {
		units[u.ID] = u
		if u.Kind == inventory.FileMetadata {
			metadata[u.FileChangeID] = u.ID
		}
	}
	for _, item := range items {
		for _, section := range item.Sections {
			for _, id := range section.UnitIDs {
				used[id] = true
			}
		}
	}
	for i := range items {
		for j := range items[i].Sections {
			section := &items[i].Sections[j]
			ids := make([]string, 0, len(section.UnitIDs)+1)
			for _, id := range section.UnitIDs {
				u := units[id]
				if u.Kind != inventory.FileMetadata {
					if id := metadata[u.FileChangeID]; id != "" && !used[id] {
						ids = append(ids, id)
						used[id] = true
					}
				}
				ids = append(ids, id)
			}
			section.UnitIDs = ids
		}
	}
	return items
}

// retained keeps only sections whose references this inventory can honour: an
// unknown or repeated unit ID costs that section alone, and its units reappear
// in the ungrouped guide, so one bad reference never hides a change.
func retained(items []Item, inv inventory.Inventory) []Item {
	known := make(map[string]bool, len(inv.Units))
	for _, u := range inv.Units {
		known[u.ID] = true
	}
	seen := map[string]bool{}
	var out []Item
	for _, item := range items {
		if item.Ungrouped {
			continue // coverage guides are synthesized here, never accepted from a model
		}
		if strings.TrimSpace(item.Title) == "" {
			out = append(out, item) // kept so Validate rejects the bundle rather than silently editing it
			continue
		}
		var kept []Section
		for _, s := range item.Sections {
			ids, ok := references(s.UnitIDs, known, seen)
			if !ok {
				continue
			}
			for _, id := range ids {
				seen[id] = true
			}
			kept = append(kept, Section{Title: s.Title, Description: s.Description, UnitIDs: ids})
		}
		if len(kept) == 0 {
			continue
		}
		out = append(out, Item{Title: item.Title, Description: item.Description, Sections: kept})
	}
	return out
}

func references(ids []string, known, seen map[string]bool) ([]string, bool) {
	out := make([]string, 0, len(ids))
	local := map[string]bool{}
	for _, id := range ids {
		if !known[id] || seen[id] || local[id] {
			return nil, false
		}
		local[id] = true
		out = append(out, id)
	}
	return out, true
}

// ungrouped is the coverage rule: partial grouping is allowed, unrepresented
// change is not.
func ungrouped(items []Item, inv inventory.Inventory) []Item {
	grouped := map[string]bool{}
	for _, item := range items {
		for _, s := range item.Sections {
			for _, id := range s.UnitIDs {
				grouped[id] = true
			}
		}
	}
	var ids []string
	for _, u := range inv.Units {
		if !grouped[u.ID] {
			ids = append(ids, u.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	const note = "Changes the analysis did not group. They are complete review surface, not lower priority."
	return []Item{{Title: "Ungrouped changes", Description: note, Ungrouped: true, Sections: []Section{{Title: "Ungrouped changes", Description: note, UnitIDs: ids}}}}
}

// bounded refuses an answer that is large enough to be a payload rather than an
// interpretation; it is checked before anything is copied into a snapshot.
func bounded(b Bundle) error {
	if len(b.Items) > maxItems {
		return errors.New("analysis response exceeds the guide limit")
	}
	text := 0
	for _, item := range b.Items {
		if len(item.Sections) > maxSections {
			return errors.New("analysis response exceeds the section limit")
		}
		text += len(item.Title) + len(item.Description)
		for _, s := range item.Sections {
			text += len(s.Title) + len(s.Description)
			for _, id := range s.UnitIDs {
				text += len(id)
			}
		}
		if text > maxTextBytes {
			return errors.New("analysis response exceeds the response byte limit")
		}
	}
	return nil
}

// stated keeps a failure reason short and renderable; the renderers escape it.
func stated(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "analyzer returned no guides"
	}
	if len(reason) > 200 {
		return reason[:200] + "..."
	}
	return reason
}
