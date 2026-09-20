package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"pr-review/internal/guide"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/tui"
	"pr-review/internal/verify"
)

type application struct {
	store       *session.Store
	gh          source.GitHub
	setupError  error
	runner      source.Runner
	limits      source.Limits
	offline     bool
	newAnalyzer func() (guide.Analyzer, error)
}

// timedGitHub measures only the metadata requests made while opening the
// pinned comparison. It is enabled solely for verifier subprocesses.
type timedGitHub struct {
	source.GitHub
	metadata time.Duration
}

func (g *timedGitHub) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	started := time.Now()
	result, err := g.GitHub.Metadata(ctx, id)
	g.metadata += time.Since(started)
	return result, err
}

func (a *application) Metadata(ctx context.Context, id source.Identity) (source.Metadata, error) {
	if err := a.online(ctx); err != nil {
		return source.Metadata{}, err
	}
	if a.setupError != nil {
		return source.Metadata{}, a.setupError
	}
	if a.gh == nil {
		return source.Metadata{}, errors.New("GitHub CLI unavailable")
	}
	return a.gh.Metadata(ctx, id)
}

// submitReviewComment is the application's narrow write boundary. It checks
// that the complete immutable comparison still matches GitHub immediately
// before a single comment request; it never retargets or retries a write.
func (a *application) submitReviewComment(ctx context.Context, submission tui.CommentSubmission) error {
	if err := a.online(ctx); err != nil {
		return err
	}
	if err := validReviewCommentSubmission(submission); err != nil {
		return err
	}
	if a.setupError != nil {
		return a.setupError
	}
	if a.gh == nil {
		return errors.New("GitHub CLI unavailable")
	}
	commenter, ok := a.gh.(source.ReviewCommenter)
	if !ok {
		return errors.New("GitHub review comment submission unavailable")
	}
	current, err := a.Metadata(ctx, submission.Metadata.Identity)
	if err != nil {
		return err
	}
	if current != submission.Metadata {
		return errors.New("pull request changed; open a new comparison before posting a comment")
	}
	return commenter.CreateReviewComment(ctx, submission.Comment)
}

func validReviewCommentSubmission(submission tui.CommentSubmission) error {
	comment, frozen := submission.Comment, submission.Metadata
	if !validIdentity(frozen.Identity) || !validIdentity(comment.Target.Identity) || frozen.Identity != comment.Target.Identity ||
		!validSHA(frozen.BaseSHA) || !validSHA(frozen.HeadSHA) || !validRepository(frozen.BaseRepository) || !validRepository(frozen.HeadRepository) ||
		comment.Target.CommitID != frozen.HeadSHA || !validSHA(comment.Target.CommitID) || comment.Target.Line <= 0 ||
		(comment.Target.Side != "LEFT" && comment.Target.Side != "RIGHT") || comment.Target.Path == "" || !utf8.ValidString(comment.Target.Path) ||
		comment.Body == "" || !utf8.ValidString(comment.Body) {
		return errors.New("invalid review comment submission")
	}
	return nil
}

func validIdentity(id source.Identity) bool {
	_, err := source.ParseIdentity(strconv.Itoa(id.Number), id.Repository)
	return err == nil
}

func validRepository(repository string) bool {
	_, err := source.ParseIdentity("1", repository)
	return err == nil
}

func validSHA(sha string) bool {
	if len(sha) != 40 {
		return false
	}
	return strings.IndexFunc(sha, func(r rune) bool {
		return r < '0' || r > '9' && (r < 'a' || r > 'f')
	}) == -1
}

func (a *application) listPullRequests(ctx context.Context, repository string) ([]source.PullRequest, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if _, err := source.ParseIdentity("1", repository); err != nil {
		return nil, err
	}
	if a.setupError != nil {
		return nil, a.setupError
	}
	if a.gh == nil {
		return nil, errors.New("gh executable required; install GitHub CLI and authenticate")
	}
	return a.gh.ListPullRequests(ctx, repository)
}

