package session

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"prui/internal/source"
)

func TestDraftProcessCrashRecovery(t *testing.T) {
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	key := DraftKeyFor(meta)
	if root := os.Getenv("PRUI_DRAFT_CRASH_FIXTURE"); root != "" {
		s, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		comment := source.ReviewComment{Target: source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, Body: "unsent"}
		d := Draft{Composer: &DraftEditor{Target: comment.Target, Body: comment.Body, PendingIndex: -1}, Attempt: "comment", Attempted: &DraftAttempt{Kind: "comment", Comment: &comment}}
		if _, err = s.SaveDraft(context.Background(), key, 0, d); err != nil {
			t.Fatal(err)
		}
		// Exit without Close or any cleanup, as a terminated process would.
		os.Exit(23)
	}
	root := t.TempDir() + "/private"
	cmd := exec.Command(os.Args[0], "-test.run=^TestDraftProcessCrashRecovery$")
	cmd.Env = append(os.Environ(), "PRUI_DRAFT_CRASH_FIXTURE="+root)
	output, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 23 {
		t.Fatalf("fixture: %s %v", output, err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, err := s.LoadDraft(context.Background(), key)
	if err != nil || d.Composer == nil || d.Composer.Body != "unsent" || d.Attempted == nil || d.Attempted.Comment.Body != "unsent" {
		t.Fatalf("crash lost committed work: %#v %v", d, err)
	}
}

func TestGeneralDraftProcessCrashRecoveryPreservesOtherKinds(t *testing.T) {
	meta := fixture().Inventory.Comparison.Metadata
	meta.BaseRepository, meta.HeadRepository = "owner/repo", "owner/repo"
	key := DraftKeyFor(meta)
	if root := os.Getenv("PRUI_GENERAL_CRASH_FIXTURE"); root != "" {
		s, err := Open(root)
		if err != nil {
			t.Fatal(err)
		}
		target := source.ReviewCommentTarget{Identity: meta.Identity, CommitID: meta.HeadSHA, Path: "a", Side: "RIGHT", Line: 2, StartLine: 1, StartSide: "RIGHT"}
		body, err := source.SuggestionBody("replacement")
		if err != nil {
			t.Fatal(err)
		}
		d := Draft{Summary: "private summary", General: &GeneralDraft{Body: "edited general", Cursor: 3, ReplyTo: "PR comment:10", AttemptedBody: "original general", ObservedIDs: []string{"PR comment:10"}, Uncertain: true}, Pending: []source.ReviewComment{{Target: target, Body: body}}, Composer: &DraftEditor{Target: target, Body: "replacement", Before: "original", Suggestion: true, PendingIndex: -1}, Reply: &DraftEditor{Target: target, Body: "inline reply", CommentID: 1, ReplyToID: 1}}
		if _, err = s.SaveDraft(context.Background(), key, 0, d); err != nil {
			t.Fatal(err)
		}
		os.Exit(23)
	}
	root := t.TempDir() + "/private"
	cmd := exec.Command(os.Args[0], "-test.run=^TestGeneralDraftProcessCrashRecoveryPreservesOtherKinds$")
	cmd.Env = append(os.Environ(), "PRUI_GENERAL_CRASH_FIXTURE="+root)
	output, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 23 {
		t.Fatalf("fixture: %s %v", output, err)
	}
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d, err := s.LoadDraft(context.Background(), key)
	if err != nil || d.Version != 4 || d.General == nil || d.General.Body != "edited general" || d.General.AttemptedBody != "original general" || !d.General.Uncertain || d.General.Cursor != 3 || d.General.ReplyTo != "PR comment:10" || len(d.General.ObservedIDs) != 1 || d.Summary != "private summary" || len(d.Pending) != 1 || d.Pending[0].Target.StartLine != 1 || d.Composer == nil || !d.Composer.Suggestion || d.Composer.Before != "original" || d.Reply == nil || d.Reply.Body != "inline reply" {
		t.Fatalf("crash lost combined private editor state: %#v %v", d, err)
	}
}
