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
