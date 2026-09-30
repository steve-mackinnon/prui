package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"prui/internal/commits"
	"prui/internal/guide"
	"prui/internal/guideconfig"
	"prui/internal/review"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/tui"
	"prui/internal/verify"
)

type application struct {
	store                  *session.Store
	gh                     source.GitHub
	setupError             error
	runner                 source.Runner
	limits                 source.Limits
	offline                bool
	newAnalyzer            func() (guide.Analyzer, error)
	guideClient            *http.Client
	guideSelection         guideconfig.Selection
	guideConfigSelection   guideconfig.Selection
	guideConfigPath        string
	guideSelectionError    error
	guideSelectionResolved bool
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
func (a *application) submitReviewComment(ctx context.Context, submission tui.CommentSubmission) (source.ReviewComment, error) {
	if err := a.online(ctx); err != nil {
		return source.ReviewComment{}, err
	}
	if err := validReviewCommentSubmission(submission); err != nil {
		return source.ReviewComment{}, err
	}
	if a.setupError != nil {
		return source.ReviewComment{}, a.setupError
	}
	if a.gh == nil {
		return source.ReviewComment{}, errors.New("GitHub CLI unavailable")
	}
	commenter, ok := a.gh.(source.ReviewCommenter)
	if !ok {
		return source.ReviewComment{}, errors.New("GitHub review comment submission unavailable")
	}
	current, err := a.Metadata(ctx, submission.Metadata.Identity)
	if err != nil {
		return source.ReviewComment{}, err
	}
	if !source.SamePinnedRevision(current, submission.Metadata) {
		return source.ReviewComment{}, errors.New("pull request changed; open a new comparison before posting a comment")
	}
	created, err := commenter.CreateReviewComment(ctx, submission.Comment)
	if err != nil {
		return source.ReviewComment{}, errors.Join(source.ErrCommentDeliveryUnknown, err)
	}
	return created, nil
}

func (a *application) submitPullRequestReview(ctx context.Context, submission tui.ReviewSubmission) error {
	if err := a.online(ctx); err != nil {
		return err
	}
	review, frozen := submission.Review, submission.Metadata
	if !validIdentity(frozen.Identity) || review.Identity != frozen.Identity || review.CommitID != frozen.HeadSHA || !validSHA(frozen.BaseSHA) || !validSHA(frozen.HeadSHA) || !validRepository(frozen.BaseRepository) || !validRepository(frozen.HeadRepository) {
		return errors.New("invalid pull request review submission")
	}
	if err := source.ValidatePullRequestReview(review); err != nil {
		return err
	}
	if a.setupError != nil {
		return a.setupError
	}
	writer, ok := a.gh.(source.PullRequestReviewWriter)
	if !ok {
		return errors.New("GitHub review submission unavailable")
	}
	current, err := a.Metadata(ctx, frozen.Identity)
	if err != nil {
		return err
	}
	if !source.SamePinnedRevision(current, frozen) {
		return errors.New("pull request changed; open a new comparison before submitting a review")
	}
	return writer.CreatePullRequestReview(ctx, review)
}

func (a *application) listReviewComments(ctx context.Context, metadata source.Metadata) ([]source.ReviewComment, error) {
	if err := a.online(ctx); err != nil {
		return nil, err
	}
	if a.setupError != nil {
		return nil, a.setupError
	}
	reader, ok := a.gh.(source.ReviewCommentReader)
	if !ok {
		return nil, errors.New("GitHub review comment refresh unavailable")
	}
	return reader.ListReviewComments(ctx, metadata.Identity)
}

func (a *application) viewer(ctx context.Context) (source.Viewer, error) {
	if err := a.online(ctx); err != nil {
		return source.Viewer{}, err
	}
	viewer, ok := a.gh.(source.ReviewCommentViewer)
	if !ok {
		return source.Viewer{}, errors.New("GitHub viewer unavailable")
	}
	return viewer.Viewer(ctx)
}

// submitReviewCommentAction keeps reply, delete, and reaction writes behind
// the same online/freshness boundary as initial comments. The TUI has already
// made the action explicit; this layer validates the frozen loaded comment.
func (a *application) submitReviewCommentAction(ctx context.Context, action tui.CommentAction) (source.ReviewComment, source.ReviewCommentReaction, error) {
	if err := a.online(ctx); err != nil {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, err
	}
	if err := validReviewCommentAction(action); err != nil {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, err
	}
	if a.setupError != nil {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, a.setupError
	}
	if a.gh == nil {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("GitHub CLI unavailable")
	}
	current, err := a.Metadata(ctx, action.Metadata.Identity)
	if err != nil {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, err
	}
	if !source.SamePinnedRevision(current, action.Metadata) {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("pull request changed; open a new comparison before changing a comment")
	}
	if action.Delete {
		viewer, ok := a.gh.(source.ReviewCommentViewer)
		if !ok {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("GitHub viewer unavailable")
		}
		identity, err := viewer.Viewer(ctx)
		if err != nil {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, err
		}
		if identity.Login != action.Comment.Author {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("only the comment author may delete it")
		}
		deleter, ok := a.gh.(source.ReviewCommentDeleter)
		if !ok {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("review comment deletion unavailable")
		}
		return source.ReviewComment{}, source.ReviewCommentReaction{}, deleter.DeleteReviewComment(ctx, action.Metadata.Identity, action.Comment.ID)
	}
	if action.Reaction != "" {
		reacter, ok := a.gh.(source.ReviewCommentReactioner)
		if !ok {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("review comment reactions unavailable")
		}
		reaction, err := reacter.AddReviewCommentReaction(ctx, action.Metadata.Identity, action.Comment.ID, action.Reaction)
		return source.ReviewComment{}, reaction, err
	}
	replier, ok := a.gh.(source.ReviewCommentReplier)
	if !ok {
		return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("review comment replies unavailable")
	}
	reply, err := replier.ReplyToReviewComment(ctx, action.Metadata.Identity, action.Comment.ID, action.Body)
	if err == nil {
		expected := action.Comment.Target
		if action.Comment.CurrentAnchor != nil {
			expected = *action.Comment.CurrentAnchor
			if reply.ParentID != action.Comment.ID {
				return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("invalid review comment reply parent")
			}
		}
		if reply.Target != expected {
			return source.ReviewComment{}, source.ReviewCommentReaction{}, errors.New("invalid review comment reply target")
		}
		if action.Comment.CurrentAnchor != nil {
			associated := reply.Target
			reply.CurrentAnchor = &associated
			reply.Target = action.Comment.Target
		}
	}
	return reply, source.ReviewCommentReaction{}, err
}

func validReviewCommentAction(action tui.CommentAction) error {
	if !validIdentity(action.Metadata.Identity) || action.Comment.ID <= 0 || action.Comment.Target.Identity != action.Metadata.Identity || action.Comment.Target.CommitID != action.Metadata.HeadSHA || !validSHA(action.Metadata.HeadSHA) {
		return errors.New("invalid review comment action")
	}
	if action.Comment.CurrentAnchor != nil {
		raw := *action.Comment.CurrentAnchor
		if !validSHA(raw.CommitID) {
			return errors.New("invalid associated comment commit")
		}
		raw.CommitID = action.Metadata.HeadSHA
		if raw != action.Comment.Target {
			return errors.New("invalid associated comment anchor")
		}
	}
	count := 0
	if action.Delete {
		count++
	}
	if action.Reaction != "" {
		count++
	}
	if action.Body != "" {
		count++
	}
	if count != 1 || (action.Body != "" && !utf8.ValidString(action.Body)) || (action.Reaction != "" && !source.IsReviewCommentReaction(action.Reaction)) {
		return errors.New("invalid review comment action")
	}
	return nil
}

func validReviewCommentSubmission(submission tui.CommentSubmission) error {
	comment, frozen := submission.Comment, submission.Metadata
	if !validIdentity(frozen.Identity) || !validIdentity(comment.Target.Identity) || frozen.Identity != comment.Target.Identity ||
		!validSHA(frozen.BaseSHA) || !validSHA(frozen.HeadSHA) || !validRepository(frozen.BaseRepository) || !validRepository(frozen.HeadRepository) ||
		(submission.CommitSHA == "" && comment.Target.CommitID != frozen.HeadSHA) || !validSHA(comment.Target.CommitID) || comment.Target.Line <= 0 ||
		(comment.Target.Side != "LEFT" && comment.Target.Side != "RIGHT") || comment.Target.Path == "" || !utf8.ValidString(comment.Target.Path) ||
		comment.Body == "" || !utf8.ValidString(comment.Body) {
		return errors.New("invalid review comment submission")
	}

	if submission.CommitSHA != "" {
		bundle, inv := submission.CommitBundle, submission.CommitInventory
		target := comment.Target
		if bundle == nil || inv == nil || bundle.BaseSHA != frozen.BaseSHA || bundle.HeadSHA != frozen.HeadSHA ||
			submission.CommitSHA != target.CommitID || !source.SamePinnedRevision(inv.Comparison.Metadata, frozen) ||
			!commits.ContainsTarget(bundle, target) {
			return errors.New("invalid captured commit comment target")
		}
		// The only established coordinate case is a captured head target that also
		// appears in the frozen PR diff. Other first-parent cases require API evidence.
		if target.CommitID != frozen.HeadSHA || !commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, target) {
			return errors.New("historical commit commenting is unavailable until GitHub targeting is verified")
		}
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
		selection := a.activeGuideSelection()
		derived.Guides.SelectionFingerprint = selection.Fingerprint(guide.PromptVersion, guide.SchemaName)
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

// cachedPullRequestSnapshot opens locally frozen source immediately. Its
// caller must check freshness asynchronously before reporting it as current.
func (a *application) cachedPullRequestSnapshot(ctx context.Context, checkout string, id source.Identity, notify func(string)) (*review.Session, error) {
	latest, err := a.store.LatestComparison(id)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, nil
	}
	checkout, err = canonicalPath(checkout)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(checkout); err != nil {
		return nil, err
	}
	if err := outsideCheckout(a.store.Path(), checkout); err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Opening local frozen source; checking the current PR revision in the background...")
	}
	snapshot := latest.Snapshot
	snapshot.Guides = nil
	snapshot.DerivedFrom = ""
	// The checkout is only a future-refresh hint; frozen source material stays
	// byte-identical to the cache entry.
	snapshot.Checkout = []byte(checkout)
	saved, err := a.store.Create(snapshot)
	if err != nil {
		return nil, err
	}
	state, err := a.store.UpdateState(ctx, saved.ID, saved.Generation, saved.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: append([]string(nil), latest.ReviewedSliceIDs...), RevisionStatus: session.Unchecked})
	if err != nil {
		return nil, err
	}
	saved.State = state
	if err := a.store.RememberRepository(saved.Inventory.Comparison.Metadata.BaseRepository, checkout); err != nil {
		return nil, err
	}
	if notify != nil {
		notify("Opened local frozen source; checking the current PR revision in the background...")
	}
	return saved, nil
}

