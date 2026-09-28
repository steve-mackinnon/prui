package guide

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	reviewcontext "prui/internal/context"
	"prui/internal/inventory"
	"prui/internal/privacy"
)

// fake is a deterministic analyzer: the grouping, coverage, and failure
// contracts are settled without a provider.
type fake struct {
	bundle Bundle
	err    error
	block  bool
	seen   Input
	calls  int
}

func (f *fake) Analyze(ctx context.Context, in Input) (Bundle, error) {
	f.calls++
	f.seen = in
	if f.block {
		<-ctx.Done()
		return Bundle{}, ctx.Err()
	}
	return f.bundle, f.err
}

func units(ids ...string) []string { return ids }

func covered(t *testing.T, b Bundle, inv inventory.Inventory) {
	t.Helper()
	if err := Validate(b, inv); err != nil {
		t.Fatal("stored bundle does not validate", err)
	}
	if b.Status != Generated {
		return // a fallback claims no navigation; the raw file plan remains
	}
	seen := map[string]int{}
	for _, item := range b.Items {
		for _, s := range item.Sections {
			for _, id := range s.UnitIDs {
				seen[id]++
			}
		}
	}
	for _, u := range inv.Units {
		if seen[u.ID] != 1 {
			t.Fatalf("unit %s appears %d times in guide navigation", u.ID, seen[u.ID])
		}
	}
}

func ungroupedIDs(b Bundle) []string {
	for _, item := range b.Items {
		if item.Ungrouped {
			var ids []string
			for _, s := range item.Sections {
				ids = append(ids, s.UnitIDs...)
			}
			return ids
		}
	}
	return nil
}

func analyzed(t *testing.T, a Analyzer, inv inventory.Inventory) Bundle {
	t.Helper()
	in := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults)
	return Analyze(context.Background(), a, inv, in)
}

func TestAnalyzeWithoutAnalyzer(t *testing.T) {
	inv := fixture("u1")
	b := analyzed(t, nil, inv)
	if b.Status != Unavailable || b.Reason != "analysis not requested" {
		t.Fatal("absent analysis is not stated as a decision", b)
	}
	covered(t, b, inv)
}

func TestAnalyzePartialResponseCoversTheRemainder(t *testing.T) {
	inv := fixture("u1", "u2", "u3")
	a := &fake{bundle: generated(Section{Title: "Add login endpoint", UnitIDs: units("u1")})}
	b := analyzed(t, a, inv)
	if b.Status != Generated || len(b.Items) != 2 {
		t.Fatal("partial grouping did not gain a coverage guide", b)
	}
	if !b.Items[1].Ungrouped || b.Items[1].Title != "Ungrouped changes" {
		t.Fatal("coverage guide missing or unlabelled", b.Items[1])
	}
	if got := ungroupedIDs(b); strings.Join(got, ",") != "u2,u3" {
		t.Fatal("ungrouped units lost their inventory order", got)
	}
	covered(t, b, inv)
	if b.InputDigest != a.seen.Digest || b.Limits != Defaults || b.PromptVersion != PromptVersion {
		t.Fatal("stored bundle lost its provenance")
	}
}

func TestAnalyzeGroupsFileMetadataWithItsFirstGroupedContentUnit(t *testing.T) {
	b := &builder{}
	metadata := b.unit("apps/cms/src/collections/Media.ts", "", inventory.FileMetadata)
	content := b.unit("apps/cms/src/collections/Media.ts", "@@ -1 +1 @@\n+export const Media = {}\n", inventory.TextHunk)
	inv := b.build()
	a := &fake{bundle: generated(Section{Title: "Update media", UnitIDs: units(content)})}

	bundle := analyzed(t, a, inv)
	if got := bundle.Items[0].Sections[0].UnitIDs; strings.Join(got, ",") != metadata+","+content {
		t.Fatalf("file metadata = %v, want it before grouped content", got)
	}
	if got := ungroupedIDs(bundle); len(got) != 0 {
		t.Fatalf("metadata for grouped content remained ungrouped: %v", got)
	}
	covered(t, bundle, inv)
}

func TestAnalyzeDropsOnlyUnusableSections(t *testing.T) {
	inv := fixture("u1", "u2", "u3")
	a := &fake{bundle: Bundle{Status: Generated, Items: []Item{{Title: "Authentication flow", Sections: []Section{
		{Title: "Add login endpoint", UnitIDs: units("u1")},
		{Title: "Invented surface", UnitIDs: units("u2", "absent")},
		{Title: "Repeated unit", UnitIDs: units("u1", "u3")},
	}}}}}
	b := analyzed(t, a, inv)
	if len(b.Items) != 2 || len(b.Items[0].Sections) != 1 || b.Items[0].Sections[0].Title != "Add login endpoint" {
		t.Fatal("a bad reference cost more than its own section", b.Items)
	}
	if got := ungroupedIDs(b); strings.Join(got, ",") != "u2,u3" {
		t.Fatal("units from dropped sections left the review surface", got)
	}
	covered(t, b, inv)
}

