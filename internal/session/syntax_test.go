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
	// The exact retained baseline now includes the required omission diagnostic.
	expected := commitFixture()
	statuses := map[string]syntax.FileStatus{"file": {Old: syntax.StorageLimit, New: syntax.StorageLimit}}
	expected.Inventory.SyntaxStatus = statuses
	expected.Commits.Entries[0].Diff.SyntaxStatus = statuses
	baseline, _ := json.Marshal(sourcePayload(expected))
	original := commitFixture()
	tokens := map[string]syntax.Patch{"unit": {1: {Old: []syntax.Span{
		{Start: 0, End: 1, Kind: syntax.Keyword}, {Start: 1, End: 2, Kind: syntax.String}, {Start: 2, End: 3, Kind: syntax.Comment},
	}}}}
	original.Inventory.Syntax = tokens
	original.Commits.Entries[0].Diff.Syntax = tokens
	full, _ := json.Marshal(sourcePayload(original))
	if len(full) <= len(baseline) {
		t.Fatal("fixture did not exceed the retained-source budget")
	}
	candidate := original
	if err := boundCommitPayload(&candidate, len(baseline)); err != nil {
		t.Fatal(err)
	}
	if candidate.Commits.Entries[0].Diff == nil {
		t.Fatal("optional syntax displaced source")
	}
	got, _ := json.Marshal(sourcePayload(candidate))
	if string(got) != string(baseline) {
		t.Fatal("pruning changed source or lost the omission diagnostic")
	}
	if original.Inventory.Syntax == nil || original.Commits.Entries[0].Diff.Syntax == nil || original.Inventory.SyntaxStatus != nil || original.Commits.Entries[0].Diff.SyntaxStatus != nil {
		t.Fatal("mutated source snapshot")
	}
}

func TestSyntaxStatusSurvivesOfflineStoreRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := commitFixture()
	statuses := map[string]syntax.FileStatus{"file": {Old: syntax.Ready, New: syntax.Timeout}}
	snapshot.Inventory.SyntaxStatus = statuses
	snapshot.Commits.Entries[0].Diff.SyntaxStatus = statuses
	saved, err := s.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Inventory.SyntaxStatus, statuses) || !reflect.DeepEqual(got.Commits.Entries[0].Diff.SyntaxStatus, statuses) {
		t.Fatal("capture reasons lost offline")
	}
}

func TestSyntaxPruningKeepsDiagnosticWithoutMutatingOriginal(t *testing.T) {
	original := commitFixture()
	statuses := map[string]syntax.FileStatus{"file": {Old: syntax.Ready, New: syntax.Timeout}}
	original.Inventory.SyntaxStatus = statuses
	original.Commits.Entries[0].Diff.SyntaxStatus = statuses
	baseline, _ := json.Marshal(sourcePayload(original))
	tokens := map[string]syntax.Patch{"unit": {1: {Old: []syntax.Span{{Start: 0, End: 3, Kind: syntax.Comment}}}}}
	original.Inventory.Syntax = tokens
	original.Commits.Entries[0].Diff.Syntax = tokens
	candidate := original
	if err := boundCommitPayload(&candidate, len(baseline)+100); err != nil {
		t.Fatal(err)
	}
	got := candidate.Inventory.SyntaxStatus["file"]
	if got.Old != syntax.StorageLimit || got.New != syntax.Timeout {
		t.Fatalf("lost diagnostics: %#v", got)
	}
	if candidate.Commits.Entries[0].Diff.SyntaxStatus["file"] != got {
		t.Fatal("commit drop reason missing")
	}
	if original.Inventory.SyntaxStatus["file"].Old != syntax.Ready || original.Commits.Entries[0].Diff.SyntaxStatus["file"].Old != syntax.Ready {
		t.Fatal("mutated original metadata")
	}
}
