package review

import (
	"context"
	"pr-review/internal/inventory"
	"pr-review/internal/session"
	"pr-review/internal/source"
)

type Slice = session.Slice
type Session = session.Record

func Open(ctx context.Context, checkout string, id source.Identity, gh source.GitHub, r source.Runner, l source.Limits, notify func(string)) (*Session, error) {
	v, p, e := source.Pin(ctx, checkout, id, gh, r, l, notify)
	if e != nil {
		return nil, e
	}
	defer v.Close()
	inv, e := inventory.Build(ctx, v, p, l)
	if e != nil {
		return nil, e
	}
	s := &Session{Snapshot: session.Snapshot{Inventory: inv, PlanVersion: "file-v1", Checkout: []byte(checkout), Slices: make([]Slice, len(inv.Files)), UnitFiles: make([]int, len(inv.Units))}}
	index := map[string]int{}
	for i, f := range inv.Files {
		index[f.ID] = i
		s.Slices[i] = Slice{FileID: f.ID, Units: []int{}}
	}
	for i, u := range inv.Units {
		f := index[u.FileChangeID]
		s.Slices[f].Units = append(s.Slices[f].Units, i)
		s.UnitFiles[i] = f
	}
	return s, nil
}
