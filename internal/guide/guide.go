// Package guide models the interpretation layer above a frozen inventory:
// functional guides whose sections reference immutable review unit IDs.
// Guides never own files; UnitFiles remains the authority for reading progress.
package guide

import (
	"errors"
	"strings"
	"time"

	"pr-review/internal/inventory"
)

type Status string

const (
	Generated   Status = "generated"
	Unavailable Status = "analysis_unavailable"
)

// Omitted records material deliberately kept out of an analysis request.
type Omitted struct {
	Path   []byte `json:"path"`
	Reason string `json:"reason"`
}

// Limits bounds one analysis request; a zero value means no analysis ran.
type Limits struct {
	Units     int           `json:"units,omitempty"`
	UnitBytes int           `json:"unit_bytes,omitempty"`
	Bytes     int           `json:"bytes,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
}

type Section struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	UnitIDs     []string `json:"unit_ids"`
}

type Item struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Sections    []Section `json:"sections"`
	Ungrouped   bool      `json:"ungrouped,omitempty"` // synthesized coverage guide, not model output
}

// Bundle is immutable snapshot data: the guides plus the provenance and scope
// needed to explain how they were produced and what was withheld.
type Bundle struct {
	Status               Status    `json:"status"`
	Items                []Item    `json:"items,omitempty"`
	Reason               string    `json:"reason,omitempty"` // set when Status is Unavailable
	Provider             string    `json:"provider,omitempty"`
	Model                string    `json:"model,omitempty"`
	PromptVersion        string    `json:"prompt_version,omitempty"`
	SchemaName           string    `json:"schema_name,omitempty"`
	SelectionFingerprint string    `json:"selection_fingerprint,omitempty"` // provider, model, endpoint, prompt and schema; never a credential
	InputDigest          string    `json:"input_digest,omitempty"`
	EvidenceIDs          []string  `json:"evidence_ids,omitempty"`
	WithheldPaths        []Omitted `json:"withheld_paths,omitempty"` // inputs excluded from the request
	Limits               Limits    `json:"limits,omitzero"`
}

// Fallback is the durable statement that no guides exist and why.
func Fallback(reason string) Bundle {
	if strings.TrimSpace(reason) == "" {
		reason = "analysis unavailable"
	}
	return Bundle{Status: Unavailable, Reason: reason}
}

// Validate keeps generated text from inventing review surface: sections may
// only reference known units, never twice, titles must be present, and a
// generated bundle must account for every unit of this inventory exactly once.
func Validate(b Bundle, inv inventory.Inventory) error {
	switch b.Status {
	case Unavailable:
		if len(b.Items) != 0 {
			return errors.New("unavailable analysis must retain no guides")
		}
		if strings.TrimSpace(b.Reason) == "" {
			return errors.New("unavailable analysis must state a reason")
		}
		return nil
	case Generated:
	default:
		return errors.New("unknown guide analysis status")
	}
	known := make(map[string]bool, len(inv.Units))
	for _, u := range inv.Units {
		known[u.ID] = true
	}
	seen := map[string]bool{}
	for _, item := range b.Items {
		if strings.TrimSpace(item.Title) == "" {
			return errors.New("guide title is empty")
		}
		for _, s := range item.Sections {
			if strings.TrimSpace(s.Title) == "" {
				return errors.New("guide section title is empty")
			}
			for _, id := range s.UnitIDs {
				if !known[id] {
					return errors.New("guide references a review unit outside this inventory")
				}
				if seen[id] {
					return errors.New("guide references a review unit more than once")
				}
				seen[id] = true
			}
		}
	}
	if len(seen) != len(known) {
		return errors.New("generated guides do not cover every review unit")
	}
	return nil
}
