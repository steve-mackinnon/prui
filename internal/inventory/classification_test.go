package inventory

import (
	"context"
	"prui/internal/source"
	"prui/internal/testutil"
	"testing"
)

func TestClassificationPinnedNestedAttributes(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write(".gitattributes", "**/*_test.go review-test\ngenerated/** linguist-generated\ndocs/** linguist-documentation\n")
	r.Write("docs/.gitattributes", "*.go -linguist-documentation review-test\n")
	r.Write("generated/.gitattributes", "keep.go -linguist-generated\n")
	r.Write("gone.go", "old\n")
	base := r.Commit()
	r.Git("rm", "gone.go")
	for _, p := range []string{"main.go", "main_test.go", "docs/check.go", "generated/a.go", "generated/keep.go", "image.png"} {
		r.Write(p, "new\n")
	}
	head := r.Commit()
	r.Write(".gitattributes", "** review-generated\n")
	v, e := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	inv, e := Build(context.Background(), v, source.PinnedComparison{Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}, MergeBaseSHA: base}, source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]Category{"gone.go": Implementation, "main.go": Implementation, "main_test.go": Tests, "docs/check.go": Tests, "generated/a.go": Generated, "generated/keep.go": Implementation, "image.png": Assets}
	for _, f := range inv.Files {
		p := string(f.NewPath)
		if p == "" {
			p = string(f.OldPath)
		}
		c := inv.Classifications[f.ID]
		if c.Category != want[p] {
			t.Errorf("%s: %+v want %s", p, c, want[p])
		}
		sha := head
		if f.Status == "D" {
			sha = base
		}
		if c.SourceSHA != sha {
			t.Errorf("wrong provenance: %+v", c)
		}
	}
}

func TestCategoryOverrideAndSafeDefaults(t *testing.T) {
	for _, tc := range []struct {
		path  string
		attrs map[string]string
		want  Category
	}{
		{"generated/x.go", map[string]string{"linguist-generated": "false"}, Implementation},
		{"x.go", map[string]string{"linguist-generated": "true", "review-implementation": "true"}, Implementation},
		{"x.go", map[string]string{"review-test": "true", "review-documentation": "true"}, Tests},
		{"script.lock", nil, Support},
	} {
		if got := classifyPath(tc.path, tc.attrs); got != tc.want {
			t.Errorf("%s: %s", tc.path, got)
		}
	}
}
