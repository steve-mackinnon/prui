package inventory

import (
	"context"
	"reflect"
	"testing"

	"prui/internal/source"
	"prui/internal/testutil"
)

func TestTreeBatchInventoryPreservesMixedChangesAndCheckout(t *testing.T) {
	repo := testutil.NewRepo(t)
	for path, content := range map[string]string{
		"plain": "old\n", "removed": "gone\n", "folder/old.go": "old child\n", "swap": "old file\n",
		"empty-existing": "", "empty-remove": "", "old-name": "stable\nstable\nstable\nold\n",
		"odd space": "old spaced path\n", "binary": "old\x00binary",
	} {
		repo.Write(path, content)
	}
	base := repo.Commit()
	repo.Git("rm", "-r", "removed", "folder", "swap", "empty-remove")
	repo.Git("mv", "old-name", "new-name")
	for path, content := range map[string]string{
		"plain": "new\n", "added": "new file\n", "folder": "new file replacing directory\n", "swap/child.go": "new child\n",
		"empty-existing": "nonempty\n", "empty-added": "", "new-name": "stable\nstable\nstable\nnew\n",
		"odd space": "new spaced path\n", "binary": "new\x00binary",
	} {
		repo.Write(path, content)
	}
	head := repo.Commit()
	before := repo.Snapshot()
	limits := source.Defaults()
	serial, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer serial.Close()
	parallel, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
	if err != nil {
		t.Fatal(err)
	}
	defer parallel.Close()
	comparison := source.PinnedComparison{MergeBaseSHA: base, Metadata: source.Metadata{BaseSHA: base, HeadSHA: head}}
	want, err := Build(context.Background(), serialBlobObjects{serial}, comparison, limits)
	if err != nil || !want.Complete {
		t.Fatal("serial fixture incomplete", err)
	}
	got, err := Build(context.Background(), parallel, comparison, limits)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("tree batching changed inventory: err=%v problems=%v", err, got.Problems)
	}
	if !reflect.DeepEqual(before, repo.Snapshot()) {
		t.Fatal("tree batching changed the reviewed checkout")
	}
}
