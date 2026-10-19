package commits

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"prui/internal/source"
	"prui/internal/testutil"
)

func TestCompositionPathspecsBoundedAndLiteral(t *testing.T) {
	paths := map[string]bool{"b": true, "a[1]": true, ":(glob)*": true}
	if got := compositionPathspecs(paths); !reflect.DeepEqual(got, []string{":(glob)*", "a[1]", "b"}) {
		t.Fatal(got)
	}
	for _, set := range []map[string]bool{nil, {"../bad": true}, {strings.Repeat("x", 4097): true}} {
		if compositionPathspecs(set) != nil {
			t.Fatal("unsafe/empty scope did not retain full listing")
		}
	}
	large := map[string]bool{}
	for i := range 100 {
		large[fmt.Sprint(i)+strings.Repeat("x", 1000)] = true
	}
	if compositionPathspecs(large) != nil {
		t.Fatal("unbounded argument bytes")
	}
	many := map[string]bool{}
	for i := range 257 {
		many[fmt.Sprint(i)] = true
	}
	if compositionPathspecs(many) != nil {
		t.Fatal("large matcher scope did not retain full listing")
	}
}

func TestCompositionScopedTreeMatchesFullProjection(t *testing.T) {
	r := testutil.NewRepo(t)
	names := []string{"a[1]", "a*", ":(glob)*", "--option", "space name", "tab\tname", "replace"}
	paths := map[string]bool{"replace/child": true, "bad\xff": true}
	// Create byte-only names in committed objects, avoiding host filesystem
	// Unicode restrictions. Neither runtime capture nor this query checks out code.
	withBytePath := func(commit string) string {
		blob := r.GitInput("byte path\n", "hash-object", "-w", "--stdin")
		raw := r.Git("ls-tree", "-z", commit) + "100644 blob " + blob + "\tbad\xff\x00"
		tree := r.GitInput(raw, "mktree", "-z")
		return r.GitInput("byte path tree\n", "commit-tree", tree)
	}
	for _, p := range names {
		r.Write(p, "content\n")
		paths[p] = true
	}
	r.Write("unrelated", "untouched\n")
	base := withBytePath(r.Commit())
	if err := os.Remove(filepath.Join(r.Dir, "replace")); err != nil {
		t.Fatal(err)
	}
	r.Write("replace/child", "child\n")
	r.Write("replace/unrelated", "outside projection\n")
	head := withBytePath(r.Commit())
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	for _, tree := range []string{base, head} {
		full, err := v.Git(context.Background(), 1<<20, compositionTreeArgs(tree, nil)...)
		if err != nil {
			t.Fatal(err)
		}
		scoped, err := v.Git(context.Background(), 1<<20, compositionTreeArgs(tree, compositionPathspecs(paths))...)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(projectTree(full, paths), projectTree(scoped, paths)) {
			t.Fatalf("projection changed for %s", tree)
		}
	}
}

func projectTree(raw []byte, paths map[string]bool) []byte {
	var out []byte
	for _, record := range bytes.Split(raw, []byte{0}) {
		parts := bytes.SplitN(record, []byte{'\t'}, 2)
		if len(parts) == 2 && paths[string(parts[1])] {
			out = append(out, record...)
			out = append(out, 0)
		}
	}
	return out
}

func BenchmarkCompositionTreeListing(b *testing.B) {
	r := testutil.NewRepo(b)
	for i := range 10000 {
		p := fmt.Sprintf("pkg/%02d/file-%05d.go", i/500, i)
		r.Write(p, "package fixture\n")
	}
	head := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = v.Close() }()
	for _, count := range []int{100, 256, 1000} {
		paths := map[string]bool{}
		for i := range count {
			paths[fmt.Sprintf("pkg/%02d/file-%05d.go", i/500, i)] = true
		}
		for _, scoped := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/%t", count, scoped), func(b *testing.B) {
				var selected []string
				if scoped {
					selected = compositionPathspecs(paths)
				}
				args := compositionTreeArgs(head, selected)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					raw, err := v.Git(context.Background(), 50<<20, args...)
					if err != nil {
						b.Fatal(err)
					}
					if bytes.Count(projectTree(raw, paths), []byte{0}) != len(paths) {
						b.Fatal("incomplete projection")
					}
					b.ReportMetric(float64(len(raw)), "metadata-bytes/op")
				}
			})
		}
	}
}