func (a *application) open(ctx context.Context, checkout string, id source.Identity, notify func(string)) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	checkout, err := canonicalPath(checkout)
	if err != nil {
		return nil, err
	}
	if err := outsideCheckout(a.store.Path(), checkout); err != nil {
		return nil, err
	}
	if a.setupError != nil {
		return nil, a.setupError
	}
	if a.gh == nil {
		return nil, errors.New("gh executable required; install GitHub CLI and authenticate")
	}
	gh := a.gh
	var timing *timedGitHub
	if os.Getenv(verify.OpenTimingEnvironment) == "1" {
		timing = &timedGitHub{GitHub: a.gh}
		gh = timing
	}
	var profile review.Timing
	started := time.Now()
	raw, err := review.OpenWithConfig(ctx, checkout, id, gh, a.runner, a.limits, notify, review.Config{Timing: &profile})
	openedIn := time.Since(started)
	if err != nil {
		return nil, err
	}
	if timing != nil {
		pinAndInventory := openedIn - timing.metadata
		if pinAndInventory < 0 {
			pinAndInventory = 0
		}
		if err := verify.WriteOpenTiming(os.Stderr, verify.OpenTiming{GitHubMetadata: timing.metadata, PinAndInventory: pinAndInventory, ViewSetup: profile.Pin.ViewSetup, Fetch: profile.Pin.Fetch, MergeBase: profile.Pin.MergeBase, Inventory: profile.Inventory, Evidence: profile.Evidence}); err != nil {
			return nil, err
		}
	}
	saved, err := a.store.Create(raw.Snapshot)
	if err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Saved session " + saved.ID + "; checking metadata freshness...")
	}
	if err := review.Refresh(ctx, a.store, saved, a); err != nil {
		return nil, err
	}
	if err := a.store.RememberRepository(saved.Inventory.Comparison.Metadata.BaseRepository, checkout); err != nil {
		return nil, err
	}
	return saved, nil
}

func (a *application) checkout(id source.Identity, explicit string) (string, bool, error) {
	if explicit != "" {
		return explicit, false, nil
	}
	checkout, err := a.store.LookupRepository(id.Repository)
	if err != nil {
		if errors.Is(err, session.ErrRepositoryNotFound) {
			return "", true, fmt.Errorf("no remembered checkout for %s; pass --repo <checkout>", id.Repository)
		}
		return "", true, err
	}
	return checkout, true, nil
}

func (a *application) fresh(ctx context.Context, old *review.Session, checkout string, notify func(string)) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if checkout == "" {
		checkout = string(old.Checkout)
	}
	if checkout == "" {
		return nil, errors.New("new comparison requires --repo checkout")
	}
	return a.open(ctx, checkout, old.Inventory.Comparison.Metadata.Identity, notify)
}

func (a *application) generateGuide(ctx context.Context, original *review.Session, analyzer guide.Analyzer) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if original == nil || original.ID == "" {
		return nil, errors.New("guide generation requires a saved review session")
	}
	derived := review.DeriveGuide(ctx, original, analyzer, review.Config{})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if derived.Guides != nil && derived.Guides.Status == guide.Generated {
		if err := a.store.SaveGeneratedGuide(guideCacheKey(original), *derived.Guides, original.Inventory); err != nil {
			return nil, err
		}
	}
	return a.store.Create(derived)
}

func guideCacheKey(s *review.Session) session.GuideCacheKey {
	m := s.Inventory.Comparison.Metadata
	return session.GuideCacheKey{Repository: m.Identity.Repository, Number: m.Identity.Number, BaseSHA: m.BaseSHA, HeadSHA: m.HeadSHA}
}

// cachedPullRequestSnapshot first verifies the PR's current immutable
// comparison through GitHub, then reuses the locally stored frozen source
// material. This avoids Git fetches and inventory/context rebuilding when the
// base and head have not changed, while never trusting a cache entry as a
// freshness signal.
func (a *application) cachedPullRequestSnapshot(ctx context.Context, checkout string, id source.Identity, notify func(string)) (*review.Session, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	hasCached, err := a.store.HasComparisonSnapshot(id)
	if err != nil {
		return nil, err
	}
	if !hasCached {
		return nil, nil
	}
	checkout, err = canonicalPath(checkout)
	if err != nil {
		return nil, err
	}
	if err := outsideCheckout(a.store.Path(), checkout); err != nil {
		return nil, err
	}
	if a.setupError != nil {
		return nil, a.setupError
	}
	if a.gh == nil {
		return nil, errors.New("gh executable required; install GitHub CLI and authenticate")
	}
	if notify != nil {
		notify("Checking current PR revision and local source cache...")
	}
	m, err := a.Metadata(ctx, id)
	if err != nil {
		return nil, err
	}
	snapshot, err := a.store.LoadComparisonSnapshot(m)
	if err != nil || snapshot == nil {
		return nil, err
	}
	// The checkout is only a future-refresh hint; frozen source material stays
	// byte-identical to the cache entry.
	snapshot.Checkout = []byte(checkout)
	saved, err := a.store.Create(*snapshot)
	if err != nil {
		return nil, err
	}
	saved.RevisionStatus = session.Current
	if err := a.store.Save(saved); err != nil {
		return nil, err
	}
	if err := a.store.RememberRepository(m.BaseRepository, checkout); err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Reused local frozen source for the unchanged PR comparison.")
	}
	return saved, nil
}

