package main

import (
	"context"
	"prui/internal/source"
	"testing"
)

type readinessGH struct {
	fixtureGH
	reads         int
	metadataCalls int
}

func (g *readinessGH) ReadReadiness(_ context.Context, id source.Identity) (source.Readiness, error) {
	g.reads++
	return source.Readiness{Identity: id, HeadSHA: "live-head"}, nil
}
func TestReadinessOfflineRefusesBeforeClientAndDoesNotUsePinnedHead(t *testing.T) {
	a, s := wiringFixture(t)
	g := &readinessGH{}
	a.gh = g
	a.offline = true
	id := s.Inventory.Comparison.Metadata.Identity
	if _, e := a.readReadiness(context.Background(), id); e == nil || g.reads != 0 || g.metadataCalls != 0 {
		t.Fatal("offline accessed client")
	}
	a.offline = false
	r, e := a.readReadiness(context.Background(), id)
	if e != nil || r.HeadSHA != "live-head" || g.reads != 1 || g.metadataCalls != 0 {
		t.Fatal("readiness tied to frozen pins", r, e)
	}
}

func (g *readinessGH) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	g.metadataCalls++
	return g.fixtureGH.Metadata(ctx, id)
}
