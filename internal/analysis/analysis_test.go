package analysis

import (
	"context"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"pr-review/internal/source"
)

type fakeProvider struct {
	calls    int
	response []byte
}

func (f *fakeProvider) Complete(_ context.Context, _ Config, _ []byte) ([]byte, error) {
	f.calls++
	return f.response, nil
}

func TestConsentBindsExactPayloadAndFallback(t *testing.T) {
	inv := inventory.Inventory{Comparison: structComparison("inv"), Units: []inventory.ReviewUnit{{ID: "u", InventoryID: "inv", FileChangeID: "f", Kind: inventory.FileMetadata}}}
	payload, raw, err := Prepare(inv, reviewcontext.ContextBundle{}, structPolicy(), Config{})
	if err != nil || payload.InventoryID != "inv" {
		t.Fatal(err)
	}
	f := &fakeProvider{response: []byte(`{"inventory_id":"inv","slices":[]}`)}
	c := ConsentFor(raw, Config{Provider: "fake", Model: "test", MaxOutputTokens: 1})
	r := Analyze(context.Background(), f, Config{Provider: "fake", Model: "test", MaxOutputTokens: 1}, raw, c, inv, reviewcontext.ContextBundle{})
	if f.calls != 1 || r.Status != "valid" || len(r.Plan.UnassignedUnitIDs) != 1 {
		t.Fatalf("calls=%d result=%+v", f.calls, r)
	}
	r = Analyze(context.Background(), f, Config{Provider: "fake", Model: "changed", MaxOutputTokens: 1}, raw, c, inv, reviewcontext.ContextBundle{})
	if f.calls != 1 || r.Status != "denied" {
		t.Fatalf("consent bypass: calls=%d result=%+v", f.calls, r)
	}
}

// Small constructors keep this test independent of source adapter details.
func structComparison(id string) source.PinnedComparison {
	return source.PinnedComparison{InventoryID: id}
}
func structPolicy() privacy.Policy { return privacy.Policy{} }
