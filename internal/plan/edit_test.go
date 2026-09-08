package plan

import "testing"

func editableFixture() ValidatedPlan {
	return ValidatedPlan{Version: "plan-v1", InventoryID: "i", Slices: []Slice{
		{SliceID: "a", Title: "A", UnitIDs: []string{"u1"}, DependsOnSliceIDs: []string{"b"}},
		{SliceID: "b", Title: "B", UnitIDs: []string{"u2"}},
	}, UnassignedUnitIDs: []string{"u3"}}
}

func TestEditMoveUnitAtomicOwnershipAndEmptySliceRemoval(t *testing.T) {
	p, err := MoveUnit(editableFixture(), "u1", "b")
	if err != nil || len(p.Slices) != 1 || p.Slices[0].SliceID != "b" || len(p.Slices[0].UnitIDs) != 2 {
		t.Fatalf("move=%+v err=%v", p, err)
	}
	if len(p.Slices[0].DependsOnSliceIDs) != 0 || p.Version == "plan-v1" {
		t.Fatal("empty slice or version not handled")
	}
	if _, err := MoveUnit(editableFixture(), "u2", "missing"); err == nil {
		t.Fatal("unknown destination accepted")
	}
}

func TestEditReorderRequiresPermutationAndPreservesDependencies(t *testing.T) {
	p, err := ReorderSlices(editableFixture(), []string{"b", "a"})
	if err != nil || p.Slices[0].SliceID != "b" || p.Slices[1].DependsOnSliceIDs[0] != "b" {
		t.Fatalf("reorder=%+v err=%v", p, err)
	}
	if _, err := ReorderSlices(editableFixture(), []string{"a", "a"}); err == nil {
		t.Fatal("non-permutation accepted")
	}
}
