package commits

import (
	"context"
	"prui/internal/source"
	"prui/internal/testutil"
	"strings"
	"testing"
)

func TestCaptureCompositionDeduplicatesAndFreezesSkippedBaseline(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("a", "base\n")
	r.Write("unrelated", "untouched\n")
	base := r.Commit()
	r.Write("a", "first\n")
	first := r.Commit()
	r.Write("b", "later\n")
	head := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{BaseSHA: base, HeadSHA: head}
	b, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: first}, {SHA: head}}}, source.Defaults(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Composition == nil || b.Composition.Status != Captured || !ValidateComposition(b) {
		t.Fatalf("invalid composition: %#v", b.Composition)
	}
	if len(b.Composition.Blobs) != 3 {
		t.Fatalf("blobs %d", len(b.Composition.Blobs))
	}
	for _, tree := range b.Composition.Trees {
		for _, e := range tree {
			if string(e.Path) == "unrelated" {
				t.Fatal("unrelated captured")
			}
		}
	}
	original := b.Composition.Trees[base]
	b.Composition.Trees[base] = append(append([]TreeEntry(nil), original...), b.Composition.Trees[head][1])
	if ValidateComposition(b) {
		t.Fatal("undeclared tree change accepted")
	}
	b.Composition.Trees[base] = original
	saved := b.Composition.Trees[head][0]
	b.Composition.Trees[head][0].Path = []byte("../escape")
	if ValidateComposition(b) {
		t.Fatal("unsafe path accepted")
	}
	b.Composition.Trees[head][0] = saved
	b.Composition.Trees[head][0].Mode = "100755"
	if ValidateComposition(b) {
		t.Fatal("undeclared mode accepted")
	}
	b.Composition.Trees[head][0] = saved
	for oid, data := range b.Composition.Blobs {
		b.Composition.Blobs[oid] = append(data, 'x')
		if ValidateComposition(b) {
			t.Fatal("corrupt content accepted")
		}
		break
	}
}

func TestCompositionLegacyAndUnavailableContract(t *testing.T) {
	if !ValidateComposition(&Bundle{}) {
		t.Fatal("legacy rejected")
	}
	b := &Bundle{Composition: &Composition{Status: Unavailable, Reason: "missing"}}
	if !ValidateComposition(b) {
		t.Fatal("unavailable rejected")
	}
	b.Composition.Blobs = map[string][]byte{"garbage": []byte("source")}
	if ValidateComposition(b) {
		t.Fatal("partial unavailable source accepted")
	}
}

func TestCompositionPathBounds(t *testing.T) {
	for _, path := range []string{strings.Repeat("a", 4097), strings.Repeat("a/", 128) + "leaf"} {
		if safeCompositionPath(path) {
			t.Fatal("oversized or deep path accepted")
		}
	}
	if !safeCompositionPath(strings.Repeat("a/", 127) + "leaf") {
		t.Fatal("bounded path rejected")
	}
	b := &Bundle{Composition: &Composition{Status: Unavailable, Reason: string([]byte{255})}}
	if ValidateComposition(b) {
		t.Fatal("invalid reason UTF-8 accepted")
	}
}

func TestCaptureDeepPathPreservesDedicatedDiff(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("base", "base\n")
	base := r.Commit()
	r.Write(strings.Repeat("d/", 128)+"leaf", "captured\n")
	head := r.Commit()
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	m := source.Metadata{BaseSHA: base, HeadSHA: head}
	b, err := Capture(context.Background(), v, source.PinnedComparison{Metadata: m}, fakeGH{metadata: m, entries: []source.PullRequestCommit{{SHA: head}}}, source.Defaults(), nil)
	if err != nil || b.Entries[0].Status != Captured || b.Composition == nil || b.Composition.Status != Unavailable || !ValidateComposition(b) {
		t.Fatalf("deep path capture: %#v %v", b, err)
	}
}
