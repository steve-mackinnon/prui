package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
)

// MoveUnit transfers one canonical unit. An empty destination means unassigned.
// The returned plan is a new version; the input is never mutated.
func MoveUnit(p ValidatedPlan, unitID, destination string) (ValidatedPlan, error) {
	next := clonePlan(p)
	if unitID == "" {
		return ValidatedPlan{}, errors.New("unit ID is required")
	}
	from := -1
	for i := range next.Slices {
		if slices.Contains(next.Slices[i].UnitIDs, unitID) {
			if from >= 0 {
				return ValidatedPlan{}, fmt.Errorf("unit %q has duplicate owners", unitID)
			}
			from = i
		}
	}
	if from < 0 && !slices.Contains(next.UnassignedUnitIDs, unitID) {
		return ValidatedPlan{}, fmt.Errorf("unknown unit %q", unitID)
	}
	to := -1
	if destination != "" {
		for i := range next.Slices {
			if next.Slices[i].SliceID == destination {
				to = i
				break
			}
		}
		if to < 0 {
			return ValidatedPlan{}, fmt.Errorf("unknown destination slice %q", destination)
		}
	}
	if from == to && to >= 0 {
		return ValidatedPlan{}, errors.New("unit already belongs to destination slice")
	}
	if from >= 0 {
		next.Slices[from].UnitIDs = removeString(next.Slices[from].UnitIDs, unitID)
	}
	next.UnassignedUnitIDs = removeString(next.UnassignedUnitIDs, unitID)
	if to >= 0 {
		next.Slices[to].UnitIDs = append(next.Slices[to].UnitIDs, unitID)
	} else {
		next.UnassignedUnitIDs = append(next.UnassignedUnitIDs, unitID)
	}
	if from >= 0 && len(next.Slices[from].UnitIDs) == 0 {
		id := next.Slices[from].SliceID
		next.Slices = slices.Delete(next.Slices, from, from+1)
		for i := range next.Slices {
			next.Slices[i].DependsOnSliceIDs = removeString(next.Slices[i].DependsOnSliceIDs, id)
		}
	}
	return reversion(next)
}

// ReorderSlices changes only the advisory display order. Dependencies retain
// their labels and the order must be an exact permutation of current slices.
func ReorderSlices(p ValidatedPlan, order []string) (ValidatedPlan, error) {
	if len(order) != len(p.Slices) {
		return ValidatedPlan{}, errors.New("slice order is not a permutation")
	}
	byID := make(map[string]Slice, len(p.Slices))
	for _, s := range p.Slices {
		if _, ok := byID[s.SliceID]; ok {
			return ValidatedPlan{}, errors.New("duplicate slice ID")
		}
		byID[s.SliceID] = s
	}
	next := clonePlan(p)
	next.Slices = make([]Slice, 0, len(order))
	for _, id := range order {
		s, ok := byID[id]
		if !ok {
			return ValidatedPlan{}, fmt.Errorf("unknown slice %q", id)
		}
		delete(byID, id)
		next.Slices = append(next.Slices, s)
	}
	if len(byID) != 0 {
		return ValidatedPlan{}, errors.New("slice order is not a permutation")
	}
	return reversion(next)
}

func clonePlan(p ValidatedPlan) ValidatedPlan {
	n := p
	n.Slices = slices.Clone(p.Slices)
	for i := range n.Slices {
		n.Slices[i].UnitIDs = slices.Clone(p.Slices[i].UnitIDs)
		n.Slices[i].Claims = slices.Clone(p.Slices[i].Claims)
		n.Slices[i].DependsOnSliceIDs = slices.Clone(p.Slices[i].DependsOnSliceIDs)
		n.Slices[i].ReviewChecklist = slices.Clone(p.Slices[i].ReviewChecklist)
	}
	n.UnassignedUnitIDs = slices.Clone(p.UnassignedUnitIDs)
	return n
}

func removeString(values []string, value string) []string {
	return slices.DeleteFunc(values, func(v string) bool { return v == value })
}

func reversion(p ValidatedPlan) (ValidatedPlan, error) {
	p.UnassignedUnitIDs = append([]string(nil), p.UnassignedUnitIDs...)
	sortStrings(p.UnassignedUnitIDs)
	b := fmt.Sprintf("%s\x00%v", p.Version, p.Slices)
	h := sha256.Sum256([]byte(b))
	p.Version = "plan-v2-" + hex.EncodeToString(h[:])[:16]
	return p, nil
}

func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
