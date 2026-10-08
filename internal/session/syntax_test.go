package session

import (
	"encoding/json"
	"path/filepath"
	"prui/internal/syntax"
	"reflect"
	"testing"
)

func TestSyntaxSurvivesOfflineStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := commitFixture()
	tokens := map[string]syntax.Patch{"unit": {1: {
		Old: []syntax.Span{{Start: 0, End: 3, Kind: syntax.Comment}},
		New: []syntax.Span{{Start: 0, End: 1, Kind: syntax.Function}, {Start: 1, End: 2, Kind: syntax.Type},
			{Start: 2, End: 3, Kind: syntax.Macro}, {Start: 3, End: 4, Kind: syntax.Constant},
			{Start: 4, End: 5, Kind: syntax.Attribute}, {Start: 5, End: 6, Kind: syntax.Builtin}},
	}}}
	snapshot.Inventory.Syntax = tokens
	snapshot.Commits.Entries[0].Diff.Syntax = tokens
	saved, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := s.Create(fixture())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Inventory.Syntax, tokens) || !reflect.DeepEqual(got.Commits.Entries[0].Diff.Syntax, tokens) {
		t.Fatal("syntax lost on restart")
	}
	old, err := s.Load(legacy.ID)
	if err != nil || old.Inventory.Syntax != nil {
		t.Fatal("legacy snapshot changed", err)
	}
}

func TestSyntaxDiscardedBeforeCommitPruning(t *testing.T) {
	original := commitFixture()
	baseline, _ := json.Marshal(sourcePayload(original))
	tokens := map[string]syntax.Patch{"unit": {1: {Old: []syntax.Span{{Start: 0, End: 3, Kind: syntax.Comment}}}}}
	original.Inventory.Syntax = tokens
	original.Commits.Entries[0].Diff.Syntax = tokens
	candidate := original
	if err := boundCommitPayload(&candidate, len(baseline)); err != nil {
		t.Fatal(err)
	}
	if candidate.Inventory.Syntax != nil || candidate.Commits.Entries[0].Diff.Syntax != nil {
		t.Fatal("syntax not dropped")
	}
	if candidate.Commits.Entries[0].Diff == nil || candidate.Commits.Entries[0].Status != original.Commits.Entries[0].Status {
		t.Fatal("optional syntax displaced source")
	}
	if original.Inventory.Syntax == nil || original.Commits.Entries[0].Diff.Syntax == nil {
		t.Fatal("mutated source snapshot")
	}
}
