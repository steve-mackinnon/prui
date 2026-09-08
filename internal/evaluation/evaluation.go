package evaluation

import (
	"fmt"
	"sort"

	"pr-review/internal/inventory"
	"pr-review/internal/plan"
)

type Case struct {
	Name          string        `json:"name"`
	UnitIDs       []string      `json:"unit_ids"`
	SavedProposal plan.Proposal `json:"saved_proposal"`
	EvidenceIDs   []string      `json:"evidence_ids"`
}

type Result struct {
	Name, Status string
	Owned, Total int
	Unassigned   int
	Warnings     []string
}

func Evaluate(c Case) Result {
	ids := map[string]bool{}
	for _, id := range c.UnitIDs {
		ids[id] = true
	}
	evidence := map[string]bool{}
	for _, id := range c.EvidenceIDs {
		evidence[id] = true
	}
	v, err := plan.Validate(c.SavedProposal, c.SavedProposal.InventoryID, ids, evidence)
	r := Result{Name: c.Name, Total: len(c.UnitIDs), Status: "valid"}
	if err != nil {
		r.Status = "fallback"
		r.Warnings = []string{err.Error()}
		return r
	}
	r.Unassigned = len(v.UnassignedUnitIDs)
	r.Owned = r.Total - r.Unassigned
	return r
}

func Run(cases []Case) []Result {
	results := make([]Result, 0, len(cases))
	for _, c := range cases {
		results = append(results, Evaluate(c))
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

func Summary(results []Result) string {
	valid, owned, total := 0, 0, 0
	for _, r := range results {
		total += r.Total
		owned += r.Owned
		if r.Status == "valid" {
			valid++
		}
	}
	return fmt.Sprintf("cases=%d valid=%d owned=%d/%d", len(results), valid, owned, total)
}

func Baseline(inv inventory.Inventory) plan.ValidatedPlan { return plan.FileFallback(inv) }