// refreshOpenedPullRequest checks an immediately opened frozen session. A
// changed immutable comparison is rebuilt; an unchanged one simply becomes
// current without refetching Git objects.
func (a *application) refreshOpenedPullRequest(ctx context.Context, opened tui.PullRequestRefreshRequest, notify func(string)) (tui.PullRequestFreshness, error) {
	if notify != nil {
		notify("Checking current PR revision in the background...")
	}
	metadata, err := a.Metadata(ctx, opened.Metadata.Identity)
	if ctx.Err() != nil {
		return tui.PullRequestFreshness{}, ctx.Err()
	}
	if err != nil {
		if notify != nil {
			notify("Could not check PR freshness; showing the local frozen comparison.")
		}
		return tui.PullRequestFreshness{Status: session.CheckFailed}, nil
	}
	if source.SamePinnedRevision(metadata, opened.Metadata) {
		if notify != nil {
			notify("Local frozen comparison is current.")
		}
		return tui.PullRequestFreshness{Status: session.Current}, nil
	}
	if notify != nil {
		notify("PR changed; fetching the new pinned comparison...")
	}
	fresh, err := a.open(ctx, opened.Checkout, opened.Metadata.Identity, notify)
	return tui.PullRequestFreshness{Session: fresh}, err
}

// openFromPullRequestList reuses a saved guide when present. Generation is
// requested separately through the review's explicit consent action.
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
	if cached, err := a.store.LoadGeneratedGuide(guideCacheKey(raw), raw.Inventory); err != nil {
		return nil, err
	} else if cached != nil && a.guideCacheEligible(*cached) {
		derived := raw.Snapshot
		derived.Guides = cached
		derived.DerivedFrom = raw.ID
		guided, err := a.store.Create(derived)
		if err != nil {
			return nil, err
		}
		state, err := a.store.UpdateState(ctx, guided.ID, guided.Generation, guided.SnapshotReference, session.StateUpdate{ReviewedSliceIDs: append([]string(nil), raw.ReviewedSliceIDs...), RevisionStatus: raw.RevisionStatus})
		if err != nil {
			return nil, err
		}
		guided.State = state
		return guided, nil
	}
	return raw, nil
}

func (a *application) load(ctx context.Context, o options, notify func(string)) (*review.Session, error) {
	if o.Command == "open" {
		checkout, cached, err := a.checkout(o.Identity, o.Checkout)
		if err != nil {
			return nil, err
		}
		var saved *review.Session
		if !o.Plain {
			saved, err = a.cachedPullRequestSnapshot(ctx, checkout, o.Identity, notify)
		}
		if err == nil && saved == nil {
			saved, err = a.open(ctx, checkout, o.Identity, notify)
		}
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
		_, _ = fmt.Fprintf(out, "%s %s #%d head %.12s %d/%d read | last check: %s | updated %s\n", entry.ID, tui.Escape(entry.Repository), entry.Number, entry.HeadSHA, entry.ReviewedCount, entry.SliceCount, entry.RevisionStatus, entry.UpdatedAt.Format("2006-01-02T15:04:05Z"))
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
