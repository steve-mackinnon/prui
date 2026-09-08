package plan

import "testing"

func TestValidateOwnershipCitationsAndMissing(t *testing.T) {

	units := map[string]bool{"u1": true, "u2": true}
	evidence := map[string]bool{"e1": true}
	p := Proposal{InventoryID: "i", OverviewClaims: []Claim{{Text: "fact", EvidenceID: []string{"e1"}}}, Slices: []Slice{{SliceID: "s", Title: "one", UnitIDs: []string{"u1"}}}}
	v, err := Validate(p, "i", units, evidence)
	if err != nil || len(v.UnassignedUnitIDs) != 1 {
		t.Fatalf("validate=%+v err=%v", v, err)
	}
	p.Slices[0].UnitIDs = []string{"u1", "u1"}
	if _, err := Validate(p, "i", units, evidence); err == nil {
		t.Fatal("duplicate owner accepted")
	}
}

func TestValidateRejectsUnknownEvidenceAndCycle(t *testing.T) {
	p := Proposal{InventoryID: "i", OverviewClaims: []Claim{{Text: "fact", EvidenceID: []string{"missing"}}}}
	if _, err := Validate(p, "i", nil, nil); err == nil {
		t.Fatal("unknown evidence accepted")
	}
	p = Proposal{InventoryID: "i", Slices: []Slice{{SliceID: "a", Title: "a", DependsOnSliceIDs: []string{"b"}}, {SliceID: "b", Title: "b", DependsOnSliceIDs: []string{"a"}}}}
	if _, err := Validate(p, "i", nil, nil); err == nil {
		t.Fatal("cycle accepted")
	}
}

func FuzzValidateProposal(f *testing.F) {
	f.Add([]byte(`{"inventory_id":"i","slices":[]}`))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Decode(b) })
}
