package guide

import (
	"strings"
	"testing"

	"pr-review/internal/inventory"
)

func fixture(ids ...string) inventory.Inventory {
	inv := inventory.Inventory{Complete: true}
	for _, id := range ids {
		inv.Units = append(inv.Units, inventory.ReviewUnit{ID: id, InventoryID: "inventory", FileChangeID: "file", Kind: inventory.TextHunk})
	}
	inv.Comparison.InventoryID = "inventory"
	return inv
}

func generated(sections ...Section) Bundle {
	return Bundle{Status: Generated, Items: []Item{{Title: "Authentication flow", Description: "Adds a login endpoint.", Sections: sections}}}
}

func TestValidateReferences(t *testing.T) {
	inv := fixture("u1", "u2")
	cases := []struct {
		name   string
		bundle Bundle
		valid  bool
	}{
		{"complete coverage", generated(Section{Title: "Add login endpoint", UnitIDs: []string{"u1", "u2"}}), true},
		{"ordered sections", generated(Section{Title: "Add", UnitIDs: []string{"u2"}}, Section{Title: "Persist", UnitIDs: []string{"u1"}}), true},
		{"unknown unit", generated(Section{Title: "Add", UnitIDs: []string{"u1", "u2", "u3"}}), false},
		{"duplicate unit in one section", generated(Section{Title: "Add", UnitIDs: []string{"u1", "u1", "u2"}}), false},
		{"duplicate unit across sections", generated(Section{Title: "Add", UnitIDs: []string{"u1", "u2"}}, Section{Title: "Persist", UnitIDs: []string{"u2"}}), false},
		{"incomplete coverage", generated(Section{Title: "Add", UnitIDs: []string{"u1"}}), false},
		{"empty section title", generated(Section{Title: "  ", UnitIDs: []string{"u1", "u2"}}), false},
		{"unknown status", Bundle{Status: "partial"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.bundle, inv)
			if c.valid != (err == nil) {
				t.Fatalf("valid=%v err=%v", c.valid, err)
			}
		})
	}
	empty := generated(Section{Title: "Add", UnitIDs: []string{"u1", "u2"}})
	empty.Items[0].Title = ""
	if Validate(empty, inv) == nil {
		t.Fatal("empty guide title accepted")
	}
	if Validate(Bundle{Status: Generated}, fixture()) != nil {
		t.Fatal("empty comparison rejected")
	}
	if Validate(Bundle{Status: Generated}, inv) == nil {
		t.Fatal("guideless generated bundle hides units")
	}
}

func TestValidateFallback(t *testing.T) {
	inv := fixture("u1")
	b := Fallback("analysis not requested")
	if b.Status != Unavailable || len(b.Items) != 0 || b.Reason != "analysis not requested" {
		t.Fatal("fallback is not an empty, explained bundle")
	}
	if b.Provider != "" || b.Model != "" || b.InputDigest != "" || b.Limits != (Limits{}) {
		t.Fatal("fallback claims analysis provenance")
	}
	if err := Validate(b, inv); err != nil {
		t.Fatal("fallback rejected", err)
	}
	if Validate(Fallback(" "), inv) != nil || !strings.Contains(Fallback(" ").Reason, "unavailable") {
		t.Fatal("reasonless fallback lost its statement")
	}
	claimed := b
	claimed.Items = []Item{{Title: "Authentication flow", Sections: []Section{{Title: "Add", UnitIDs: []string{"u1"}}}}}
	if Validate(claimed, inv) == nil {
		t.Fatal("unavailable analysis kept guides")
	}
	silent := b
	silent.Reason = ""
	if Validate(silent, inv) == nil {
		t.Fatal("unavailable analysis accepted without a reason")
	}
}
