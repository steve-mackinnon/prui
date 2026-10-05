package session

import (
	"context"
	"encoding/json"
	"errors"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestDraftRestartComparisonIsolationAndStaleWriter(t *testing.T) {
	s := openSQLiteTestStore(t)
	ctx := context.Background()
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	key := DraftKeyFor(meta)
	d, err := s.LoadDraft(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	d.Summary = "private review summary"
	d.Pending = []source.ReviewComment{{Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "private comment"}}
	d.Attempt = "review"
	d.Attempted = &DraftAttempt{Kind: "review", Review: &source.PullRequestReview{Identity: meta.Identity, CommitID: meta.HeadSHA, Event: "COMMENT", Body: d.Summary, Comments: d.Pending}}
	saved, err := s.SaveDraft(ctx, key, d.Generation, d)
	if err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	got, err := other.LoadDraft(ctx, key)
	if err != nil || got.Summary != d.Summary || got.Attempt != "review" || len(got.Pending) != 1 {
		t.Fatalf("restart: %#v %v", got, err)
	}
	if _, err := other.SaveDraft(ctx, key, d.Generation, d); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("stale writer: %v", err)
	}
	changed := key
	changed.HeadSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if changed.HeadSHA == key.HeadSHA {
		changed.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}
	empty, err := other.LoadDraft(ctx, changed)
	if err != nil || empty.Summary != "" {
		t.Fatalf("retargeted: %#v %v", empty, err)
	}
	saved.Summary = ""
	saved.Pending = nil
	saved.Attempt = ""
	saved.Attempted = nil
	if _, err := s.SaveDraft(ctx, key, saved.Generation, saved); err != nil {
		t.Fatal(err)
	}
	got, err = other.LoadDraft(ctx, key)
	if err != nil || got.Summary != "" {
		t.Fatalf("discard: %#v %v", got, err)
	}
}

func TestDraftDeletionFollowsLastComparisonOwner(t *testing.T) {
	s := openSQLiteTestStore(t)
	snap := fixture()
	snap.Inventory.Comparison.Metadata.BaseRepository = "owner/repo"
	snap.Inventory.Comparison.Metadata.HeadRepository = "owner/repo"
	first, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	key := DraftKeyFor(snap.Inventory.Comparison.Metadata)
	if _, err = s.SaveDraft(context.Background(), key, 0, Draft{Summary: "keep"}); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	d, err := s.LoadDraft(context.Background(), key)
	if err != nil || d.Summary != "keep" {
		t.Fatal("shared comparison draft deleted")
	}
	if err = s.Delete(second.ID); err != nil {
		t.Fatal(err)
	}
	d, err = s.LoadDraft(context.Background(), key)
	if err != nil || d.Generation != 0 {
		t.Fatal("last owner deletion left draft")
	}
}
func TestDraftCorruptionRetainedAndReadonlyNeverWrites(t *testing.T) {
	s := openSQLiteTestStore(t)
	ctx := context.Background()
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository = "owner/repo"
	meta.HeadRepository = "owner/repo"
	key := DraftKeyFor(meta)
	if _, err := s.SaveDraft(ctx, key, 0, Draft{Summary: "secret"}); err != nil {
		t.Fatal(err)
	}
	ro, err := OpenReadOnly(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	d, err := ro.LoadDraft(ctx, key)
	if err != nil || d.Summary != "secret" {
		t.Fatal(d, err)
	}
	if _, err = ro.SaveDraft(ctx, key, d.Generation, d); err == nil {
		t.Fatal("readonly draft write")
	}
	db, _ := s.db.SQL()
	if _, err = db.Exec(`UPDATE review_drafts SET payload=?`, []byte(`{"Version":99}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LoadDraft(ctx, key); err == nil {
		t.Fatal("corrupt draft accepted")
	}
	var payload []byte
	if err = db.QueryRow(`SELECT payload FROM review_drafts`).Scan(&payload); err != nil || string(payload) != `{"Version":99}` {
		t.Fatal("corrupt draft not preserved")
	}
}

func TestDraftUpdatesNeverChangeImmutableSnapshotOrProgress(t *testing.T) {
	s := openSQLiteTestStore(t)
	snap := fixture()
	snap.Inventory.Comparison.Metadata.BaseRepository = "owner/repo"
	snap.Inventory.Comparison.Metadata.HeadRepository = "owner/repo"
	record, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	key := DraftKeyFor(snap.Inventory.Comparison.Metadata)
	if _, err = s.SaveDraft(context.Background(), key, 0, Draft{Summary: "private-draft-sentinel"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SnapshotReference != record.SnapshotReference || got.Generation != record.Generation {
		t.Fatal("draft changed immutable references/progress")
	}
	db, _ := s.db.SQL()
	var payload []byte
	if err = db.QueryRow(`SELECT payload FROM snapshots`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "private-draft-sentinel") {
		t.Fatal("draft entered frozen source payload")
	}
}

func TestExtendedDraftVersionAndInvalidAttemptTargets(t *testing.T) {
	store := openSQLiteTestStore(t)
	ctx := context.Background()
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	k := DraftKeyFor(meta)
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "RIGHT", Line: 3, StartLine: 1, StartSide: "RIGHT"}
	d := Draft{Version: 1, Composer: &DraftEditor{Target: target, Body: "work", PendingIndex: -1}, Attempt: "comment", Attempted: &DraftAttempt{Kind: "comment", Comment: &source.ReviewComment{Target: target, Body: "attempt"}}}
	saved, err := store.SaveDraft(ctx, k, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadDraft(ctx, k)
	if err != nil || got.Version != 2 || got.Composer.Target != target || got.Attempted.Comment.Target != target {
		t.Fatal("target/schema lost", got, err)
	}
	stale := got
	attemptCopy := *got.Attempted
	commentCopy := *attemptCopy.Comment
	commentCopy.Target.CommitID = strings.Repeat("f", 40)
	attemptCopy.Comment = &commentCopy
	stale.Attempted = &attemptCopy
	if _, err := store.SaveDraft(ctx, k, saved.Generation, stale); err == nil {
		t.Fatal("unsupported historical extended attempt accepted")
	}
	got.Attempted.Comment.Target.StartSide = "LEFT"
	if _, err := store.SaveDraft(ctx, k, saved.Generation, got); err == nil {
		t.Fatal("invalid immutable attempt accepted")
	}
	original, err := store.LoadDraft(ctx, k)
	if err != nil || original.Attempted.Comment.Target != target {
		t.Fatal("invalid update destroyed original")
	}
}

func TestSuggestionRestackLegacyRangeAndReplacementPayloadVersions(t *testing.T) {
	store := openSQLiteTestStore(t)
	ctx := context.Background()
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	key := DraftKeyFor(meta)
	target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}
	legacy := Draft{Version: 1, Composer: &DraftEditor{Target: target, Body: "legacy single-line work", PendingIndex: -1}}
	if _, err := store.SaveDraft(ctx, key, 0, legacy); err != nil {
		t.Fatal(err)
	}
	// Simulate an actual previously persisted v1 record, rather than a new save's
	// automatic schema upgrade. Source/session data are never modified.
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.db.SQL()
	if err != nil {
		t.Fatal(err)
	}
	args := append([]any{payload}, draftArgs(key)...)
	if _, err = db.ExecContext(ctx, `UPDATE review_drafts SET payload=? WHERE `+draftWhere, args...); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadDraft(ctx, key)
	if err != nil || got.Version != 1 || got.Composer.Body != legacy.Composer.Body || got.Composer.Target != target {
		t.Fatal("legacy draft lost", got, err)
	}
	got.Composer.Target.StartLine = 1
	got.Composer.Target.StartSide = "RIGHT"
	got.Composer.Target.Line = 3
	saved, err := store.SaveDraft(ctx, key, got.Generation, got)
	if err != nil || saved.Version != 2 {
		t.Fatal("range payload did not use v2", saved, err)
	}
	got, err = store.LoadDraft(ctx, key)
	if err != nil || got.Composer.Target.StartLine != 1 || got.Composer.Target.Line != 3 {
		t.Fatal("range flattened", got, err)
	}
	got.Composer.Suggestion = true
	got.Composer.Before = "old range"
	got.Composer.Body = "edited replacement"
	saved, err = store.SaveDraft(ctx, key, got.Generation, got)
	if err != nil || saved.Version != 3 {
		t.Fatal("replacement not versioned safely", saved, err)
	}
	reopened, err := Open(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err = reopened.LoadDraft(ctx, key)
	if err != nil || got.Version != 3 || !got.Composer.Suggestion || got.Composer.Body != "edited replacement" || got.Composer.Before != "old range" || got.Composer.Target.Line != 3 || got.Composer.Target.StartLine != 1 {
		t.Fatal("combined schema restart lost work", got, err)
	}
}

func TestGeneralDraftVersionCompatibilityAndBounds(t *testing.T) {
	s := openSQLiteTestStore(t)
	ctx := context.Background()
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	k := DraftKeyFor(meta)
	d := Draft{General: &GeneralDraft{Body: "private general", Cursor: 3, ReplyTo: "PR comment:12", AttemptedBody: "original", ObservedIDs: []string{"PR comment:12"}, Uncertain: true}}
	saved, err := s.SaveDraft(ctx, k, 0, d)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadDraft(ctx, k)
	if err != nil || got.Version != 4 || got.General.AttemptedBody != "original" {
		t.Fatal(got, err)
	}
	db, _ := s.db.SQL()
	// The same payload with an older version cannot hide the new private data.
	for _, version := range []int{1, 2, 3} {
		got.Version = version
		if validateDraft(k, got) == nil {
			t.Fatal("general accepted under old schema", version)
		}
	}
	for _, payload := range []string{`{"Version":1,"Summary":"old"}`, `{"Version":2,"Summary":"old"}`, `{"Version":3,"Summary":"old"}`} {
		if _, err = db.Exec(`UPDATE review_drafts SET payload=?`, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		old, err := s.LoadDraft(ctx, k)
		if err != nil || old.Summary != "old" {
			t.Fatal("old version lost", old, err)
		}
	}
	d.General.Cursor = 100
	if _, err = s.SaveDraft(ctx, k, saved.Generation, d); err == nil {
		t.Fatal("invalid cursor accepted")
	}
	d.General.Cursor = 3
	d.General.ObservedIDs = []string{"duplicate", "duplicate"}
	if _, err = s.SaveDraft(ctx, k, saved.Generation, d); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	d.General.ObservedIDs = make([]string, 501)
	if _, err = s.SaveDraft(ctx, k, saved.Generation, d); err == nil {
		t.Fatal("unbounded IDs accepted")
	}
}

func TestGeneralDraftDeletionAndSnapshotPrivacy(t *testing.T) {
	s := openSQLiteTestStore(t)
	snap := fixture()
	snap.Inventory.Comparison.Metadata.BaseRepository = "owner/repo"
	snap.Inventory.Comparison.Metadata.HeadRepository = "owner/repo"
	one, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.Create(snap)
	if err != nil {
		t.Fatal(err)
	}
	key := DraftKeyFor(snap.Inventory.Comparison.Metadata)
	if _, err = s.SaveDraft(context.Background(), key, 0, Draft{General: &GeneralDraft{Body: "private-general-sentinel", Cursor: 3}}); err != nil {
		t.Fatal(err)
	}
	db, _ := s.db.SQL()
	var payload []byte
	if err = db.QueryRow(`SELECT payload FROM snapshots`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "private-general-sentinel") {
		t.Fatal("general draft leaked into snapshot/guide input")
	}
	if err = s.Delete(one.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadDraft(context.Background(), key); err != nil || got.General == nil {
		t.Fatal("other comparison owner lost work", err)
	}
	if err = s.Delete(two.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.LoadDraft(context.Background(), key); err != nil || got.Generation != 0 {
		t.Fatal("last owner left general work", err)
	}
}
