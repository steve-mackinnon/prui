package inventory

import (
	"context"
	"path"
	"strings"
	"unicode/utf8"
)

type Category string

const (
	Implementation Category = "implementation"
	Tests          Category = "tests"
	Documentation  Category = "documentation"
	Generated      Category = "generated"
	Assets         Category = "assets"
	Support        Category = "support"
)

var Categories = []Category{Implementation, Tests, Documentation, Generated, Assets, Support}

// Classification is frozen presentation evidence. It never contributes to raw
// file/unit identities, source patches, comment coordinates, or progress carry.
type Classification struct {
	Category   Category          `json:"category"`
	SourceSHA  string            `json:"source_sha"`
	Attributes map[string]string `json:"attributes,omitempty"`
	Partial    bool              `json:"partial,omitempty"`
}

var categoryAttributes = map[string]Category{
	"review-implementation": Implementation, "review-test": Tests, "review-documentation": Documentation,
	"review-generated": Generated, "review-assets": Assets, "review-agent-guidance": Support,
	"review-localization": Support, "linguist-generated": Generated, "linguist-documentation": Documentation,
	"linguist-vendored": Support,
}

func classifyPath(p string, a map[string]string) Category {
	// Explicit review categories override Linguist. Conflicts use this documented
	// order, independent of map iteration. False suppresses that default category.
	for _, name := range []string{"review-implementation", "review-test", "review-documentation", "review-generated", "review-assets", "review-agent-guidance", "review-localization", "linguist-generated", "linguist-documentation", "linguist-vendored"} {
		if a[name] == "true" {
			return categoryAttributes[name]
		}
	}
	p = strings.ToLower(p)
	base := path.Base(p)
	ext := path.Ext(p)
	defaults := Implementation
	switch {
	case strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") || strings.HasPrefix(p, "tests/") || strings.HasPrefix(p, "test/"):
		defaults = Tests
	case strings.HasPrefix(p, "docs/") || ext == ".md" || ext == ".rst":
		defaults = Documentation
	case strings.HasPrefix(p, "generated/") || strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".pb.go"):
		defaults = Generated
	case assetExtension(ext):
		defaults = Assets
	case strings.HasPrefix(p, ".github/") || strings.HasPrefix(p, "scripts/") || strings.HasPrefix(base, ".") || base == "go.mod" || base == "go.sum" || strings.HasSuffix(base, "lock.json") || strings.HasSuffix(base, ".lock"):
		defaults = Support
	}
	for name, c := range categoryAttributes {
		if c == defaults && a[name] == "false" {
			if defaults == Implementation {
				return Support
			}
			return Implementation
		}
	}
	return defaults
}

func assetExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".woff", ".woff2", ".mp3", ".mp4":
		return true
	default:
		return false
	}
}

// Only the isolated committed-object view can provide authoritative attributes.
type attributeObjects interface {
	Attributes(context.Context, string, []string, [][]byte) (map[string]map[string]string, error)
}

func captureClassifications(ctx context.Context, o Objects, files []FileChange, oldSHA, newSHA string) map[string]Classification {
	if len(files) == 0 {
		return nil
	}
	result := make(map[string]Classification, len(files))
	names := []string{"review-implementation", "review-test", "review-documentation", "review-generated", "review-assets", "review-agent-guidance", "review-localization", "linguist-generated", "linguist-documentation", "linguist-vendored"}
	bySHA := map[string][]FileChange{}
	for _, f := range files {
		sha := newSHA
		if len(f.NewPath) == 0 {
			sha = oldSHA
		}
		bySHA[sha] = append(bySHA[sha], f)
	}
	reader, available := o.(attributeObjects)
	for sha, group := range bySHA {
		paths := make([][]byte, 0, len(group))
		for _, f := range group {
			p := f.NewPath
			if len(p) == 0 {
				p = f.OldPath
			}
			paths = append(paths, p)
		}
		var evidence map[string]map[string]string
		partial := !available
		if available {
			var err error
			evidence, err = reader.Attributes(ctx, sha, names, paths)
			partial = err != nil
		}
		for i, f := range group {
			p := string(paths[i])
			a := map[string]string{}
			incomplete := partial
			for name, value := range evidence[p] {
				switch value {
				case "set", "true":
					a[name] = "true"
				case "unset", "false":
					a[name] = "false"
				case "unspecified":
				default:
					incomplete = true
				}
			}
			category := classifyPath(p, a)
			if incomplete || !utf8.ValidString(p) {
				category = Support
				incomplete = true
				a = nil
			}
			if len(a) == 0 {
				a = nil
			}
			result[f.ID] = Classification{Category: category, SourceSHA: sha, Attributes: a, Partial: incomplete}
		}
	}
	return result
}
