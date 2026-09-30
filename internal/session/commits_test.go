package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"prui/internal/commits"
	reviewcontext "prui/internal/context"
	"prui/internal/inventory"
)

func commitFixture() Snapshot {
	s := fixture()
	f := s.Inventory.Files[0]
	f.OldOID, f.NewOID = strings.Repeat("0", 40), strings.Repeat("d", 40)
	f.OldMode, f.NewMode, f.Status = "000000", "100644", "A"
	s.Commits = &commits.Bundle{BaseSHA: s.Inventory.Comparison.Metadata.BaseSHA, HeadSHA: s.Inventory.Comparison.Metadata.HeadSHA, Status: commits.Captured, Complete: true, Entries: []commits.Entry{{SHA: strings.Repeat("c", 40), Subject: "Change", Author: "Alice", Parents: []string{strings.Repeat("a", 40)}, Status: commits.Captured, Diff: &commits.Diff{Files: []inventory.FileChange{f}, Units: append([]inventory.ReviewUnit(nil), s.Inventory.Units...), Patches: map[string][]byte{s.Inventory.Units[0].PatchReference: s.Inventory.Patches[s.Inventory.Units[0].PatchReference]}, Complete: true}}}}
	return s
}

func TestCommitBundleSurvivesOfflineRestartAndSourceReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := commitFixture()
	rec, err := store.Create(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Load(rec.ID)
	if err != nil || !reflect.DeepEqual(got.Commits, snapshot.Commits) {
		t.Fatalf("restarted commits = %#v, %v", got, err)
	}
	cached, err := store.LoadComparisonSnapshot(snapshot.Inventory.Comparison.Metadata)
	if err != nil || cached == nil || !reflect.DeepEqual(cached.Commits, snapshot.Commits) {
		t.Fatalf("cached commits = %#v, %v", cached, err)
	}
	derived := got.Snapshot
	bundle := generatedGuide()
	derived.Guides = &bundle
	derived.DerivedFrom = got.ID
	copied, err := store.Create(derived)
	if err != nil || !reflect.DeepEqual(copied.Commits, snapshot.Commits) {
		t.Fatalf("derived commits = %#v, %v", copied, err)
	}
}

func TestAbsentCommitBundlePreservesLegacyCanonicalSourceBytes(t *testing.T) {
	s := fixture()
	// This is the source schema that existed before commit bundles. Its field order
	// deliberately does not depend on sqliteSourcePayload.
	old := struct {
		Version                int                         `json:"version"`
		Inventory              inventory.Inventory         `json:"inventory"`
		Slices                 []Slice                     `json:"slices"`
		UnitFiles              []int                       `json:"unit_files"`
		Context                reviewcontext.ContextBundle `json:"context"`
		PullRequestDescription *string                     `json:"pull_request_description,omitempty"`
	}{1, s.Inventory, s.Slices, s.UnitFiles, s.Context, nil}
	legacy, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	key, current, err := encodeSource(s)
	if err != nil || !bytes.Equal(current, legacy) || key != digest(legacy) {
		t.Fatalf("legacy source changed: %v", err)
	}
	decoded, err := decodeSource(digest(legacy), legacy)
	if err != nil || decoded.Commits != nil {
		t.Fatalf("legacy decode = %#v, %v", decoded, err)
	}
	_, again, err := encodeSource(decoded)
	if err != nil || !bytes.Equal(again, legacy) {
		t.Fatalf("legacy reencode changed: %v", err)
	}
}