func TestAnalyzeRejectsModelAuthoredCoverageGuides(t *testing.T) {
	inv := fixture("u1", "u2")
	a := &fake{bundle: Bundle{Status: Generated, Items: []Item{
		{Title: "Ungrouped changes", Ungrouped: true, Sections: []Section{{Title: "Ungrouped changes", UnitIDs: units("u1")}}},
		{Title: "Authentication flow", Sections: []Section{{Title: "Add login endpoint", UnitIDs: units("u2")}}},
	}}}
	b := analyzed(t, a, inv)
	if len(b.Items) != 2 || b.Items[0].Title != "Authentication flow" || !b.Items[1].Ungrouped {
		t.Fatal("model claimed the synthesized coverage guide", b.Items)
	}
	if got := ungroupedIDs(b); strings.Join(got, ",") != "u1" {
		t.Fatal("coverage guide is not the local synthesis", got)
	}
	covered(t, b, inv)
}

func TestAnalyzeFallbacks(t *testing.T) {
	inv := fixture("u1", "u2")
	big := Bundle{Status: Generated}
	for i := 0; i < maxItems+1; i++ {
		big.Items = append(big.Items, Item{Title: "guide", Sections: []Section{{Title: "section"}}})
	}
	wordy := generated(Section{Title: "Add", UnitIDs: units("u1", "u2")})
	wordy.Items[0].Description = strings.Repeat("x", maxTextBytes+1)
	cases := []struct {
		name    string
		fake    *fake
		reason  string
		limits  Limits
		wantErr bool
	}{
		{name: "analyzer error", fake: &fake{err: errors.New("provider refused the request")}, reason: "provider refused the request"},
		{name: "unavailable response", fake: &fake{bundle: Fallback("provider disabled")}, reason: "provider disabled"},
		{name: "statusless response", fake: &fake{bundle: Bundle{}}, reason: "analyzer returned no guides"},
		{name: "nothing usable", fake: &fake{bundle: generated(Section{Title: "Add", UnitIDs: units("absent")})}, reason: "no usable guides"},
		{name: "invalid after repair", fake: &fake{bundle: generated(Section{Title: "  ", UnitIDs: units("u1", "u2")})}, reason: "analysis discarded"},
		{name: "oversize response", fake: &fake{bundle: big}, reason: "exceeds the guide limit"},
		{name: "oversize text", fake: &fake{bundle: wordy}, reason: "exceeds the response byte limit"},
		{name: "timeout", fake: &fake{block: true}, reason: "analysis timed out", limits: Limits{Duration: 20 * time.Millisecond}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			limits := c.limits
			if limits == (Limits{}) {
				limits = Defaults
			}
			in := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, limits)
			b := Analyze(context.Background(), c.fake, inv, in)
			if b.Status != Unavailable {
				t.Fatal("failed analysis kept guides", b)
			}
			if !strings.Contains(b.Reason, c.reason) {
				t.Fatalf("reason %q does not state %q", b.Reason, c.reason)
			}
			if len(b.Items) != 0 || b.InputDigest != "" || b.Provider != "" {
				t.Fatal("fallback claims analysis provenance", b)
			}
			covered(t, b, inv)
		})
	}
}

func TestAnalyzeBoundsTheFailureReason(t *testing.T) {
	inv := fixture("u1")
	a := &fake{err: errors.New(strings.Repeat("noise ", 400))}
	b := Analyze(context.Background(), a, inv, InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults))
	if len(b.Reason) > 256 {
		t.Fatal("unbounded provider text became durable session content", len(b.Reason))
	}
}

func TestAnalyzeSendsOnlyRequestMaterial(t *testing.T) {
	b := &builder{}
	b.unit("a.go", "@@ -1 +1 @@\n+one\n", inventory.TextHunk)
	b.unit(".env", "@@ -0,0 +1 @@\n+TOKEN=abc\n", inventory.TextHunk)
	inv := b.build()
	a := &fake{bundle: Bundle{Status: Generated, Items: []Item{{Title: "One", Sections: []Section{{Title: "One", UnitIDs: units("u1")}}}}}}
	bundle := analyzed(t, a, inv)
	if a.calls != 1 {
		t.Fatal("analysis is not a single bounded request", a.calls)
	}
	if len(a.seen.Units) != 1 || string(a.seen.Units[0].Path) != "a.go" {
		t.Fatal("withheld unit reached the analyzer", a.seen.Units)
	}
	if len(bundle.WithheldPaths) != 1 || string(bundle.WithheldPaths[0].Path) != ".env" {
		t.Fatal("stored bundle does not disclose withheld input", bundle.WithheldPaths)
	}
	if got := ungroupedIDs(bundle); strings.Join(got, ",") != "u2" {
		t.Fatal("withheld unit lost its place in guide navigation", got)
	}
	covered(t, bundle, inv)
}
