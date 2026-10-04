package inbox

import (
	"context"
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
