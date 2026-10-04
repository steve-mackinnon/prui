package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/source"
	"prui/internal/testutil"
)

type FixtureGitHub struct{ Value source.Metadata }

func (g FixtureGitHub) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.Value, nil
}
func (g FixtureGitHub) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	return nil, nil
}
func (g FixtureGitHub) Token(context.Context) (string, error) {
	panic("unexpected network credentials")
}
func TestFileFallback(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "before\n")
	base := r.Commit()
	r.Write("a", "after\n")
	r.Write("b", "added\n")
	head := r.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head, Description: "Review the migration path."}
	s, e := Open(context.Background(), r.Dir, m.Identity, FixtureGitHub{m}, source.NewRunner(), source.Defaults(), nil)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Slices) != 2 || len(s.Inventory.Files) != 2 {
		t.Fatal("file slices missing")
	}
	seen := map[int]bool{}
	for _, slice := range s.Slices {
		for _, i := range slice.Units {
			if seen[i] {
				t.Fatal("multiple owners")
			}
			seen[i] = true
			if s.Inventory.Units[i].FileChangeID != slice.FileID {
				t.Fatal("wrong file ownership")
			}
		}
	}
	if len(seen) != len(s.Inventory.Units) {
		t.Fatal("unassigned fallback units")
	}
	if s.Guides == nil {
		t.Fatal("session carries no guide bundle")
	}
	if s.PullRequestDescription == nil || *s.PullRequestDescription != m.Description {
		t.Fatalf("session description = %#v, want %q", s.PullRequestDescription, m.Description)
	}
	if s.Guides.Status != guide.Unavailable || s.Guides.Reason == "" || len(s.Guides.Items) != 0 {
		t.Fatal("unrequested analysis is not an explained fallback")
	}
	if s.Guides.Provider != "" || s.Guides.Model != "" || s.Guides.InputDigest != "" {
		t.Fatal("fallback claims analysis provenance")
	}
	if err := guide.Validate(*s.Guides, s.Inventory); err != nil {
		t.Fatal("attached bundle does not validate", err)
	}
}

// fakeAnalyzer groups every eligible unit into one guide, so the review
// coordinator's contract is tested without a provider.
type fakeAnalyzer struct {
	err   error
	seen  guide.Input
	calls int
}

func (f *fakeAnalyzer) Analyze(_ context.Context, in guide.Input) (guide.Bundle, error) {
	f.calls++
	f.seen = in
	if f.err != nil {
		return guide.Bundle{}, f.err
	}
	section := guide.Section{Title: "Change every file", Description: "All eligible units."}
	for _, u := range in.Units {
		section.UnitIDs = append(section.UnitIDs, u.ID)
	}
	return guide.Bundle{Status: guide.Generated, Provider: "fake", Model: "fake-1", SchemaName: guide.SchemaName,
		Items: []guide.Item{{Title: "Whole comparison", Description: "One functional chunk.", Sections: []guide.Section{section}}}}, nil
}

func TestGuidesDoNotChangeTheFilePlan(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "before\n")
	base := r.Commit()
	r.Write("a", "after\n")
	r.Write("b", "added\n")
	head := r.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	open := func(cfg Config) *Session {
		t.Helper()
		s, e := OpenWithConfig(context.Background(), r.Dir, m.Identity, FixtureGitHub{m}, source.NewRunner(), source.Defaults(), nil, cfg)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	plain := open(Config{})
	if plain.Guides.Status != guide.Unavailable || plain.Guides.Reason != "analysis not requested" {
		t.Fatal("a nil analyzer is not an explained fallback", plain.Guides)
	}
	a := &fakeAnalyzer{}
	analyzed := open(Config{Analyzer: a})
	if a.calls != 1 || len(a.seen.Units) != len(analyzed.Inventory.Units) || a.seen.Digest == "" {
		t.Fatal("analysis did not receive the frozen inventory once", a.calls, len(a.seen.Units))
	}
	if analyzed.Guides.Status != guide.Generated || analyzed.Guides.Provider != "fake" || analyzed.Guides.InputDigest != a.seen.Digest {
		t.Fatal("generated guides did not reach the snapshot with provenance", analyzed.Guides)
	}
	if err := guide.Validate(*analyzed.Guides, analyzed.Inventory); err != nil {
		t.Fatal("stored guides do not validate against the inventory", err)
	}
	before, _ := json.Marshal([]any{plain.Slices, plain.UnitFiles, plain.Inventory})
	after, _ := json.Marshal([]any{analyzed.Slices, analyzed.UnitFiles, analyzed.Inventory})
	if !bytes.Equal(before, after) {
		t.Fatal("analysis changed file ownership or the raw inventory")
	}
	failed := open(Config{Analyzer: &fakeAnalyzer{err: errors.New("provider unavailable")}})
	if failed.Guides.Status != guide.Unavailable || !strings.Contains(failed.Guides.Reason, "provider unavailable") {
		t.Fatal("a failing analyzer did not degrade to an explained raw session", failed.Guides)
	}
	if len(failed.Slices) != len(plain.Slices) {
		t.Fatal("failed analysis cost the file plan")
	}
}

func TestDeriveGuideUsesFrozenMaterialWithoutChangingSource(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "before\n")
	base := r.Commit()
	r.Write("a", "after\n")
	head := r.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	original, err := Open(context.Background(), r.Dir, m.Identity, FixtureGitHub{m}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	original.ID = "0123456789abcdef0123456789abcdef"
	before, err := json.Marshal(original.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	analyzer := &fakeAnalyzer{}
	derived := DeriveGuide(context.Background(), original, analyzer, Config{})
	if derived.DerivedFrom != original.ID || derived.Guides == nil || derived.Guides.Status != guide.Generated || analyzer.calls != 1 {
		t.Fatal("derived guide did not use frozen session", derived)
	}
	after, err := json.Marshal(original.Snapshot)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("guide derivation modified the original snapshot", err)
	}
	if !bytes.Equal(derived.Inventory.Patches[derived.Inventory.Units[0].PatchReference], original.Inventory.Patches[original.Inventory.Units[0].PatchReference]) {
		t.Fatal("derived guide did not retain frozen patches")
	}
}

func TestFullSourceConsentPinsAndUploadBoundary(t *testing.T) {
	r := testutil.NewRepo(t)
	old := strings.Repeat("unchanged\n", 12) + "old\n"
	newText := strings.Repeat("unchanged\n", 12) + "new\n"
	r.Write("a", old)
	base := r.Commit()
	r.Write("a", newText)
	head := r.Commit()
	r.Write("a", "UNCOMMITTED CONTENT MUST NEVER BE READ\n")
	meta := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
	ordinary, err := Open(context.Background(), r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.Inventory.FullSource != nil {
		t.Fatal("source captured without explicit consent")
	}
	cached, err := OpenWithConfig(context.Background(), r.Dir, meta.Identity, FixtureGitHub{meta}, source.NewRunner(), source.Defaults(), nil, Config{CacheFullSource: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, oldSide := range []bool{true, false} {
		lines, ok := cached.Inventory.SourceLines(0, oldSide)
		want := newText
		if oldSide {
			want = old
		}
		if !ok || strings.Join(lines, "\n")+"\n" != want {
			t.Fatal("did not use complete pinned source", oldSide, lines)
		}
	}
	a := guide.InputFrom(ordinary.Inventory, ordinary.Context, Config{}.Policy, guide.Defaults)
	b := guide.InputFrom(cached.Inventory, cached.Context, Config{}.Policy, guide.Defaults)
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	if !bytes.Equal(aa, bb) {
		t.Fatal("local cache changed AI upload material")
	}
}
