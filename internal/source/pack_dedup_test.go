package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pjbgf/sha1cd"

	"prui/internal/testutil"
)

type packVerifyRunner struct {
	Runner
	calls int
	fail  error
}

func (r *packVerifyRunner) Run(ctx context.Context, request Request) ([]byte, error) {
	if len(request.Args) > 2 && request.Args[2] == "index-pack" {
		r.calls++
		want := []string{"index-pack", "--verify", "--no-rev-index", "--threads=1", "--max-input-size=16777216"}
		if len(request.Args) != len(want)+3 || !reflect.DeepEqual(request.Args[2:len(request.Args)-1], want) {
			return nil, fmt.Errorf("unsafe verify arguments: %v", request.Args)
		}
		if r.fail != nil {
			return nil, r.fail
		}
	}
	return r.Runner.Run(ctx, request)
}

func duplicatedPackFixture(t *testing.T) (*testutil.Repo, string, string) {
	t.Helper()
	repo := testutil.NewRepo(t)
	for i := range 2100 {
		repo.Write(fmt.Sprintf("file%04d", i), fmt.Sprintf("content %d\n", i))
	}
	head := repo.Commit()
	repo.Git("repack", "-a") // Deliberately retain loose copies; only fixture objects change.
	packs, err := filepath.Glob(filepath.Join(repo.Dir, ".git/objects/pack/*.pack"))
	if err != nil || len(packs) != 1 {
		t.Fatal("fixture pack missing", packs, err)
	}
	// A real reverse index must remain byte/mode-identical during verification.
	repo.Git("index-pack", "--rev-index", packs[0])
	// Git accepts noncanonical pack basenames; the real repository uses loose-.
	renamed := filepath.Join(filepath.Dir(packs[0]), "loose-"+strings.TrimPrefix(filepath.Base(packs[0]), "pack-"))
	for _, extension := range []string{".pack", ".idx", ".rev"} {
		from := strings.TrimSuffix(packs[0], ".pack") + extension
		to := strings.TrimSuffix(renamed, ".pack") + extension
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	return repo, head, renamed
}

func TestVerifiedPackOmitsDuplicatesAndRetainsImmutableSource(t *testing.T) {
	repo, head, pack := duplicatedPackFixture(t)
	oid := repo.Git("rev-parse", head+":file0000")
	before := repo.Snapshot()
	runner := &packVerifyRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if runner.calls != 1 {
		t.Fatal("pack was not independently verified", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(v.dir, "borrowed", oid[:2], oid[2:])); !os.IsNotExist(err) {
		t.Fatal("duplicate loose object retained", err)
	}
	copyPath := filepath.Join(v.dir, "borrowed/pack", filepath.Base(pack))
	original, err := os.Stat(pack)
	if err != nil {
		t.Fatal(err)
	}
	copied, err := os.Stat(copyPath)
	if err != nil || os.SameFile(original, copied) {
		t.Fatal("verified input is not a private immutable copy", err)
	}
	if !reflect.DeepEqual(before, repo.Snapshot()) {
		t.Fatal("verification changed the reviewed checkout")
	}
	// Fetch deliberately removes borrowed reachability before checking remote
	// ancestry. Deduplication must not leave local packs visible in objects/.
	altPath := filepath.Join(v.dir, "objects/info/alternates")
	alt, err := os.ReadFile(altPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(altPath); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Git(context.Background(), 100, "cat-file", "-e", head+"^{commit}"); err == nil {
		t.Fatal("borrowed pack bypassed the fetch reachability barrier")
	}
	if size, err := storageSize(filepath.Join(v.dir, "objects")); err != nil || size != 0 {
		t.Fatal("local copies consumed fetch storage allowance", size, err)
	}
	if err := os.WriteFile(altPath, alt, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(repo.Dir, ".git/objects")); err != nil {
		t.Fatal(err)
	}
	data, err := v.Blob(context.Background(), oid, 100)
	if err != nil || !bytes.Equal(data, []byte("content 0\n")) {
		t.Fatal("removing original storage lost verified source", string(data), err)
	}
	if _, err := v.Git(context.Background(), 100, "cat-file", "-e", head+"^{commit}"); err != nil {
		t.Fatal("pinned commit lost", err)
	}
}

func TestPackVerificationFailuresNeverAuthorizeOmission(t *testing.T) {
	repo, head, _ := duplicatedPackFixture(t)
	oid := repo.Git("rev-parse", head+":file0000")
	for _, failure := range []error{ErrCommand, ErrLimit, context.Canceled} {
		t.Run(failure.Error(), func(t *testing.T) {
			runner := &packVerifyRunner{Runner: NewRunner(), fail: failure}
			private := t.TempDir()
			t.Setenv("TMPDIR", private)
			v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
			if runner.calls != 1 {
				t.Fatal("verification retried or skipped", runner.calls)
			}
			if errors.Is(failure, ErrCommand) {
				if err != nil {
					t.Fatal("optional verification failure changed ordinary mirroring", err)
				}
				defer v.Close()
				if _, err := os.Stat(filepath.Join(v.dir, "borrowed", oid[:2], oid[2:])); err != nil {
					t.Fatal("failed verification omitted loose source", err)
				}
				if _, err := v.Blob(context.Background(), oid, 100); err != nil {
					t.Fatal(err)
				}
			} else {
				if v != nil || !errors.Is(err, failure) {
					t.Fatal("resource/cancellation failure retried through ordinary mirroring", v, err)
				}
				entries, err := os.ReadDir(private)
				if err != nil || len(entries) != 0 {
					t.Fatal("failed verification leaked private data", entries, err)
				}
			}
		})
	}
}

func TestPackPreflightMatchesRealRefAndOffsetDeltas(t *testing.T) {
	for _, offset := range []bool{false, true} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			repo := testutil.NewRepo(t)
			for i := range 20 {
				repo.Write("source", strings.Repeat("unchanged source line\n", 1000)+fmt.Sprintf("change %d\n", i))
				repo.Commit()
			}
			base := filepath.Join(t.TempDir(), "fixture")
			args := []string{"pack-objects", "--window=100", "--depth=50"}
			if offset {
				args = append(args, "--delta-base-offset")
			}
			args = append(args, base)
			digest := repo.GitInput(repo.Git("rev-list", "--objects", "--all")+"\n", args...)
			index, err := os.ReadFile(base + "-" + digest + ".idx")
			if err != nil {
				t.Fatal(err)
			}
			pack, err := os.ReadFile(base + "-" + digest + ".pack")
			if err != nil {
				t.Fatal(err)
			}
			plan := preflightPack(context.Background(), index, pack, Defaults())
			if plan == nil {
				t.Fatal("bounded real delta pack rejected")
			}
			deltas := 0
			want := byte(7)
			if offset {
				want = 6
			}
			for _, record := range plan.records {
				if (pack[record.offset]>>4)&7 == want {
					deltas++
				}
			}
			if deltas == 0 {
				t.Fatal("fixture did not exercise delta encoding", want)
			}
		})
	}
}

