package inventory

import (
	"context"
	"encoding/json"
	"os"
	"prui/internal/source"
	"prui/internal/testutil"
	"reflect"
	"strings"
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

func TestClassificationDeletionRenameBinaryAndInvalidValues(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write(".gitattributes", "gone.go review-test\nold.go review-documentation\n*.png review-assets\n")
	r.Write("gone.go", "deleted\n")
	r.Write("old.go", "renamed identical content\n")
	base := r.Commit()
	r.Git("rm", "gone.go")
	r.Git("mv", "old.go", "new.go")
	r.Write(".gitattributes", "gone.go review-generated\nnew.go review-test\ninvalid.go linguist-generated=maybe\n")
	r.Write("asset.png", "\x00binary\n")
	r.Write("invalid.go", "invalid attr\n")
	r.Write("plain.go", "no attrs\n")
	head := r.Commit()
	v, e := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	inv, e := Build(context.Background(), v, source.PinnedComparison{Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}, MergeBaseSHA: base}, source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	expected := map[string]Category{".gitattributes": Support, "gone.go": Tests, "new.go": Tests, "asset.png": Assets, "invalid.go": Support, "plain.go": Implementation}
	sawRename, sawBinary := false, false
	for _, f := range inv.Files {
		p := string(f.NewPath)
		sha := head
		if p == "" {
			p = string(f.OldPath)
			sha = base
		}
		c := inv.Classifications[f.ID]
		if c.Category != expected[p] || c.SourceSHA != sha {
			t.Errorf("%s %+v", p, c)
		}
		if p == "invalid.go" && !c.Partial {
			t.Fatal("invalid attr not explicit")
		}
		if p == "new.go" {
			sawRename = strings.HasPrefix(f.Status, "R") && string(f.OldPath) == "old.go"
		}
		if p == "asset.png" {
			for _, u := range inv.Units {
				if u.FileChangeID == f.ID && u.Kind == Binary {
					sawBinary = true
				}
			}
		}
	}
	if !sawRename || !sawBinary {
		t.Fatal("missing binary/rename fixture")
	}
	// Evidence is completely frozen before the originating checkout disappears.
	if e := os.RemoveAll(r.Dir); e != nil {
		t.Fatal(e)
	}
	if !inv.Complete || len(inv.Classifications) != len(inv.Files) {
		t.Fatal("classification damaged inventory")
	}
}

func TestClassificationEmptyAttributesCanonicalRoundtrip(t *testing.T) {
	v, p, _ := fixture(t)
	inv, e := Build(context.Background(), v, p, source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	data, e := json.Marshal(inv)
	if e != nil {
		t.Fatal(e)
	}
	var restored Inventory
	if e = json.Unmarshal(data, &restored); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(inv.Classifications, restored.Classifications) {
		t.Fatal("classification differs after frozen storage roundtrip")
	}
}