// openFromPullRequestList is the only automatic guide path. It still pins a
// current PR revision before using a local frozen-source cache, so a base/head
// change cannot reuse source material or interpretation for different units.
func (a *application) openFromPullRequestList(ctx context.Context, checkout string, id source.Identity, notify func(string)) (*review.Session, error) {
	raw, err := a.cachedPullRequestSnapshot(ctx, checkout, id, notify)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		raw, err = a.open(ctx, checkout, id, notify)
	}
	if err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Checking local guide cache for this pinned comparison...")
	}
	if cached, err := a.store.LoadGeneratedGuide(guideCacheKey(raw), raw.Inventory); err != nil {
		return nil, err
	} else if cached != nil {
		if notify != nil {
			notify("Reusing local guide for this pinned comparison...")
		}
		derived := raw.Snapshot
		derived.Guides = cached
		derived.DerivedFrom = raw.ID
		return a.store.Create(derived)
	}
	if notify != nil {
		notify("Generating OpenAI guide from bounded pinned source and evidence...")
	}
	analyzer, err := a.createGuideAnalyzer()
	if err != nil {
		return raw, nil
	}
	return a.generateGuide(ctx, raw, analyzer)
}

func (a *application) load(ctx context.Context, o options, notify func(string)) (*review.Session, error) {
	if o.Command == "open" {
		checkout, cached, err := a.checkout(o.Identity, o.Checkout)
		if err != nil {
			return nil, err
		}
		saved, err := a.open(ctx, checkout, o.Identity, notify)
		if err != nil && cached {
			return nil, fmt.Errorf("remembered checkout for %s is unavailable or does not match; pass --repo <checkout>: %w", o.Identity.Repository, err)
		}
		return saved, err
	}
	var reader review.MetadataReader = a
	if o.Offline || a.offline {
		reader = nil
	}
	saved, err := review.Resume(ctx, a.store, o.SessionID, reader)
	if err != nil {
		return nil, err
	}
	if o.New {
		return a.fresh(ctx, saved, o.Checkout, notify)
	}
	return saved, nil
}

func listSessions(store *session.Store, out io.Writer) int {
	entries, err := store.List()
	if err != nil {
		_, _ = fmt.Fprintln(out, tui.Escape(err.Error()))
		return 1
	}
	_, _ = fmt.Fprintln(out, "Stored freshness checks are historical; resume checks again. Reading is not GitHub approval.")
	code := 0
	for _, entry := range entries {
		if entry.Err != nil {
			_, _ = fmt.Fprintln(out, entry.ID, "UNREADABLE (retained):", tui.Escape(entry.Err.Error()))
			code = 1
			continue
		}
		s := entry.Record
		_, _ = fmt.Fprintf(out, "%s %s #%d head %.12s %d/%d read | last check: %s | updated %s\n", entry.ID, tui.Escape(s.Inventory.Comparison.Metadata.Identity.Repository), s.Inventory.Comparison.Metadata.Identity.Number, s.Inventory.Comparison.Metadata.HeadSHA, len(s.ReviewedSliceIDs), len(s.Slices), s.RevisionStatus, s.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	}
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(out, "No saved sessions.")
	}
	return code
}

func listPullRequests(out io.Writer, repository string, prs []source.PullRequest) {
	for _, pr := range prs {
		_, _ = fmt.Fprintf(out, "%s #%d %s\n", tui.Escape(repository), pr.Identity.Number, tui.Escape(pr.Title))
	}
	if len(prs) == 0 {
		_, _ = fmt.Fprintln(out, "No open pull requests.")
	}
}
