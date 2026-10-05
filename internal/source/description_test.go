package source

import (
	"context"
	"strings"
	"testing"
)

func TestGitHubMetadataCapturesAndValidatesDescription(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	metadataJSON := func(body string) []byte {
		return []byte(`{"number":42,"body":` + body + `,"base":{"ref":"release/next","sha":"` + base + `","repo":{"full_name":"owner/repo"}},"head":{"sha":"` + head + `","repo":{"full_name":"fork/repo"}}}`)
	}
	for _, tc := range []struct {
		name, body string
		want       string
		valid      bool
	}{
		{"text", `"Summary\n\n- item"`, "Summary\n\n- item", true},
		{"null", `null`, "", true},
		{"oversized", `"` + strings.Repeat("x", descriptionMaxBytes+1) + `"`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: requestFunc(func(context.Context, Request) ([]byte, error) { return metadataJSON(tc.body), nil })}
			got, err := g.Metadata(context.Background(), Identity{Repository: "owner/repo", Number: 42})
			if (err == nil) != tc.valid {
				t.Fatalf("Metadata error = %v, want valid=%v", err, tc.valid)
			}
			if tc.valid && (got.Description != tc.want || got.TargetBranch != "release/next") {
				t.Fatalf("description = %q, want %q", got.Description, tc.want)
			}
		})
	}
}

func TestSamePinnedRevisionIgnoresDescriptionButNotSourcePins(t *testing.T) {
	base := Metadata{Identity: Identity{Repository: "owner/repo", Number: 42}, BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), Description: "first"}
	changedDescription := base
	changedDescription.Description = "edited"
	if !SamePinnedRevision(base, changedDescription) {
		t.Fatal("description-only metadata edit changed the pinned revision")
	}
	changedHead := changedDescription
	changedHead.HeadSHA = strings.Repeat("c", 40)
	if SamePinnedRevision(base, changedHead) {
		t.Fatal("changed head SHA matched pinned revision")
	}
}

func TestGitHubMetadataRejectsInvalidUTF8DescriptionResponse(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	body := append([]byte(`{"number":42,"body":"`), 0xff)
	body = append(body, []byte(`","base":{"sha":"`+base+`","repo":{"full_name":"owner/repo"}},"head":{"sha":"`+head+`","repo":{"full_name":"fork/repo"}}}`)...)
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: requestFunc(func(context.Context, Request) ([]byte, error) { return body, nil })}
	if _, err := g.Metadata(context.Background(), Identity{Repository: "owner/repo", Number: 42}); err == nil {
		t.Fatal("invalid UTF-8 description response was accepted")
	}
}
