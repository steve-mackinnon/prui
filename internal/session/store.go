package session

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"prui/internal/commits"
	reviewcontext "prui/internal/context"
	"prui/internal/guide"
	"prui/internal/inventory"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const SchemaVersion = 1

type RevisionStatus string

const (
	Unchecked   RevisionStatus = "unchecked"
	Current     RevisionStatus = "current"
	Stale       RevisionStatus = "stale"
	CheckFailed RevisionStatus = "check_failed"
)

type Slice struct {
	FileID string
	Units  []int
}

type Snapshot struct {
	Checkout  []byte
	Inventory inventory.Inventory
	Slices    []Slice
	UnitFiles []int
	Context   reviewcontext.ContextBundle
	// PullRequestDescription distinguishes an unavailable body from a captured empty body.
	PullRequestDescription *string `json:"pull_request_description,omitempty"`
	// Commits holds optional frozen commit history for offline browsing.
	Commits *commits.Bundle `json:"commits,omitempty"`
	// Guides contains optional analysis for the frozen comparison.
	Guides *guide.Bundle `json:"guides,omitempty"`
	// DerivedFrom links a guided copy to its immutable source session.
	DerivedFrom string `json:"derived_from,omitempty"`
}

type State struct {
	SchemaVersion     int            `json:"schema_version"`
	ID                string         `json:"session_id"`
	SnapshotReference string         `json:"snapshot_reference"`
	ReviewedSliceIDs  []string       `json:"reviewed_slice_ids"`
	RevisionStatus    RevisionStatus `json:"revision_status"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Generation        uint64         `json:"generation"`
}

type Record struct {
	Snapshot
	State
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

var ErrRepositoryNotFound = errors.New("remembered repository not found")

type Repository struct {
	Repository string `json:"repository"`
	Checkout   string `json:"checkout"`
}

// GuideCacheKey identifies the immutable comparison a generated guide
// describes. Both base and head revisions matter because either can change the
// review units for the same pull request.
type GuideCacheKey struct {
	Repository string
	Number     int
	BaseSHA    string
	HeadSHA    string
}

func normalizeRepository(repository string) (string, error) {
	if !repositoryPattern.MatchString(repository) {
		return "", errors.New("invalid GitHub repository")
	}
	return strings.ToLower(repository), nil
}

func normalizeGuideCacheKey(key GuideCacheKey) (GuideCacheKey, error) {
	repository, err := normalizeRepository(key.Repository)
	if err != nil || key.Number <= 0 || !shaPattern.MatchString(key.BaseSHA) || !shaPattern.MatchString(key.HeadSHA) {
		return GuideCacheKey{}, errors.New("invalid guide cache key")
	}
	key.Repository = repository
	return key, nil
}

func guideCacheMatchesInventory(key GuideCacheKey, inv inventory.Inventory) bool {
	comparison := inv.Comparison.Metadata
	repository, err := normalizeRepository(comparison.Identity.Repository)
	return err == nil && repository == key.Repository && comparison.Identity.Number == key.Number && comparison.BaseSHA == key.BaseSHA && comparison.HeadSHA == key.HeadSHA
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func validate(r *Record) error {
	bad := errors.New("invalid session references or progress; original retained")
	if !r.Inventory.FullSource.Valid(r.Inventory.Files) {
		return bad
	}
	comparison := r.Inventory.Comparison.Metadata
	if r.SchemaVersion != SchemaVersion || r.Inventory.Comparison.InventoryID == "" || r.Generation == 0 ||
		comparison.Identity.Number <= 0 || !shaPattern.MatchString(comparison.BaseSHA) || !shaPattern.MatchString(comparison.HeadSHA) {
		return bad
	}
	if r.PullRequestDescription != nil && (!utf8.ValidString(*r.PullRequestDescription) || len(*r.PullRequestDescription) > 1<<20) {
		return bad
	}
	if !validateCommits(r.Commits, comparison.BaseSHA, comparison.HeadSHA) {
		return bad
	}
	switch r.RevisionStatus {
	case Unchecked, Current, Stale, CheckFailed:
	default:
		return bad
	}
	if len(r.UnitFiles) != len(r.Inventory.Units) || len(r.Slices) != len(r.Inventory.Files) {
		return bad
	}
	files, units, owned := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for i, slice := range r.Slices {
		if slice.FileID == "" || files[slice.FileID] || slice.FileID != r.Inventory.Files[i].ID || len(slice.Units) == 0 {
			return bad
		}
		files[slice.FileID] = true
		for _, u := range slice.Units {
			if u < 0 || u >= len(r.Inventory.Units) || owned[u] || r.UnitFiles[u] != i || r.Inventory.Units[u].FileChangeID != slice.FileID {
				return bad
			}
			owned[u] = true
		}
	}
	if len(owned) != len(r.Inventory.Units) {
		return bad
	}
	for _, u := range r.Inventory.Units {
		if u.ID == "" || units[u.ID] || u.InventoryID != r.Inventory.Comparison.InventoryID {
			return bad
		}
		units[u.ID] = true
		switch u.Kind {
		case inventory.TextHunk:
			if u.PatchReference == "" {
				return bad
			}
		case inventory.FileMetadata, inventory.Binary, inventory.Gitlink, inventory.Unavailable:
		default:
			return bad
		}
		if u.PatchReference != "" {
			if b, ok := r.Inventory.Patches[u.PatchReference]; !ok || digest(b) != u.PatchReference {
				return bad
			}
		}
	}
	seen := map[string]bool{}
	for _, id := range r.ReviewedSliceIDs {
		if !files[id] || seen[id] {
			return bad
		}
		seen[id] = true
	}
	if r.Guides != nil && guide.Validate(*r.Guides, r.Inventory) != nil {
		return bad
	}
	return nil
}
