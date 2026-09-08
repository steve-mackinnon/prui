package plan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"

	"pr-review/internal/inventory"
)

const MaxText = 32 << 10

type Claim struct {
	Text       string   `json:"text"`
	EvidenceID []string `json:"evidence_ids"`
	Hypothesis bool     `json:"hypothesis"`
}

type Slice struct {
	SliceID           string   `json:"slice_id"`
	Title             string   `json:"title"`
	UnitIDs           []string `json:"unit_ids"`
	Claims            []Claim  `json:"claims"`
	ReviewChecklist   []string `json:"review_checklist"`
	DependsOnSliceIDs []string `json:"depends_on_slice_ids"`
	OrderingRationale string   `json:"ordering_rationale"`
}

type Proposal struct {
	InventoryID    string  `json:"inventory_id"`
	OverviewClaims []Claim `json:"overview_claims"`
	Slices         []Slice `json:"slices"`
}

type Provenance struct {
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	PromptHash        string `json:"prompt_hash"`
	ConfigurationHash string `json:"configuration_hash"`
	InputBytes        int    `json:"input_bytes"`
	OutputTokens      int    `json:"output_tokens"`
}

type ValidatedPlan struct {
	Version           string     `json:"version"`
	InventoryID       string     `json:"inventory_id"`
	Slices            []Slice    `json:"slices"`
	UnassignedUnitIDs []string   `json:"unassigned_unit_ids"`
	AnalysisStatus    string     `json:"analysis_status"`
	Warnings          []string   `json:"warnings"`
	Provenance        Provenance `json:"provenance"`
}

// FileFallback gives every unit a deterministic, source-truth-backed owner.
func FileFallback(inv inventory.Inventory) ValidatedPlan {
	if len(inv.Files) == 0 {
		ids := make([]string, 0, len(inv.Units))
		for _, u := range inv.Units {
			ids = append(ids, u.ID)
		}
		sort.Strings(ids)
		return ValidatedPlan{Version: "file-v1", InventoryID: inv.Comparison.InventoryID, UnassignedUnitIDs: ids, AnalysisStatus: "fallback", Warnings: []string{"provider analysis unavailable; units remain unassigned"}}
	}
	groups := make([]Slice, len(inv.Files))
	files := map[string]int{}
	for i, f := range inv.Files {
		groups[i] = Slice{SliceID: f.ID, Title: string(firstPath(f))}
		files[f.ID] = i
	}
	unitFiles := make([]string, len(inv.Units))
	for _, u := range inv.Units {
		i := files[u.FileChangeID]
		groups[i].UnitIDs = append(groups[i].UnitIDs, u.ID)
		unitFiles[i] = u.ID
	}
	return ValidatedPlan{Version: "file-v1", InventoryID: inv.Comparison.InventoryID, Slices: groups, AnalysisStatus: "fallback", Warnings: []string{"provider analysis unavailable; deterministic file grouping shown"}}
}
func firstPath(f inventory.FileChange) []byte {
	if len(f.NewPath) > 0 {
		return f.NewPath
	}
	return f.OldPath
}

func Decode(data []byte) (Proposal, error) {
	var p Proposal
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, fmt.Errorf("proposal schema: %w", err)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return p, errors.New("proposal schema: trailing data")
	}
	return p, nil
}