func TestStoreRejectsInvalidCommitBundles(t *testing.T) {
	cases := map[string]func(*commits.Bundle){
		"oversized subject":             func(b *commits.Bundle) { b.Entries[0].Subject = strings.Repeat("a", 4097) },
		"duplicate parent":              func(b *commits.Bundle) { b.Entries[0].Parents = append(b.Entries[0].Parents, b.Entries[0].Parents[0]) },
		"unavailable list with entries": func(b *commits.Bundle) { b.Status = commits.Unavailable; b.Reason = "offline"; b.Complete = false },
		"too many entries": func(b *commits.Bundle) {
			entry := b.Entries[0]
			b.Entries = nil
			for i := 0; i < 101; i++ {
				entry.SHA = fmt.Sprintf("%040x", i+1)
				b.Entries = append(b.Entries, entry)
			}
		},
		"capped marked complete": func(b *commits.Bundle) {
			entry := b.Entries[0]
			b.Entries = nil
			for i := 0; i < 100; i++ {
				entry.SHA = fmt.Sprintf("%040x", i+1)
				b.Entries = append(b.Entries, entry)
			}
		},
		"bad file oid":                   func(b *commits.Bundle) { b.Entries[0].Diff.Files[0].NewOID = "invalid" },
		"bad file status":                func(b *commits.Bundle) { b.Entries[0].Diff.Files[0].Status = "X" },
		"bad file mode":                  func(b *commits.Bundle) { b.Entries[0].Diff.Files[0].NewMode = "invalid" },
		"mismatched pin":                 func(b *commits.Bundle) { b.HeadSHA = strings.Repeat("d", 40) },
		"bad status":                     func(b *commits.Bundle) { b.Status = "mystery" },
		"duplicate commit":               func(b *commits.Bundle) { b.Entries = append(b.Entries, b.Entries[0]) },
		"invalid sha":                    func(b *commits.Bundle) { b.Entries[0].SHA = strings.Repeat("C", 40) },
		"invalid parent":                 func(b *commits.Bundle) { b.Entries[0].Parents = []string{"bad"} },
		"invalid subject":                func(b *commits.Bundle) { b.Entries[0].Subject = string([]byte{255}) },
		"oversized author":               func(b *commits.Bundle) { b.Entries[0].Author = strings.Repeat("a", 257) },
		"unavailable diff with material": func(b *commits.Bundle) { b.Entries[0].Status = commits.Unavailable; b.Entries[0].Reason = "missing" },
		"captured diff missing":          func(b *commits.Bundle) { b.Entries[0].Diff = nil },
		"dangling file":                  func(b *commits.Bundle) { b.Entries[0].Diff.Units[0].FileChangeID = "missing" },
		"duplicate unit":                 func(b *commits.Bundle) { d := b.Entries[0].Diff; d.Units = append(d.Units, d.Units[0]) },
		"corrupt patch": func(b *commits.Bundle) {
			for ref := range b.Entries[0].Diff.Patches {
				b.Entries[0].Diff.Patches[ref] = []byte("changed")
			}
		},
		"orphan patch":       func(b *commits.Bundle) { p := []byte("extra"); b.Entries[0].Diff.Patches[digest(p)] = p },
		"negative range":     func(b *commits.Bundle) { b.Entries[0].Diff.Units[0].OldRange.Start = -1 },
		"false completeness": func(b *commits.Bundle) { b.Entries[0].Diff.Problems = []string{"limited"} },
		"too many patch lines": func(b *commits.Bundle) {
			p := bytes.Repeat([]byte("+x\n"), 100001)
			d := b.Entries[0].Diff
			d.Patches = map[string][]byte{digest(p): p}
			d.Units[0].PatchReference = digest(p)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := commitFixture()
			mutate(s.Commits)
			store, err := Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if _, err := store.Create(s); err == nil {
				t.Fatal("invalid commits were saved")
			}
		})
	}
}

func TestBoundCommitPayloadKeepsListWhenPatchEncodingExceedsStorageBudget(t *testing.T) {
	s := commitFixture()
	minimal := commitFixture()
	minimal.Commits.Entries[0].Status = commits.Unavailable
	minimal.Commits.Entries[0].Reason = "commit source storage limit"
	minimal.Commits.Entries[0].Diff = nil
	_, small, err := encodeSource(minimal)
	if err != nil {
		t.Fatal(err)
	}
	if err := boundCommitPayload(&s, len(small)); err != nil {
		t.Fatal(err)
	}
	_, encoded, err := encodeSource(s)
	if err != nil || len(encoded) > len(small) || !reflect.DeepEqual(s.Commits, minimal.Commits) {
		t.Fatalf("bounded bundle = %#v, size=%d: %v", s.Commits, len(encoded), err)
	}
	if !reflect.DeepEqual(s.Inventory, fixture().Inventory) {
		t.Fatal("PR inventory changed")
	}
}

func TestBoundCommitPayloadDoesNotAlterSourceThatFits(t *testing.T) {
	s := commitFixture()
	before := commitFixture()
	if err := BoundCommitPayload(&s); err != nil || !reflect.DeepEqual(s, before) {
		t.Fatalf("source changed: %v", err)
	}
}

