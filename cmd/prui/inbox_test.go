package main

import (
	"context"
	"path/filepath"
	"prui/internal/session"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestInboxOptionsFiltersAndOffline(t *testing.T) {
	o, err := parseOptions([]string{"inbox", "--view", "participated", "--repository", "o/r", "--author", "other", "--review", "required", "--state", "closed", "--draft", "no", "--requests", "team", "--activity", "changed", "--offline", "--plain"})
	if err != nil || !o.Offline || o.Inbox.View != "participated" || o.Inbox.Repository != "o/r" || o.Inbox.Author != "other" || o.Inbox.Requests != "team" {
		t.Fatalf("%+v %v", o, err)
	}
	for _, args := range [][]string{{"inbox", "--offline", "--refresh"}, {"inbox", "--view", "all"}, {"inbox", "--repository", "../r"}, {"inbox", "--author", "a OR b"}} {
		if _, err := parseOptions(args); err == nil {
			t.Fatal(args)
		}
	}
}
func TestInboxOpeningDoesNotBorrowAnotherRepository(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.RememberRepository("o/first", "/wrong"); err != nil {
		t.Fatal(err)
	}
	a := application{store: store}
	id := source.Identity{Repository: "o/second", Number: 2}
	if _, err := a.openInbox(context.Background(), "/wrong", id, nil); err == nil || !strings.Contains(err.Error(), "no checkout for o/second") {
		t.Fatal(err)
	}
	a.offline = true
	a.gh = forbiddenGitHub{}
	if _, err := a.openInbox(context.Background(), "/wrong", id, nil); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
	if _, err := a.inbox(context.Background(), source.InboxOptions{View: "authored"}, true); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
}

func TestInboxFrozenOwnSessionRemainsOfflineAndUnchanged(t *testing.T) {
	a, saved := wiringFixture(t)
	id := saved.Inventory.Comparison.Metadata.Identity
	before, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	original, err := a.store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.offline = true
	a.gh = forbiddenGitHub{}
	opened, err := a.openInbox(context.Background(), "/another-repository", id, nil)
	if err != nil || opened.ID != saved.ID || opened.Inventory.Comparison.Metadata.Identity != id || opened.RevisionStatus != session.Unchecked {
		t.Fatalf("%+v %v", opened, err)
	}
	after, err := a.store.List()
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := a.store.Load(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) || persisted.SnapshotReference != original.SnapshotReference || persisted.Generation != original.Generation || persisted.RevisionStatus != original.RevisionStatus || string(opened.Checkout) != string(original.Checkout) {
		t.Fatal("frozen record changed or borrowed checkout")
	}
}
