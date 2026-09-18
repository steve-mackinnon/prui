// Package guideeval evaluates stored guide bundles without creating guides or
// contacting an analysis provider. It treats the frozen inventory as the
// source of truth and uses only deterministic synthetic expectations.
package guideeval

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"pr-review/internal/guide"
	"pr-review/internal/inventory"
)

// curatedCorpus is intentionally synthetic. It is embedded so command users
// evaluate the same fixed corpus regardless of their working directory.
//
//go:embed testdata/corpus.json
var curatedCorpus []byte

type Status string

const (
	NotAvailable Status = "not_available"
	Passed       Status = "passed"
	Failed       Status = "failed"
)

// Check records one deterministic assertion and its explanatory detail.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Result groups related checks. Structural validation always appears first.
type Result struct {
	Name   string  `json:"name"`
	Status Status  `json:"status"`
	Checks []Check `json:"checks"`
}

// Report is the full outcome for one stored guide bundle.
type Report struct {
	Status  Status   `json:"status"`
	Results []Result `json:"results,omitempty"`
}

// Case applies when its unit IDs exactly match the frozen inventory. Cases
// intentionally use synthetic IDs and expectations only.
type Case struct {
	Name         string        `json:"name"`
	UnitIDs      []string      `json:"unit_ids"`
	Expectations []Expectation `json:"expectations,omitempty"`
}

// Expectation makes only explicit assertions about one guide item's title,
// its section titles, and exact section unit groups.
type Expectation struct {
	TitleKeywords        []string   `json:"title_keywords,omitempty"`
	SectionTitleKeywords []string   `json:"section_title_keywords,omitempty"`
	UnitIDGroups         [][]string `json:"unit_id_groups,omitempty"`
}

// LoadCorpus parses and validates a synthetic semantic corpus.
func LoadCorpus(r io.Reader) ([]Case, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var cases []Case
	if err := decoder.Decode(&cases); err != nil {
		return nil, fmt.Errorf("decode guide corpus: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("decode guide corpus: multiple JSON values")
		}
		return nil, fmt.Errorf("decode guide corpus: %w", err)
	}
	if err := validateCorpus(cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// CuratedCorpus returns the built-in synthetic semantic expectations.
func CuratedCorpus() ([]Case, error) { return LoadCorpus(strings.NewReader(string(curatedCorpus))) }

// Evaluate validates a stored generated bundle first, then evaluates every
// corpus case whose unit IDs exactly match the inventory. It never generates
// guides or invokes a provider.
func Evaluate(bundle *guide.Bundle, inv inventory.Inventory, cases []Case) Report {
	if bundle == nil || bundle.Status != guide.Generated {
		return Report{Status: NotAvailable}
	}
	if err := guide.Validate(*bundle, inv); err != nil {
		return Report{Status: Failed, Results: []Result{{
			Name: "structural", Status: Failed,
			Checks: []Check{{Name: "guide.validate", Status: Failed, Detail: err.Error()}},
		}}}
	}
	results := []Result{{Name: "structural", Status: Passed, Checks: []Check{{Name: "guide.validate", Status: Passed}}}}
	for _, c := range cases {
		if sameIDs(c.UnitIDs, inventoryIDs(inv)) {
			results = append(results, evaluateCase(c, *bundle))
		}
	}
	report := Report{Status: Passed, Results: results}
	for _, result := range results {
		if result.Status == Failed {
			report.Status = Failed
			break
		}
	}
	return report
}

func evaluateCase(c Case, bundle guide.Bundle) Result {
	checks := make([]Check, 0, len(c.Expectations)*3)
	for i, expected := range c.Expectations {
		prefix := c.Name
		if len(c.Expectations) > 1 {
			prefix = fmt.Sprintf("%s/%d", prefix, i+1)
		}
		matching := matchingItems(expected.TitleKeywords, bundle.Items)
		checks = append(checks, keywordCheck(prefix+"/title-keywords", expected.TitleKeywords, itemTitles(matching)))
		checks = append(checks, keywordCheck(prefix+"/section-title-keywords", expected.SectionTitleKeywords, itemSectionTitles(matching)))
		for groupIndex, group := range expected.UnitIDGroups {
			checks = append(checks, groupCheck(fmt.Sprintf("%s/unit-id-group-%d", prefix, groupIndex+1), group, matching))
		}
	}
	result := Result{Name: c.Name, Status: Passed, Checks: checks}
	for _, check := range checks {
		if check.Status == Failed {
			result.Status = Failed
			break
		}
	}
	return result
}

func keywordCheck(name string, keywords, titles []string) Check {
	if len(keywords) == 0 {
		return Check{Name: name, Status: Passed}
	}
	text := strings.ToLower(strings.Join(titles, "\n"))
	for _, keyword := range keywords {
		if !strings.Contains(text, strings.ToLower(keyword)) {
			return Check{Name: name, Status: Failed, Detail: fmt.Sprintf("missing keyword %q", keyword)}
		}
	}
	return Check{Name: name, Status: Passed}
}

func groupCheck(name string, want []string, items []guide.Item) Check {
	for _, item := range items {
		for _, section := range item.Sections {
			if sameIDs(want, section.UnitIDs) {
				return Check{Name: name, Status: Passed}
			}
		}
	}
	return Check{Name: name, Status: Failed, Detail: fmt.Sprintf("missing section group %s", strings.Join(want, ", "))}
}

func matchingItems(keywords []string, items []guide.Item) []guide.Item {
	if len(keywords) == 0 {
		return items
	}
	var matches []guide.Item
	for _, item := range items {
		title := strings.ToLower(item.Title)
		matched := true
		for _, keyword := range keywords {
			if !strings.Contains(title, strings.ToLower(keyword)) {
				matched = false
				break
			}
		}
		if matched {
			matches = append(matches, item)
		}
	}
	return matches
}

func itemTitles(items []guide.Item) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, item.Title)
	}
	return titles
}

