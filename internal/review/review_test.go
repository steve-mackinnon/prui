package review

import (
	"context"
	"pr-review/internal/guide"
	"pr-review/internal/source"
	"pr-review/internal/testutil"
	"testing"
)

type FixtureGitHub struct{ Value source.Metadata }

func (g FixtureGitHub) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return g.Value, nil
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
	m := source.Metadata{Identity: source.Identity{Repository: "o/r", Number: 1}, BaseRepository: "o/r", HeadRepository: "o/r", BaseSHA: base, HeadSHA: head}
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
