package guideeval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/inventory"
)

func testInventory(ids ...string) inventory.Inventory {
	inv := inventory.Inventory{Complete: true}
	for _, id := range ids {
		inv.Units = append(inv.Units, inventory.ReviewUnit{ID: id, InventoryID: "test", FileChangeID: "file", Kind: inventory.TextHunk})
	}
	return inv
}

func testBundle() guide.Bundle {
	return guide.Bundle{Status: guide.Generated, Items: []guide.Item{
		{Title: "Authentication flow", Sections: []guide.Section{{Title: "Validate credentials", UnitIDs: []string{"config", "handler"}}, {Title: "Exercise login", UnitIDs: []string{"test"}}}},
	}}
}

func testCorpus() []Case {
	return []Case{{
		Name:    "authentication-flow",
		UnitIDs: []string{"config", "handler", "test"},
		Expectations: []Expectation{{
			TitleKeywords:        []string{"authentication"},
			SectionTitleKeywords: []string{"credentials", "login"},
			UnitIDGroups:         [][]string{{"config", "handler"}, {"test"}},
		}},
	}}
}

func TestEvaluateReportsNotAvailableWithoutStoredGeneratedGuides(t *testing.T) {
	report := Evaluate(nil, testInventory("config"), nil)
	if report.Status != NotAvailable || len(report.Results) != 0 {
		t.Fatalf("report = %#v, want not_available with no results", report)
	}

	fallback := guide.Fallback("not requested")
	report = Evaluate(&fallback, testInventory("config"), nil)
	if report.Status != NotAvailable {
		t.Fatalf("fallback report status = %q, want %q", report.Status, NotAvailable)
	}
}

func TestEvaluatePassesStructuralAndMatchingSyntheticExpectation(t *testing.T) {
	report := Evaluate(ptr(testBundle()), testInventory("config", "handler", "test"), testCorpus())
	if report.Status != Passed || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	for _, result := range report.Results {
		if result.Status != Passed {
			t.Fatalf("result = %#v", result)
		}
	}
}

func TestEvaluateFailsStructuralBeforeSemanticChecks(t *testing.T) {
	bundle := testBundle()
	bundle.Items[0].Sections[0].UnitIDs = []string{"invented"}
	report := Evaluate(&bundle, testInventory("config", "handler", "test"), testCorpus())
	if report.Status != Failed || len(report.Results) != 1 || report.Results[0].Name != "structural" || report.Results[0].Status != Failed {
		t.Fatalf("report = %#v", report)
	}
}

func TestEvaluateNamesSemanticFailures(t *testing.T) {
	bundle := testBundle()
	bundle.Items[0].Title = "Request flow"
	bundle.Items[0].Sections[0].Title = "Validate input"
	bundle.Items[0].Sections[0].UnitIDs = []string{"config"}
	bundle.Items[0].Sections[1].UnitIDs = []string{"handler", "test"}
	report := Evaluate(&bundle, testInventory("config", "handler", "test"), testCorpus())
	if report.Status != Failed || len(report.Results) != 2 {
		t.Fatalf("report = %#v", report)
	}
	result := report.Results[1]
	if result.Name != "authentication-flow" || result.Status != Failed || len(result.Checks) != 4 {
		t.Fatalf("result = %#v", result)
	}
	for _, check := range result.Checks {
		if check.Status != Failed || !strings.Contains(check.Name, "authentication-flow") {
			t.Fatalf("check = %#v", check)
		}
	}
}

func TestEvaluateSkipsSemanticChecksWhenNoCorpusCaseApplies(t *testing.T) {
	report := Evaluate(ptr(testBundle()), testInventory("config", "handler", "test"), []Case{{Name: "other", UnitIDs: []string{"other"}}})
	if report.Status != Passed || len(report.Results) != 1 || report.Results[0].Name != "structural" {
		t.Fatalf("report = %#v", report)
	}
}

func TestLoadCorpusRejectsMalformedCases(t *testing.T) {
	cases := []string{
		`[{"name":"","unit_ids":["unit"]}]`,
		`[{"name":"no-expectation","unit_ids":["unit"],"expectations":[{}]}]`,
		`[{"name":"duplicate","unit_ids":["unit"]},{"name":"duplicate","unit_ids":["other"]}]`,
		`[{"name":"duplicate-unit","unit_ids":["unit","unit"]}]`,
		`[{"name":"bad-keyword","unit_ids":["unit"],"expectations":[{"title_keywords":[""]}]}]`,
		`[{"name":"bad-group","unit_ids":["unit"],"expectations":[{"unit_id_groups":[["unknown"]]}]}]`,
		`[] []`,
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			if _, err := LoadCorpus(strings.NewReader(input)); err == nil {
				t.Fatal("LoadCorpus accepted malformed corpus")
			}
		})
	}
}

func TestCorpusFixtureIsValid(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cases, err := LoadCorpus(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "authentication-flow" {
		t.Fatalf("cases = %#v", cases)
	}
}

func TestCuratedCorpusLoadsEmbeddedFixture(t *testing.T) {
	cases, err := CuratedCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || cases[0].Name != "authentication-flow" {
		t.Fatalf("cases = %#v", cases)
	}
}

func ptr(b guide.Bundle) *guide.Bundle { return &b }
