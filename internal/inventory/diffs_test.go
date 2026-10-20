package inventory

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"prui/internal/source"
	"prui/internal/testutil"
)

func TestParallelDiffInventoryPreservesByteAndLineLimits(t *testing.T) {
	_, comparison, repo := fixture(t)
	for _, quota := range []struct {
		name         string
		bytes, lines int
	}{
		{"tiny bytes", 10, 100000},
		{"small reservation", 1024, 100000},
		{"bounded bytes", 16000, 100000},
		{"tiny lines", 50 << 20, 1},
		{"bounded lines", 50 << 20, 64},
	} {
		t.Run(quota.name, func(t *testing.T) {
			limits := source.Defaults()
			limits.ContentBytes, limits.DiffLines = quota.bytes, quota.lines
			serial, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
			if err != nil {
				t.Fatal(err)
			}
			defer serial.Close()
			parallel, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), limits)
			if err != nil {
				t.Fatal(err)
			}
			defer parallel.Close()
			want, err := Build(context.Background(), serialBlobObjects{serial}, comparison, limits)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Build(context.Background(), parallel, comparison, limits)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("parallel work changed bounded inventory", err)
			}
		})
	}
}

func TestDiffReservationBoundsActualMaterializedPatches(t *testing.T) {
	repo := testutil.NewRepo(t)
	var separatedOld, separatedNew strings.Builder
	for i := 0; i < 40; i++ {
		separatedOld.WriteString("old\n" + strings.Repeat("context\n", 12))
		separatedNew.WriteString("new\n" + strings.Repeat("context\n", 12))
	}
	for _, pair := range [][2]string{
		{"", "added\n"},
		{separatedOld.String(), separatedNew.String()},
		{strings.Repeat("long", 4000) + "\nold\n", strings.Repeat("long", 4000) + "\nnew\n"},
		{strings.Repeat("function", 2000) + "\n" + strings.Repeat("  context\n", 12) + "old\n", strings.Repeat("function", 2000) + "\n" + strings.Repeat("  context\n", 12) + "new\n"},
		{strings.Repeat("x", 1<<20), strings.Repeat("y", 1<<20)},
		{strings.Repeat("x", 9000) + "\x00old", strings.Repeat("x", 9000) + "\x00new"},
	} {
		old := repo.GitInput(pair[0], "hash-object", "-w", "--stdin")
		next := repo.GitInput(pair[1], "hash-object", "-w", "--stdin")
		view, err := source.NewView(context.Background(), repo.Dir, source.NewRunner(), source.Defaults())
		if err != nil {
			t.Fatal(err)
		}
		patch, err := view.Git(context.Background(), 50<<20, source.BlobDiffArgs(old, next)...)
		view.Close()
		if err != nil {
			t.Fatal(err)
		}
		hunks, err := parseHunks(patch)
		if err != nil {
			t.Fatal(err)
		}
		materialized, lines := 0, 0
		for _, h := range hunks {
			materialized += len(h.patch)
			lines += bytes.Count(h.patch, []byte{'\n'})
		}
		reservedBytes, reservedLines, fits := diffReservation([]byte(pair[0]), []byte(pair[1]), 50<<20, 100000)
		if !fits || reservedBytes < len(patch) || reservedBytes < materialized || reservedLines < lines {
			t.Fatalf("reservation lost source: raw=%d materialized=%d lines=%d reserved=%d/%d fits=%t", len(patch), materialized, lines, reservedBytes, reservedLines, fits)
		}
	}
}
