package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"prui/internal/testutil"
	"reflect"
	"strings"
	"testing"
)

func TestAttributesPinnedGitSemanticsAndIsolation(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write(".gitattributes", "*.go review-test\n[a-z].go review-assets\n*.go linguist-generated\ndir/** review-documentation\nroot?.txt review-test\n/root.txt review-assets\n**/deep/** review-assets\ndir/ review-generated\n\"space name.go\" -review-test\n!bad.go review-generated\n*.go filter=evil diff=evil\n[attr]doc review-documentation\nmacro doc\n")
	r.Write("dir/.gitattributes", "*.go -review-test !linguist-generated\n")
	r.Write("dir/x.go", "source\n")
	sha := r.Commit()
	marker := filepath.Join(t.TempDir(), "executed")
	r.Git("config", "filter.evil.clean", "touch "+marker)
	r.Git("config", "diff.evil.command", "touch "+marker)
	global := filepath.Join(t.TempDir(), "global-attributes")
	if err := os.WriteFile(global, []byte("* review-generated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r.Git("config", "core.attributesFile", global)
	r.Write(".git/info/attributes", "* review-generated\n")
	r.Write(".gitattributes", "* review-generated\n")
	before := r.Snapshot()
	v, err := NewView(context.Background(), r.Dir, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	names := []string{"review-test", "linguist-generated", "review-documentation", "review-assets", "review-generated"}
	paths := [][]byte{[]byte("a.go"), []byte("dir/x.go"), []byte("space name.go"), []byte("macro"), []byte("bad.go"), []byte("rootx.txt"), []byte("nested/root.txt"), []byte("x/deep/z.txt")}
	got, err := v.Attributes(context.Background(), sha, names, paths)
	if err != nil {
		t.Fatal(err)
	}
	if got["a.go"]["review-assets"] != "set" || got["dir/x.go"]["review-test"] != "unset" || got["dir/x.go"]["linguist-generated"] != "unspecified" || got["dir/x.go"]["review-documentation"] != "set" || got["space name.go"]["review-test"] != "unset" || got["macro"]["review-documentation"] != "set" {
		t.Fatal(got)
	}
	if got["rootx.txt"]["review-test"] != "set" || got["nested/root.txt"]["review-assets"] != "unspecified" || got["x/deep/z.txt"]["review-assets"] != "set" {
		t.Fatal(got)
	}
	for _, a := range got {
		if a["review-generated"] != "unspecified" {
			t.Fatal("borrowed noncommitted policy", got)
		}
	}
	if _, err = os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("repository command ran")
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("checkout changed")
	}
	entries, _ := os.ReadDir(v.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "attributes-") {
			t.Fatal("temporary index leaked")
		}
	}
}
func TestAttributesBudgetAndInvalidPins(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write(".gitattributes", strings.Repeat("* review-test\n", 6000))
	sha := r.Commit()
	v, err := NewView(context.Background(), r.Dir, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if _, err = v.Attributes(context.Background(), sha, []string{"review-test"}, [][]byte{[]byte("x")}); err == nil {
		t.Fatal("oversized attributes accepted")
	}
	if _, err = v.Attributes(context.Background(), "HEAD", []string{"review-test"}, [][]byte{[]byte("x")}); err == nil {
		t.Fatal("unpinned source accepted")
	}
}

func TestAttributesAggregateBudgetsAndCancellation(t *testing.T) {
	for _, count := range []int{257, 34} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			r := testutil.NewRepo(t)
			for i := 0; i < count; i++ {
				content := "* review-test\n"
				if count == 34 {
					content = strings.Repeat("#", 63<<10) + "\n* review-test\n"
				}
				r.Write(fmt.Sprintf("dir%d/.gitattributes", i), content)
			}
			sha := r.Commit()
			v, err := NewView(context.Background(), r.Dir, NewRunner(), Defaults())
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			if _, err = v.Attributes(context.Background(), sha, []string{"review-test"}, [][]byte{[]byte("x")}); !errors.Is(err, ErrLimit) {
				t.Fatalf("budget accepted: %v", err)
			}
		})
	}
	r := testutil.NewRepo(t)
	r.Write("a", "a")
	sha := r.Commit()
	v, err := NewView(context.Background(), r.Dir, NewRunner(), Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = v.Attributes(ctx, sha, []string{"review-test"}, [][]byte{[]byte("x")}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = v.Attributes(context.Background(), sha, []string{"review-test"}, [][]byte{[]byte(strings.Repeat("a", 4097))}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}
