package inbox

import (
	"context"
	"os"
	"path/filepath"
	"prui/internal/source"
	"testing"
	"time"
)

func TestCacheRestartAndExplicitRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.sqlite")
	ctx := context.Background()
	o := source.InboxOptions{View: "authored"}
	r := source.Inbox{Viewer: "me", Complete: true, ObservedAt: time.Now(), Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: source.Identity{Repository: "o/r", Number: 1}}, UpdatedAt: time.Now(), HeadSHA: "a"}}}
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	got, err := c.Load(ctx, o)
	if err != nil || got.Items[0].Activity != "unknown" || !got.Cached {
		t.Fatalf("%+v %v", got, err)
	}
	if err := c.MarkRead(ctx, got.Viewer, got.Items[0]); err != nil {
		t.Fatal(err)
	}
	got, err = c.Load(ctx, o)
	if err != nil || got.Items[0].Activity != "read" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r.ObservedAt = r.ObservedAt.Add(time.Second)
	r.Items[0].HeadSHA = "b"
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	got, err = c.Load(ctx, o)
	if err != nil || got.Items[0].Activity != "changed" {
		t.Fatalf("%+v %v", got, err)
	}
	r.Viewer = "other"
	r.ObservedAt = r.ObservedAt.Add(time.Second)
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	got, err = c.Load(ctx, o)
	if err != nil || got.Items[0].Activity != "unknown" {
		t.Fatalf("account leaked %+v %v", got, err)
	}
}

func TestCacheStaleCaptureAndCrossViewActivity(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "inbox.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	o := source.InboxOptions{View: "requested"}
	r := source.Inbox{Viewer: "me", Complete: true, ObservedAt: time.Now(), Items: []source.InboxItem{{PullRequest: source.PullRequest{Identity: source.Identity{Repository: "o/r", Number: 1}}, UpdatedAt: time.Now(), HeadSHA: "a", TeamRequest: true}}}
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	if err := c.MarkRead(ctx, r.Viewer, r.Items[0]); err != nil {
		t.Fatal(err)
	}
	stale := r
	stale.ObservedAt = r.ObservedAt.Add(-time.Second)
	stale.Items = append([]source.InboxItem(nil), r.Items...)
	stale.Items[0].HeadSHA = "old"
	if err := c.Save(ctx, o, stale); err != nil {
		t.Fatal(err)
	}
	got, err := c.Load(ctx, o)
	if err != nil || got.Items[0].HeadSHA != "a" || got.Items[0].Activity != "read" {
		t.Fatal(got, err)
	}
	o.View = "authored"
	r.Items[0].TeamRequest = false
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	got, err = c.Load(ctx, o)
	if err != nil || got.Items[0].Activity != "read" {
		t.Fatal("query noise changed read evidence", got, err)
	}
}
func TestCacheRejectsCorruptionAndPrivateFileViolations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inbox.sqlite")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	o := source.InboxOptions{View: "authored"}
	r := source.Inbox{Viewer: "me", Complete: true, ObservedAt: time.Now()}
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.Exec("UPDATE captures SET payload='{}'"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(ctx, o); err == nil {
		t.Fatal("corrupt cache accepted")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(path); err == nil {
		opened.Close()
		t.Fatal("public cache accepted")
	}
	link := filepath.Join(dir, "link.sqlite")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(link); err == nil {
		opened.Close()
		t.Fatal("symlink cache accepted")
	}
}

func TestCacheCapturesRemainAccountScoped(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "inbox.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := context.Background()
	o := source.InboxOptions{View: "authored"}
	r := source.Inbox{Viewer: "A", Complete: true, ObservedAt: time.Now()}
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	r.Viewer = "B"
	r.ObservedAt = r.ObservedAt.Add(time.Second)
	if err := c.Save(ctx, o, r); err != nil {
		t.Fatal(err)
	}
	got, err := c.LoadViewer(ctx, o, "A")
	if err != nil || got.Viewer != "A" {
		t.Fatal("account A replaced", got, err)
	}
	got, err = c.LoadViewer(ctx, o, "B")
	if err != nil || got.Viewer != "B" {
		t.Fatal("account B replaced", got, err)
	}
	o.Account = "A"
	got, err = c.Load(ctx, o)
	if err != nil || got.Viewer != "A" {
		t.Fatal("explicit offline selection lost", got, err)
	}
}
