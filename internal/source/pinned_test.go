package source

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pr-review/internal/testutil"
)

type fakeGH struct {
	values        []Metadata
	calls, tokens int
	tokenErr      error
}

func (g *fakeGH) Metadata(context.Context, Identity) (Metadata, error) {
	m := g.values[min(g.calls, len(g.values)-1)]
	g.calls++
	return m, nil
}
func (g *fakeGH) Token(context.Context) (string, error) {
	g.tokens++
	return "synthetic-token", g.tokenErr
}
func metadata(base, head string) Metadata {
	return Metadata{Identity: Identity{"owner/repo", 42}, BaseRepository: "owner/repo", HeadRepository: "fork/repo", BaseSHA: base, HeadSHA: head}
}

type fixtureRunner struct {
	t        *testing.T
	remote   string
	requests []Request
	fetchErr error
}

func (r *fixtureRunner) Run(ctx context.Context, q Request) ([]byte, error) {
	r.requests = append(r.requests, q)
	isFetch := false
	for _, a := range q.Args {
		if a == "fetch" {
			isFetch = true
		}
	}
	if !isFetch {
		return NewRunner().Run(ctx, q)
	}
	args := strings.Join(q.Args, " ")
	env := strings.Join(q.Env, "\n")
	if strings.Contains(args, "synthetic-token") || !strings.Contains(env, base64.StdEncoding.EncodeToString([]byte("x-access-token:synthetic-token"))) {
		r.t.Fatal("invalid credential transport")
	}
	if !strings.Contains(args, "https://github.com/") || !strings.Contains(args, "--no-auto-maintenance") || !strings.Contains(args, "--recurse-submodules=no") || q.StorageLimit == 0 {
		r.t.Fatal("unsafe fetch", args)
	}
	if r.fetchErr != nil {
		return nil, r.fetchErr
	}
	// The fake network boundary copies synthetic objects; production HTTPS is never invoked.
	err := filepath.WalkDir(filepath.Join(r.remote, ".git/objects"), func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(filepath.Join(r.remote, ".git/objects"), p)
		if e != nil {
			return e
		}
		dest := filepath.Join(q.Dir, "objects", rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(dest, b, 0600)
	})
	return nil, err
}

