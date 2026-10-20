package review

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prui/internal/guide"
	"prui/internal/source"
	"prui/internal/testutil"
)

// Derivation uses an already frozen snapshot; repository setup and initial
// capture are outside the timer. Supporting evidence makes accidental request
// preparation measurable while analysis is disabled.
func BenchmarkGuideDerivationWithoutAnalyzer(b *testing.B) {
	repo := testutil.NewRepo(b)
	for i := range 100 {
		repo.Write(fmt.Sprintf("docs/context-%03d.md", i), fmt.Sprintf("Context %d\n", i)+strings.Repeat("Ordinary supporting documentation for local review.\n", 40))
	}
	repo.Write("change.go", "package example\nconst Value = 1\n")
	base := repo.Commit()
	repo.Write("change.go", "package example\nconst Value = 2\n")
	head := repo.Commit()
	m := source.Metadata{Identity: source.Identity{Repository: "owner/repo", Number: 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	original, err := Open(context.Background(), repo.Dir, m.Identity, FixtureGitHub{m}, source.NewRunner(), source.Defaults(), nil)
	if err != nil || !original.Inventory.Complete || len(original.Context.Evidence) != 100 {
		b.Fatal("fixture omitted complete inventory or supporting evidence", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		derived := DeriveGuide(context.Background(), original, nil, Config{})
		if derived.Guides == nil || derived.Guides.Status != guide.Unavailable || derived.Guides.Reason != "analysis not requested" {
			b.Fatal("disabled analysis lost its explained fallback")
		}
	}
}