func Validate(p Proposal, inventoryID string, unitIDs, evidenceIDs map[string]bool) (ValidatedPlan, error) {
	if p.InventoryID == "" || p.InventoryID != inventoryID {
		return ValidatedPlan{}, errors.New("invalid inventory version")
	}
	seenSlices := map[string]bool{}
	owners := map[string]string{}
	checkClaims := func(cs []Claim) error {
		for _, c := range cs {
			if c.Text == "" || len(c.Text) > MaxText {
				return errors.New("invalid claim text")
			}
			if !c.Hypothesis && len(c.EvidenceID) == 0 {
				return errors.New("claim requires citation or hypothesis label")
			}
			seen := map[string]bool{}
			for _, id := range c.EvidenceID {
				if seen[id] || !evidenceIDs[id] {
					return fmt.Errorf("invalid evidence reference %q", id)
				}
				seen[id] = true
			}
		}
		return nil
	}
	if err := checkClaims(p.OverviewClaims); err != nil {
		return ValidatedPlan{}, err
	}
	for _, s := range p.Slices {
		if s.SliceID == "" || seenSlices[s.SliceID] || s.Title == "" || len(s.Title) > MaxText {
			return ValidatedPlan{}, errors.New("invalid or duplicate slice")
		}
		seenSlices[s.SliceID] = true
		if err := checkClaims(s.Claims); err != nil {
			return ValidatedPlan{}, err
		}
		for _, id := range s.UnitIDs {
			if !unitIDs[id] {
				return ValidatedPlan{}, fmt.Errorf("unknown unit %q", id)
			}
			if old, ok := owners[id]; ok {
				return ValidatedPlan{}, fmt.Errorf("unit %q owned by %s and %s", id, old, s.SliceID)
			}
			owners[id] = s.SliceID
		}
	}
	for _, s := range p.Slices {
		for _, dep := range s.DependsOnSliceIDs {
			if !seenSlices[dep] || dep == s.SliceID {
				return ValidatedPlan{}, errors.New("invalid slice dependency")
			}
		}
	}
	missing := make([]string, 0)
	for id := range unitIDs {
		if owners[id] == "" {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	if cycle(p.Slices) {
		return ValidatedPlan{}, errors.New("slice dependency cycle")
	}
	return ValidatedPlan{Version: "plan-v1", InventoryID: inventoryID, Slices: p.Slices, UnassignedUnitIDs: missing, AnalysisStatus: "valid"}, nil
}

func cycle(ss []Slice) bool {
	graph := map[string][]string{}
	for _, s := range ss {
		graph[s.SliceID] = s.DependsOnSliceIDs
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(n string) bool {
		if state[n] == 1 {
			return true
		}
		if state[n] == 2 {
			return false
		}
		state[n] = 1
		for _, d := range graph[n] {
			if visit(d) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for n := range graph {
		if visit(n) {
			return true
		}
	}
	return false
}

// CoalesceCycles merges each strongly connected component and preserves all units.
func CoalesceCycles(p Proposal) Proposal { // deterministic SCC coalescing for provider repairs
	graph := map[string][]string{}
	for _, s := range p.Slices {
		graph[s.SliceID] = s.DependsOnSliceIDs
	}
	next, ids, low := 0, map[string]int{}, map[string]int{}
	stack := []string{}
	on := map[string]bool{}
	components := [][]string{}
	var visit func(string)
	visit = func(v string) {
		next++
		ids[v], low[v] = next, next
		stack = append(stack, v)
		on[v] = true
		for _, w := range graph[v] {
			if ids[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if on[w] && ids[w] < low[v] {
				low[v] = ids[w]
			}
		}
		if low[v] == ids[v] {
			c := []string{}
			for {
				n := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				on[n] = false
				c = append(c, n)
				if n == v {
					break
				}
			}
			if len(c) > 1 {
				components = append(components, c)
			}
		}
	}
	for _, s := range p.Slices {
		if ids[s.SliceID] == 0 {
			visit(s.SliceID)
		}
	}
	for _, comp := range components {
		set := map[string]bool{}
		for _, id := range comp {
			set[id] = true
		}
		merged := Slice{SliceID: comp[0]}
		for _, s := range p.Slices {
			if set[s.SliceID] {
				if merged.Title == "" {
					merged.Title = s.Title
				} else {
					merged.Title += " + " + s.Title
				}
				merged.UnitIDs = append(merged.UnitIDs, s.UnitIDs...)
				merged.Claims = append(merged.Claims, s.Claims...)
				merged.ReviewChecklist = append(merged.ReviewChecklist, s.ReviewChecklist...)
				for _, d := range s.DependsOnSliceIDs {
					if !set[d] && !contains(merged.DependsOnSliceIDs, d) {
						merged.DependsOnSliceIDs = append(merged.DependsOnSliceIDs, d)
					}
				}
			}
		}
		p.Slices = removeMany(p.Slices, set)
		p.Slices = append(p.Slices, merged)
	}
	for i := range p.Slices {
		for j, dep := range p.Slices[i].DependsOnSliceIDs {
			for _, comp := range components {
				if contains(comp, dep) {
					p.Slices[i].DependsOnSliceIDs[j] = comp[0]
				}
			}
		}
	}
	sort.SliceStable(p.Slices, func(i, j int) bool { return p.Slices[i].SliceID < p.Slices[j].SliceID })
	return p
}
func contains(a []string, x string) bool {
	for _, v := range a {
		if v == x {
			return true
		}
	}
	return false
}
func remove(a []Slice, id string) []Slice {
	r := a[:0]
	for _, v := range a {
		if v.SliceID != id {
			r = append(r, v)
		}
	}
	return r
}
func removeMany(a []Slice, ids map[string]bool) []Slice {
	r := a[:0]
	for _, v := range a {
		if !ids[v.SliceID] {
			r = append(r, v)
		}
	}
	return r
}