func TestPinnedDivergentFork(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("common", "common\n")
	common := r.Commit()
	r.Write("base-only", "base\n")
	base := r.Commit()
	r.Git("checkout", "-b", "feature", common)
	r.Write("head-only", "head\n")
	head := r.Commit()
	r.Write("head-only", "dirty\n")
	before := r.Snapshot()
	g := &fakeGH{values: []Metadata{metadata(base, head)}}
	v, p, e := Pin(context.Background(), r.Dir, g.values[0].Identity, g, NewRunner(), Defaults(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if p.MergeBaseSHA != common || p.Metadata.HeadRepository != "fork/repo" || g.tokens != 0 {
		t.Fatal("incorrect pinned comparison", p)
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("checkout changed")
	}
}

func TestPinnedAmbiguousMergeBases(t *testing.T) {
	r := testutil.NewRepo(t)
	r.Write("file", "same\n")
	root := r.Commit()
	tree := r.Git("rev-parse", root+"^{tree}")
	a := r.Git("commit-tree", tree, "-p", root, "-m", "a")
	b := r.Git("commit-tree", tree, "-p", root, "-m", "b")
	left := r.Git("commit-tree", tree, "-p", a, "-p", b, "-m", "left")
	right := r.Git("commit-tree", tree, "-p", b, "-p", a, "-m", "right")
	g := &fakeGH{values: []Metadata{metadata(left, right)}}
	_, _, e := Pin(context.Background(), r.Dir, g.values[0].Identity, g, NewRunner(), Defaults(), nil)
	if !errors.Is(e, ErrAmbiguous) || g.tokens != 0 {
		t.Fatal("ambiguous comparison fetched or accepted", e)
	}
}

func TestPinnedShallowMissingAndRaces(t *testing.T) {
	remote := testutil.NewRepo(t)
	remote.Write("f", "old\n")
	base := remote.Commit()
	remote.Write("f", "new\n")
	head := remote.Commit()
	remote.Write("f", "newer\n")
	newer := remote.Commit()
	for _, tc := range []struct {
		name   string
		race   bool
		repeat bool
	}{{"missing", false, false}, {"race", true, false}, {"repeated-race", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			local := testutil.NewRepo(t)
			// A real depth-one clone has a tip object but not its parents.
			local.Git("fetch", "--depth=1", "file://"+remote.Dir, head)
			before := local.Snapshot()
			values := []Metadata{metadata(base, head)}
			if tc.race {
				values = append(values, metadata(base, newer))
			}
			if tc.repeat {
				values = append(values, metadata(base, head))
			}
			g := &fakeGH{values: values}
			runner := &fixtureRunner{t: t, remote: remote.Dir}
			// Fail first fetch on races so retry must rebuild the view and fetch the new pin.
			if tc.race {
				runner.fetchErr = ErrCommand
			}
			notifications := 0
			v, p, e := Pin(context.Background(), local.Dir, values[0].Identity, g, runner, Defaults(), func(string) { notifications++ })
			if tc.race {
				if e == nil {
					v.Close()
					t.Fatal("failed fetch accepted")
				}
				if tc.repeat && !strings.Contains(e.Error(), "changed repeatedly") {
					t.Fatal(e)
				}
				if g.calls != 3 {
					t.Fatal("race retry count", g.calls)
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				if p.MergeBaseSHA != base || notifications != 1 {
					t.Fatal(p, notifications)
				}
				dir := v.dir
				if e = v.Close(); e != nil {
					t.Fatal(e)
				}
				if _, e = os.Stat(dir); !errors.Is(e, fs.ErrNotExist) {
					t.Fatal("temporary view retained")
				}
			}
			if !reflect.DeepEqual(before, local.Snapshot()) {
				t.Fatal("shallow checkout mutated")
			}
			for _, q := range runner.requests {
				if _, e = os.Stat(q.Dir); !errors.Is(e, fs.ErrNotExist) {
					t.Fatal("failed/successful view not cleaned", q.Dir)
				}
			}
		})
	}
}

func TestSafetyAuthenticationAndFetchLimit(t *testing.T) {
	r := testutil.NewRepo(t)
	m := metadata(strings.Repeat("a", 40), strings.Repeat("b", 40))
	for _, want := range []error{ErrAuthentication, ErrLimit} {
		g := &fakeGH{values: []Metadata{m}}
		runner := &fixtureRunner{t: t, remote: r.Dir, fetchErr: want}
		if want == ErrAuthentication {
			g.tokenErr = want
		}
		_, _, e := Pin(context.Background(), r.Dir, m.Identity, g, runner, Defaults(), nil)
		if !errors.Is(e, want) || g.calls != 1 {
			t.Fatal("failure retried or hidden", e, g.calls)
		}
	}
}

type requestFunc func(context.Context, Request) ([]byte, error)

func (f requestFunc) Run(c context.Context, r Request) ([]byte, error) { return f(c, r) }
func TestPinnedGHReadOnlyContract(t *testing.T) {
	m := metadata(strings.Repeat("a", 40), strings.Repeat("b", 40))
	calls := 0
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: requestFunc(func(_ context.Context, r Request) ([]byte, error) {
		calls++
		if !reflect.DeepEqual(r.Args, []string{"api", "--hostname", "github.com", "--method", "GET", "repos/owner/repo/pulls/42"}) {
			t.Fatal("unexpected GitHub operation", r.Args)
		}
		return []byte(`{"number":42,"base":{"sha":"` + m.BaseSHA + `","repo":{"full_name":"owner/repo"}},"head":{"sha":"` + m.HeadSHA + `","repo":{"full_name":"fork/repo"}}}`), nil
	})}
	got, e := g.Metadata(context.Background(), m.Identity)
	if e != nil || got != m || calls != 1 {
		t.Fatal(got, e)
	}
}
