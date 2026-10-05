package session

import (
	"context"
	"path/filepath"
	"prui/internal/inventory"
	"reflect"
	"testing"
)

func TestClassificationOfflineRestartPreservesRawProgress(t *testing.T) {
	p := filepath.Join(t.TempDir(), "store")
	store, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := fixture()
	f := snapshot.Inventory.Files[0]
	evidence := map[string]inventory.Classification{f.ID: {Category: inventory.Generated, SourceSHA: snapshot.Inventory.Comparison.Metadata.HeadSHA, Attributes: map[string]string{"linguist-generated": "true"}}}
	snapshot.Inventory.Classifications = evidence
	record, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateState(context.Background(), record.ID, record.Generation, record.SnapshotReference, StateUpdate{ReviewedSliceIDs: []string{f.ID}, RevisionStatus: Current})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Inventory.Classifications, evidence) || !reflect.DeepEqual(got.Inventory.Patches, snapshot.Inventory.Patches) || !reflect.DeepEqual(got.Inventory.Units, snapshot.Inventory.Units) || got.Generation != updated.Generation || !reflect.DeepEqual(got.ReviewedSliceIDs, []string{f.ID}) {
		t.Fatal("frozen evidence or raw progress changed")
	}
}
