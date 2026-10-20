package source

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPinInputsOverlapAndPreserveSerialRunners(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		started := make(chan struct{})
		readDone := false
		view := &View{dir: filepath.Join(t.TempDir(), "view")}
		if err := os.Mkdir(view.dir, 0700); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		metadata := Metadata{Description: "captured"}
		read := func(ctx context.Context) (Metadata, error) {
			if parallel {
				select {
				case <-started:
				case <-ctx.Done():
					return Metadata{}, ctx.Err()
				}
			}
			readDone = true
			return metadata, nil
		}
		prepare := func(context.Context) (*View, error) {
			if !parallel && !readDone {
				return nil, errors.New("serial preparation preceded metadata")
			}
			close(started)
			return view, nil
		}
		got, v, elapsed, err := pinInputs(ctx, parallel, read, prepare)
		if err != nil || got.Description != metadata.Description || v != view || elapsed <= 0 {
			t.Fatalf("pin inputs = %#v, %p, %v, %v", got, v, elapsed, err)
		}
		if err := v.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPinInputsMetadataFailureCancelsJoinsAndCleansView(t *testing.T) {
	started, done := make(chan struct{}), make(chan struct{})
	view := &View{dir: filepath.Join(t.TempDir(), "view")}
	if err := os.Mkdir(view.dir, 0700); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("metadata unavailable")
	read := func(ctx context.Context) (Metadata, error) {
		select {
		case <-started:
			return Metadata{}, failure
		case <-ctx.Done():
			return Metadata{}, ctx.Err()
		}
	}
	prepare := func(ctx context.Context) (*View, error) { close(started); <-ctx.Done(); close(done); return view, nil }
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, v, _, err := pinInputs(ctx, true, read, prepare)
	if !errors.Is(err, failure) || v != nil {
		t.Fatalf("failure = %p, %v", v, err)
	}
	select {
	case <-done:
	default:
		t.Fatal("view worker not joined")
	}
	if _, err := os.Stat(view.dir); !os.IsNotExist(err) {
		t.Fatal("speculative view retained")
	}
}

func TestPinInputsFailurePrecedenceAndNoSerialPreparation(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		failure := errors.New("metadata unavailable")
		read := func(context.Context) (Metadata, error) { return Metadata{}, failure }
		prepare := func(context.Context) (*View, error) {
			if !parallel {
				t.Error("serial metadata failure prepared a view")
			}
			return nil, ErrLimit
		}
		_, v, _, err := pinInputs(context.Background(), parallel, read, prepare)
		if v != nil || !errors.Is(err, failure) {
			t.Fatal("metadata failure precedence changed")
		}
	}
	read := func(context.Context) (Metadata, error) { return Metadata{Description: "valid"}, nil }
	prepare := func(context.Context) (*View, error) { return nil, ErrLimit }
	_, v, _, err := pinInputs(context.Background(), true, read, prepare)
	if v != nil || !errors.Is(err, ErrLimit) {
		t.Fatal("view resource failure hidden")
	}
	failed := &View{dir: filepath.Join(t.TempDir(), "failed")}
	if err := os.Mkdir(failed.dir, 0700); err != nil {
		t.Fatal(err)
	}
	prepare = func(context.Context) (*View, error) { return failed, ErrLimit }
	_, v, _, err = pinInputs(context.Background(), true, read, prepare)
	if v != nil || !errors.Is(err, ErrLimit) {
		t.Fatal("failed view returned")
	}
	if _, err := os.Stat(failed.dir); !os.IsNotExist(err) {
		t.Fatal("failed view retained")
	}
}

func TestPinInputsParentCancellationCleansCompletedView(t *testing.T) {
	view := &View{dir: filepath.Join(t.TempDir(), "view")}
	if err := os.Mkdir(view.dir, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	prepare := func(context.Context) (*View, error) { close(ready); return view, nil }
	read := func(context.Context) (Metadata, error) { <-ready; cancel(); return Metadata{}, nil }
	_, v, _, err := pinInputs(ctx, true, read, prepare)
	if v != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("parent cancellation lost")
	}
	if _, err := os.Stat(view.dir); !os.IsNotExist(err) {
		t.Fatal("cancelled preparation retained")
	}
	read = func(context.Context) (Metadata, error) {
		t.Error("read after cancellation")
		return Metadata{}, ErrCommand
	}
	prepare = func(context.Context) (*View, error) {
		t.Error("preparation after cancellation")
		return nil, ErrCommand
	}
	if _, _, _, err := pinInputs(ctx, true, read, prepare); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
