package inventory

import (
	"context"
	"prui/internal/source"
	"reflect"
	"testing"
)

func TestCommitComparisonUsesSharedInventoryRules(t *testing.T) {
	v, p, _ := fixture(t)
	normal, err := Build(context.Background(), v, p, source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	diff, err := BuildCommit(context.Background(), v, CommitComparison{ParentSHA: p.MergeBaseSHA, CommitSHA: p.Metadata.HeadSHA}, source.Defaults())
	if err != nil || !diff.Complete || !reflect.DeepEqual(diff.Files, normal.Files) || !reflect.DeepEqual(diff.Patches, normal.Patches) {
		t.Fatal(diff, err)
	}
	if diff.Comparison.Metadata.Identity.Repository != "" {
		t.Fatal("fabricated PR metadata")
	}
}