func itemSectionTitles(items []guide.Item) []string {
	var titles []string
	for _, item := range items {
		for _, section := range item.Sections {
			titles = append(titles, section.Title)
		}
	}
	return titles
}

func inventoryIDs(inv inventory.Inventory) []string {
	ids := make([]string, 0, len(inv.Units))
	for _, unit := range inv.Units {
		ids = append(ids, unit.ID)
	}
	return ids
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	left := append([]string(nil), a...)
	right := append([]string(nil), b...)
	sort.Strings(left)
	sort.Strings(right)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateCorpus(cases []Case) error {
	names := map[string]bool{}
	for _, c := range cases {
		if strings.TrimSpace(c.Name) == "" {
			return fmt.Errorf("guide corpus case has an empty name")
		}
		if names[c.Name] {
			return fmt.Errorf("guide corpus has duplicate case %q", c.Name)
		}
		names[c.Name] = true
		if err := validateIDs(c.UnitIDs, "unit_ids", nil); err != nil {
			return fmt.Errorf("guide corpus case %q: %w", c.Name, err)
		}
		known := make(map[string]bool, len(c.UnitIDs))
		for _, id := range c.UnitIDs {
			known[id] = true
		}
		for _, expected := range c.Expectations {
			if len(expected.TitleKeywords) == 0 && len(expected.SectionTitleKeywords) == 0 && len(expected.UnitIDGroups) == 0 {
				return fmt.Errorf("guide corpus case %q has an empty expectation", c.Name)
			}
			if err := validateKeywords(expected.TitleKeywords); err != nil {
				return fmt.Errorf("guide corpus case %q title keywords: %w", c.Name, err)
			}
			if err := validateKeywords(expected.SectionTitleKeywords); err != nil {
				return fmt.Errorf("guide corpus case %q section title keywords: %w", c.Name, err)
			}
			for _, group := range expected.UnitIDGroups {
				if err := validateIDs(group, "unit_id_groups", known); err != nil {
					return fmt.Errorf("guide corpus case %q: %w", c.Name, err)
				}
			}
		}
	}
	return nil
}

func validateIDs(ids []string, field string, known map[string]bool) error {
	if len(ids) == 0 {
		return fmt.Errorf("%s is empty", field)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("%s contains an empty ID", field)
		}
		if seen[id] {
			return fmt.Errorf("%s contains duplicate ID %q", field, id)
		}
		if known != nil && !known[id] {
			return fmt.Errorf("%s references unknown ID %q", field, id)
		}
		seen[id] = true
	}
	return nil
}

func validateKeywords(keywords []string) error {
	for _, keyword := range keywords {
		if strings.TrimSpace(keyword) == "" {
			return fmt.Errorf("contains an empty keyword")
		}
	}
	return nil
}
