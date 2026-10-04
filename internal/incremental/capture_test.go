package incremental

import (
	"context"
	"errors"
	"prui/internal/source"
	"testing"
)

// Repository changes, unavailable objects, failed freshness and cancellation
// cannot be confused with a captured empty head comparison.
func TestCaptureUnavailableIdentityAndFreshness(t *testing.T) {
	r, old, next, _ := fixture(t)
	v, err := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	before, after := old.Comparison.Metadata, next.Comparison.Metadata
	after.HeadRepository = "fork/repo"
	b, err := Capture(context.Background(), v, "id", "ref", before, after, nil, source.Defaults(), nil)
	if err != nil || b.Status != "unavailable" || b.Diff != nil {
		t.Fatalf("fork %#v %v", b, err)
	}
	after = next.Comparison.Metadata
	before.HeadSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	b, err = Capture(context.Background(), v, "id", "ref", before, after, nil, source.Defaults(), nil)
	if err != nil || b.Status != "unavailable" || b.Diff != nil {
		t.Fatalf("missing %#v %v", b, err)
	}
	before = old.Comparison.Metadata
	b, err = Capture(context.Background(), v, "id", "ref", before, after, failedGH{}, source.Defaults(), nil)
	if err != nil || b.Status != "unavailable" || b.Diff != nil {
		t.Fatalf("freshness %#v %v", b, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Capture(ctx, v, "id", "ref", before, after, nil, source.Defaults(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
}

type failedGH struct{}

func (failedGH) Metadata(context.Context, source.Identity) (source.Metadata, error) {
	return source.Metadata{}, errors.New("unavailable")
}
func (failedGH) ListPullRequests(context.Context, string) ([]source.PullRequest, error) {
	panic("unexpected")
}
func (failedGH) Token(context.Context) (string, error) { panic("unexpected") }