func TestCommitSourceDecoderRejectsUnknownFieldsAndInvalidFrozenReferences(t *testing.T) {
	s := commitFixture()
	_, encoded, err := encodeSource(s)
	if err != nil {
		t.Fatal(err)
	}
	extra := bytes.Replace(encoded, []byte(`"commits":{`), []byte(`"commits":{"unexpected":1,`), 1)
	if _, err := decodeSource(digest(extra), extra); err == nil {
		t.Fatal("unknown commit field accepted")
	}
	s.Commits.Entries[0].Diff.Units[0].FileChangeID = "missing"
	_, invalid, err := encodeSource(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSource(digest(invalid), invalid); err == nil {
		t.Fatal("invalid frozen commit reference decoded")
	}
}

func TestCommitBundleSafeStatesRoundTrip(t *testing.T) {
	for _, state := range []string{"empty", "unavailable list", "unavailable diff", "partial diff", "capped"} {
		t.Run(state, func(t *testing.T) {
			s := commitFixture()
			switch state {
			case "empty":
				s.Commits.Entries = []commits.Entry{}
			case "unavailable list":
				s.Commits.Status = commits.Unavailable
				s.Commits.Reason = "offline"
				s.Commits.Complete = false
				s.Commits.Entries = nil
			case "unavailable diff":
				s.Commits.Entries[0].Status = commits.Unavailable
				s.Commits.Entries[0].Reason = "missing object"
				s.Commits.Entries[0].Diff = nil
			case "partial diff":
				s.Commits.Entries[0].Diff.Complete = false
				s.Commits.Entries[0].Diff.Problems = []string{"limited"}
			case "capped":
				entry := s.Commits.Entries[0]
				s.Commits.Complete = false
				s.Commits.Entries = nil
				for i := 0; i < 100; i++ {
					entry.SHA = fmt.Sprintf("%040x", i+1)
					s.Commits.Entries = append(s.Commits.Entries, entry)
				}
			}
			store, err := Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			rec, err := store.Create(s)
			if err != nil {
				t.Fatal(err)
			}
			got, err := store.Load(rec.ID)
			if err != nil || !reflect.DeepEqual(got.Commits, s.Commits) {
				t.Fatalf("safe state did not survive: %#v, %v", got, err)
			}
		})
	}
}

func TestCommitDiffPreservesLongGitPathsAndPaddedRenameScores(t *testing.T) {
	for _, status := range []string{"M", "R050"} {
		t.Run(status, func(t *testing.T) {
			s := commitFixture()
			d := s.Commits.Entries[0].Diff
			d.Files[0].OldPath = bytes.Repeat([]byte("nested/"), 700)
			d.Files[0].NewPath = append(bytes.Clone(d.Files[0].OldPath), []byte("renamed.txt")...)
			d.Files[0].Status = status
			d.Files[0].OldMode = "100644"
			d.Files[0].OldOID = strings.Repeat("e", 40)
			d.Units = []inventory.ReviewUnit{{ID: "metadata", InventoryID: "commit-inventory", FileChangeID: d.Files[0].ID, Kind: inventory.FileMetadata}}
			d.Patches = nil
			store, err := Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			rec, err := store.Create(s)
			if err != nil {
				t.Fatal(err)
			}
			got, err := store.Load(rec.ID)
			if err != nil || !reflect.DeepEqual(got.Commits, s.Commits) {
				t.Fatalf("Git path/status changed: %#v, %v", got, err)
			}
		})
	}
}

func TestCommitSourceRejectsHiddenPatchesOnInformationalUnits(t *testing.T) {
	for _, kind := range []inventory.Kind{inventory.FileMetadata, inventory.Binary, inventory.Gitlink} {
		t.Run(string(kind), func(t *testing.T) {
			s := commitFixture()
			// Keep a valid patch digest and map entry so reference integrity alone cannot
			// accept source material that the informational renderer will never display.
			s.Commits.Entries[0].Diff.Units[0].Kind = kind
			_, payload, err := encodeSource(s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeSource(digest(payload), payload); err == nil {
				t.Fatal("informational unit with hidden patch accepted")
			}
			store, err := Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if _, err := store.Create(s); err == nil {
				t.Fatal("informational unit with hidden patch saved")
			}
		})
	}
}

func TestCommitPayloadBoundMatchesCanonicalCodecContribution(t *testing.T) {
	for _, count := range []int{0, 1, 25, 100} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			original := commitFixture()
			original.Commits.Entries = nil
			for i := 0; i < count; i++ {
				entry := commitFixture().Commits.Entries[0]
				entry.SHA = fmt.Sprintf("%040x", i+1)
				entry.Subject = "escaped <tag> & \"label\" \\ unicode λ"
				original.Commits.Entries = append(original.Commits.Entries, entry)
			}
			original.Commits.Complete = count < 100
			before, _ := json.Marshal(original.Commits)
			baseline := original
			baseline.Commits = nil
			_, baseBytes, err := encodeSource(baseline)
			if err != nil {
				t.Fatal(err)
			}
			full, _ := json.Marshal(sourcePayload(original))
			if len(full) != len(baseBytes)+len(`,"commits":`)+len(before) {
				t.Fatal("optional payload contribution changed")
			}
			for _, limit := range []int{len(full), len(full) - 1, len(baseBytes) + 200, len(baseBytes)} {
				candidate := original
				if err := boundCommitPayload(&candidate, limit); err != nil {
					t.Fatal(err)
				}
				actual, _ := json.Marshal(sourcePayload(candidate))
				if len(actual) > limit {
					t.Fatalf("encoded size=%d exceeds %d", len(actual), limit)
				}
				// At the exact full size no material may be discarded.
				if limit == len(full) && !reflect.DeepEqual(candidate.Commits, original.Commits) {
					t.Fatal("exact-size bundle pruned")
				}
				after, _ := json.Marshal(original.Commits)
				if !bytes.Equal(before, after) {
					t.Fatal("immutable source bundle mutated")
				}
			}
		})
	}
}

func BenchmarkBoundCommitPayloadManyEntries(b *testing.B) {
	original := commitFixture()
	original.Inventory.Patches = map[string][]byte{"baseline": bytes.Repeat([]byte("x"), 1<<20)}
	entry := original.Commits.Entries[0]
	original.Commits.Entries = nil
	for i := 0; i < 100; i++ {
		original.Commits.Entries = append(original.Commits.Entries, entry)
	}
	baseline := original
	baseline.Commits = nil
	encoded, _ := json.Marshal(sourcePayload(baseline))
	limit := len(encoded) + 200
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidate := original
		if err := boundCommitPayload(&candidate, limit); err != nil {
			b.Fatal(err)
		}
	}
}
