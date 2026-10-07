package main

import (
	"context"
	"errors"
	"prui/internal/inventory"
	"prui/internal/source"
)

type sourceBlobReader interface {
	SourceBlob(context.Context, string, string, int) ([]byte, error)
}
type fileSourceObjects struct {
	reader     sourceBlobReader
	file       inventory.FileChange
	comparison source.PinnedComparison
}

func (o fileSourceObjects) Git(context.Context, int, ...string) ([]byte, error) {
	return nil, errors.New("source loading does not run Git")
}
func (o fileSourceObjects) Blob(ctx context.Context, oid string, limit int) ([]byte, error) {
	repository := o.comparison.Metadata.HeadRepository
	if oid == o.file.OldOID {
		repository = o.comparison.Metadata.BaseRepository
	}
	return o.reader.SourceBlob(ctx, repository, oid, limit)
}
func (a *application) loadFileSource(ctx context.Context, f inventory.FileChange, comparison source.PinnedComparison) (*inventory.FullSource, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	reader, ok := a.gh.(sourceBlobReader)
	if !ok {
		return nil, errors.New("source loading unavailable")
	}
	inv := inventory.Inventory{Files: []inventory.FileChange{f}}
	inv.FullSource = inventory.CaptureFullSource(ctx, fileSourceObjects{reader, f, comparison}, inv, a.limits)
	if _, ok := inv.SourceLines(0, true); !ok {
		return nil, errors.New("OLD source unavailable (connectivity, binary file or source limit); E: retry")
	}
	if _, ok := inv.SourceLines(0, false); !ok {
		return nil, errors.New("NEW source unavailable (connectivity, binary file or source limit); E: retry")
	}
	return inv.FullSource, nil
}