func TestUnsupportedPackEligibilityRetainsOrdinarySource(t *testing.T) {
	repo, head, pack := duplicatedPackFixture(t)
	oid := repo.Git("rev-parse", head+":file0000")
	index := strings.TrimSuffix(pack, ".pack") + ".idx"
	// Git writes immutable pack indices read-only. This synthetic fixture
	// deliberately corrupts/restores its own index before opening a view.
	if err := os.Chmod(index, 0600); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range []string{"tight blob limit", "corrupt index", "forged index", "duplicate symlink"} {
		t.Run(shape, func(t *testing.T) {
			limits := Defaults()
			sourcePath := filepath.Join(repo.Dir, ".git/objects", oid[:2], oid[2:])
			if shape == "tight blob limit" {
				limits.BlobBytes = 1
			}
			if shape == "corrupt index" || shape == "forged index" {
				bad := bytes.Clone(original)
				if shape == "forged index" {
					bad[8+1024+19] ^= 1 // Self-consistent checksum, wrong object identity.
					sum, _ := sha1cd.Sum(bad[:len(bad)-20])
					copy(bad[len(bad)-20:], sum[:])
					if preflightIndex(bad, limits) == nil {
						t.Fatal("forgery did not reach independent verification")
					}
				} else {
					bad[len(bad)-1] ^= 1
				}
				if err := os.WriteFile(index, bad, 0600); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.WriteFile(index, original, 0600); err != nil {
						t.Fatal(err)
					}
				}()
			}
			if shape == "duplicate symlink" {
				data, err := os.ReadFile(sourcePath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(sourcePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/untrusted/never-read", sourcePath); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := os.Remove(sourcePath); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(sourcePath, data, 0600); err != nil {
						t.Fatal(err)
					}
				}()
			}
			runner := &packVerifyRunner{Runner: NewRunner()}
			v, err := NewView(context.Background(), repo.Dir, runner, limits)
			if shape == "duplicate symlink" {
				if err == nil {
					_ = v.Close()
					t.Fatal("verified duplicate bypassed symlink rejection")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			wantCalls := 0
			if shape == "forged index" {
				wantCalls = 1
			}
			if runner.calls != wantCalls {
				t.Fatal("unsupported pack eligibility launched verification", runner.calls)
			}
			if _, err := os.Stat(filepath.Join(v.dir, "borrowed", oid[:2], oid[2:])); err != nil {
				t.Fatal("unverified source omitted", err)
			}
			if _, err := v.Blob(context.Background(), oid, 100); err != nil {
				t.Fatal(err)
			}
		})
	}
	repo.Git("prune-packed") // Synthetic fixture only: pure packs need no optimization.
	runner := &packVerifyRunner{Runner: NewRunner()}
	v, err := NewView(context.Background(), repo.Dir, runner, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if runner.calls != 0 {
		t.Fatal("purely packed store was needlessly verified")
	}
	if _, err := v.Blob(context.Background(), oid, 100); err != nil {
		t.Fatal(err)
	}
}

func TestPrivatePackCopyCannotOverwriteBorrowedLink(t *testing.T) {
	original := filepath.Join(t.TempDir(), "original")
	linked := filepath.Join(t.TempDir(), "borrowed.pack")
	if err := os.WriteFile(original, []byte("immutable"), 0400); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Fatal(err)
	}
	if err := writeNewPackFile(linked, []byte("changed")); !errors.Is(err, os.ErrExist) {
		t.Fatal("private copy overwrote existing data", err)
	}
	data, err := os.ReadFile(original)
	if err != nil || string(data) != "immutable" {
		t.Fatal("borrowed object changed", string(data), err)
	}
	for _, name := range []string{"../bad.idx", "space name.idx", ".idx", "bad\x1b.idx", strings.Repeat("x", 129) + ".idx"} {
		if safePackIndexName(name) {
			t.Fatal("unsafe pack name accepted", name)
		}
	}
}
