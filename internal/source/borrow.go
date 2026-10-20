package source

import (
	"context"
	"io/fs"
	"path/filepath"
	"sync"
)

// Directories are validated and created in walk order before files are queued.
// Only object-file hard links run concurrently, with at most four workers and
// four queued entries. Join every worker before the caller can remove the view.
func borrowObjects(ctx context.Context, objects, borrowed string, workers int) error {
	return borrowObjectsSkipping(ctx, objects, borrowed, workers, nil)
}

func borrowObjectsSkipping(ctx context.Context, objects, borrowed string, workers int, skip map[string]bool) error {
	visit := func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && len(skip) > 0 {
			rel, relErr := filepath.Rel(objects, path)
			if relErr != nil {
				return relErr
			}
			if skip[filepath.ToSlash(rel)] {
				return ctx.Err()
			}
		}
		return borrowObjectEntry(ctx, objects, borrowed, path, d, err)
	}
	workers = max(1, min(4, workers))
	if workers == 1 {
		return filepath.WalkDir(objects, func(path string, d fs.DirEntry, err error) error {
			return visit(path, d, err)
		})
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type entry struct {
		path string
		item fs.DirEntry
	}
	jobs := make(chan entry, workers)
	var wg sync.WaitGroup
	var once sync.Once
	var firstError error
	fail := func(err error) {
		if err != nil {
			once.Do(func() { firstError = err; cancel() })
		}
	}
	for range workers {
		wg.Go(func() {
			for job := range jobs {
				fail(visit(job.path, job.item, nil))
			}
		})
	}
	err := filepath.WalkDir(objects, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return borrowObjectEntry(ctx, objects, borrowed, path, d, err)
		}
		select {
		case jobs <- entry{path, d}:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	fail(err)
	close(jobs)
	wg.Wait()
	return firstError
}
