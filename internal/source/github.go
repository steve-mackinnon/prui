package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

const descriptionMaxBytes = 1 << 20

type Identity struct {
	Repository string
	Number     int
}

func (i Identity) URL() string {
	return fmt.Sprintf("https://github.com/%s/pull/%d", i.Repository, i.Number)
}
func ParseIdentity(input, repository string) (Identity, error) {
	number := input
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
			return Identity{}, errors.New("expected github.com HTTPS PR URL")
		}
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if len(parts) != 4 || parts[2] != "pull" {
			return Identity{}, errors.New("expected PR URL")
		}
		derived := parts[0] + "/" + parts[1]
		if repository != "" && repository != derived {
			return Identity{}, errors.New("conflicting repository identities")
		}
		repository = derived
		number = parts[3]
	}
	n, err := strconv.Atoi(number)
	if err != nil || n <= 0 || strconv.Itoa(n) != number || !repositoryPattern.MatchString(repository) {
		return Identity{}, errors.New("expected PR number and explicit owner/repo")
	}
	return Identity{repository, n}, nil
}

type Metadata struct {
	Identity                                         Identity
	BaseRepository, HeadRepository, BaseSHA, HeadSHA string
	Description                                      string
}

type PullRequest struct {
	Identity     Identity
	Title        string
	Author       string
	LastModifier string // Author of the latest commit in the PR.
	OpenedAt     time.Time
	Checks       CheckStatus
}

type CheckStatus string

const (
	ChecksUnknown CheckStatus = "unknown"
	ChecksPending CheckStatus = "pending"
	ChecksPassed  CheckStatus = "passed"
	ChecksFailed  CheckStatus = "failed"
)

type GitHub interface {
	Metadata(context.Context, Identity) (Metadata, error)
	ListPullRequests(context.Context, string) ([]PullRequest, error)
	Token(context.Context) (string, error)
}

// ReviewComment describes one explicitly requested, line-anchored pull request
// review comment. It is intentionally separate from GitHub, whose operations
// are otherwise read-only.
type ReviewComment struct {
	ID       int64
	ParentID int64
	Author   string
	Target   ReviewCommentTarget
	Body     string
}

// ReviewCommentTarget identifies the frozen pull request diff line to comment
// on. Side is either LEFT or RIGHT, following GitHub's review-comment API.
type ReviewCommentTarget struct {
	Identity Identity
	CommitID string
	Path     string
	Side     string
	Line     int
}

type ReviewCommenter interface {
	CreateReviewComment(context.Context, ReviewComment) (ReviewComment, error)
}

// PullRequestReview is one explicit review submission. Comments are held by the
// caller until this request; creating it submits the review in a single write.
type PullRequestReview struct {
	Identity Identity
	CommitID string
	Event    string // COMMENT, APPROVE, or REQUEST_CHANGES
	Body     string
	Comments []ReviewComment
}

type PullRequestReviewWriter interface {
	CreatePullRequestReview(context.Context, PullRequestReview) error
}

const MaxPendingReviewComments = 100

// ReviewCommentReader is the narrow, read-only overlay boundary. Results are
// deliberately ephemeral: callers must not put them in review sessions.
type ReviewCommentReader interface {
	ListReviewComments(context.Context, Identity) ([]ReviewComment, error)
}

// Viewer is the bounded authenticated identity needed to decide whether a
// destructive action can be offered locally. It is deliberately not a general
// user profile contract.
type Viewer struct{ Login string }

type ReviewCommentViewer interface {
	Viewer(context.Context) (Viewer, error)
}
type ReviewCommentReplier interface {
	ReplyToReviewComment(context.Context, Identity, int64, string) (ReviewComment, error)
}
type ReviewCommentDeleter interface {
	DeleteReviewComment(context.Context, Identity, int64) error
}
type ReviewCommentReactioner interface {
	AddReviewCommentReaction(context.Context, Identity, int64, string) (ReviewCommentReaction, error)
}

// ReviewCommentReaction is a canonical, bounded response from GitHub. It is
// overlay-only and must never be stored in a session or emitted in plain mode.
type ReviewCommentReaction struct {
	ID      int64
	Content string
	Author  string
}

var reviewCommentReactions = map[string]bool{"+1": true, "-1": true, "laugh": true, "confused": true, "heart": true, "hooray": true, "rocket": true, "eyes": true}

func IsReviewCommentReaction(content string) bool { return reviewCommentReactions[content] }

// ReviewCommentReactions returns the documented picker order. The caller gets
// a copy so transient UI code cannot mutate the source contract.
func ReviewCommentReactions() []string {
	return []string{"+1", "-1", "laugh", "confused", "heart", "hooray", "rocket", "eyes"}
}

type GH struct {
	Runner     Runner
	Executable string
	Limits     Limits
	Dir        string
}

func NewGH(r Runner, l Limits, dir string) (*GH, error) {
	p, e := trustedExecutable("gh")
	return &GH{r, p, l, dir}, e
}
func (g *GH) call(ctx context.Context, args ...string) ([]byte, error) {
	return g.callWithStdin(ctx, nil, args...)
}
func (g *GH) callWithStdin(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	return g.Runner.Run(ctx, Request{Program: g.Executable, Args: args, Stdin: stdin, Env: userEnvironment(), Dir: g.Dir, Limit: 1 << 20, Timeout: g.Limits.Operation})
}

func validateReviewComment(comment ReviewComment) error {
	target := comment.Target
	if _, err := ParseIdentity(strconv.Itoa(target.Identity.Number), target.Identity.Repository); err != nil {
		return errors.New("invalid review comment target")
	}
	if !shaPattern.MatchString(target.CommitID) || target.Line <= 0 || (target.Side != "LEFT" && target.Side != "RIGHT") || target.Path == "" || !utf8.ValidString(target.Path) || comment.Body == "" || !utf8.ValidString(comment.Body) {
		return errors.New("invalid review comment")
	}
	return nil
}

func (g *GH) CreateReviewComment(ctx context.Context, comment ReviewComment) (ReviewComment, error) {
	if err := validateReviewComment(comment); err != nil {
		return ReviewComment{}, err
	}
	payload, err := json.Marshal(struct {
		Body     string `json:"body"`
		CommitID string `json:"commit_id"`
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Side     string `json:"side"`
	}{
		Body: comment.Body, CommitID: comment.Target.CommitID, Path: comment.Target.Path,
		Line: comment.Target.Line, Side: comment.Target.Side,
	})
	if err != nil {
		return ReviewComment{}, errors.New("could not prepare review comment")
	}
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/comments", comment.Target.Identity.Repository, comment.Target.Identity.Number))
	if err != nil {
		return ReviewComment{}, safeReviewCommentError(err)
	}
	created, err := parseReviewComment(data, comment.Target.Identity)
	if err != nil || created.Target != comment.Target || created.Body != comment.Body {
		return ReviewComment{}, errors.New("invalid created review comment")
	}
	return created, nil
}

func ValidatePullRequestReview(review PullRequestReview) error {
	if _, err := ParseIdentity(strconv.Itoa(review.Identity.Number), review.Identity.Repository); err != nil {
		return errors.New("invalid pull request review")
	}
	if !shaPattern.MatchString(review.CommitID) || !utf8.ValidString(review.Body) || len(review.Comments) > MaxPendingReviewComments {
		return errors.New("invalid pull request review")
	}
	switch review.Event {
	case "COMMENT", "REQUEST_CHANGES":
		if strings.TrimSpace(review.Body) == "" {
			return errors.New("review summary is required")
		}
	case "APPROVE":
	default:
		return errors.New("invalid pull request review event")
	}
	for _, comment := range review.Comments {
		if validateReviewComment(comment) != nil || comment.Target.Identity != review.Identity || comment.Target.CommitID != review.CommitID {
			return errors.New("invalid pending review comment")
		}
	}
	return nil
}

func (g *GH) CreatePullRequestReview(ctx context.Context, review PullRequestReview) error {
	if err := ValidatePullRequestReview(review); err != nil {
		return err
	}
	type inline struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Side string `json:"side"`
		Body string `json:"body"`
	}
	comments := make([]inline, 0, len(review.Comments))
	for _, c := range review.Comments {
		comments = append(comments, inline{Path: c.Target.Path, Line: c.Target.Line, Side: c.Target.Side, Body: c.Body})
	}
	payload, err := json.Marshal(struct {
		CommitID string   `json:"commit_id"`
		Event    string   `json:"event"`
		Body     string   `json:"body"`
		Comments []inline `json:"comments"`
	}{review.CommitID, review.Event, review.Body, comments})
	if err != nil {
		return errors.New("could not prepare pull request review")
	}
	_, err = g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/reviews", review.Identity.Repository, review.Identity.Number))
	if err != nil {
		return safeReviewCommentError(err)
	}
	return nil
}

func validCommentAction(id Identity, commentID int64) bool {
	_, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository)
	return err == nil && commentID > 0
}

func (g *GH) Viewer(ctx context.Context) (Viewer, error) {
	data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", "user")
	if err != nil {
		return Viewer{}, safeReviewCommentError(err)
	}
	var raw struct {
		Login string `json:"login"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.Login == "" || !utf8.ValidString(raw.Login) || len(raw.Login) > 256 {
		return Viewer{}, errors.New("invalid GitHub viewer")
	}
	return Viewer{Login: raw.Login}, nil
}

func (g *GH) ReplyToReviewComment(ctx context.Context, id Identity, commentID int64, body string) (ReviewComment, error) {
	if !validCommentAction(id, commentID) || body == "" || !utf8.ValidString(body) {
		return ReviewComment{}, errors.New("invalid review comment reply")
	}
	payload, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return ReviewComment{}, errors.New("could not prepare review comment reply")
	}
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/comments/%d/replies", id.Repository, id.Number, commentID))
	if err != nil {
		return ReviewComment{}, safeReviewCommentError(err)
	}
	comment, err := parseReviewComment(data, id)
	if err != nil || comment.Body != body || comment.ParentID != commentID {
		return ReviewComment{}, errors.New("invalid created review comment reply")
	}
	return comment, nil
}

func (g *GH) DeleteReviewComment(ctx context.Context, id Identity, commentID int64) error {
	if !validCommentAction(id, commentID) {
		return errors.New("invalid review comment deletion")
	}
	if _, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "DELETE", fmt.Sprintf("repos/%s/pulls/comments/%d", id.Repository, commentID)); err != nil {
		return safeReviewCommentError(err)
	}
	return nil
}

func (g *GH) AddReviewCommentReaction(ctx context.Context, id Identity, commentID int64, content string) (ReviewCommentReaction, error) {
	if !validCommentAction(id, commentID) || !reviewCommentReactions[content] {
		return ReviewCommentReaction{}, errors.New("invalid review comment reaction")
	}
	payload, err := json.Marshal(struct {
		Content string `json:"content"`
	}{content})
	if err != nil {
		return ReviewCommentReaction{}, errors.New("could not prepare review comment reaction")
	}
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/comments/%d/reactions", id.Repository, commentID))
	if err != nil {
		return ReviewCommentReaction{}, safeReviewCommentError(err)
	}
	var raw struct {
		ID      int64  `json:"id"`
		Content string `json:"content"`
		User    *struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.ID <= 0 || raw.User == nil || raw.User.Login == "" || !utf8.ValidString(raw.User.Login) || !reviewCommentReactions[raw.Content] {
		return ReviewCommentReaction{}, errors.New("invalid review comment reaction")
	}
	return ReviewCommentReaction{ID: raw.ID, Content: raw.Content, Author: raw.User.Login}, nil
}

const maxReviewComments = 100

// ListReviewComments reads exactly one explicitly bounded page. The API may
// have more history, but an overlay never needs unbounded remote data.
func (g *GH) ListReviewComments(ctx context.Context, id Identity) ([]ReviewComment, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return nil, err
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("repos/%s/pulls/%d/comments?per_page=%d&page=1", id.Repository, id.Number, maxReviewComments))
	if err != nil {
		return nil, safeReviewCommentError(err)
	}
	var raw []json.RawMessage
	if json.Unmarshal(data, &raw) != nil || len(raw) > maxReviewComments {
		return nil, errors.New("invalid review comment list")
	}
	comments := make([]ReviewComment, 0, len(raw))
	seen := map[int64]bool{}
	for _, item := range raw {
		comment, err := parseReviewComment(item, id)
		if err != nil || seen[comment.ID] {
			return nil, errors.New("invalid review comment list")
		}
		seen[comment.ID] = true
		comments = append(comments, comment)
	}
	return comments, nil
}

func parseReviewComment(data []byte, identity Identity) (ReviewComment, error) {
	var raw struct {
		ID       int64  `json:"id"`
		Body     string `json:"body"`
		CommitID string `json:"commit_id"`
		Path     string `json:"path"`
		Side     string `json:"side"`
		Line     int    `json:"line"`
		ParentID int64  `json:"in_reply_to_id"`
		User     *struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.ID <= 0 || raw.User == nil || raw.User.Login == "" || !utf8.ValidString(raw.User.Login) {
		return ReviewComment{}, errors.New("invalid review comment")
	}
	if raw.ParentID < 0 || raw.ParentID == raw.ID {
		return ReviewComment{}, errors.New("invalid review comment")
	}
	comment := ReviewComment{ID: raw.ID, ParentID: raw.ParentID, Author: raw.User.Login, Body: raw.Body, Target: ReviewCommentTarget{Identity: identity, CommitID: raw.CommitID, Path: raw.Path, Side: raw.Side, Line: raw.Line}}
	if err := validateReviewComment(comment); err != nil {
		return ReviewComment{}, err
	}
	return comment, nil
}

func safeReviewCommentError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, ErrLimit):
		return fmt.Errorf("GitHub review comment unavailable: %w", ErrLimit)
	default:
		return fmt.Errorf("GitHub review comment unavailable (check authentication and connectivity): %w", ErrCommand)
	}
}
func (g *GH) Metadata(ctx context.Context, id Identity) (Metadata, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return Metadata{}, err
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("repos/%s/pulls/%d", id.Repository, id.Number))
	if err != nil {
		return Metadata{}, fmt.Errorf("GitHub metadata unavailable (check authentication and connectivity): %w", err)
	}
	type side struct {
		SHA  string `json:"sha"`
		Repo *struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	}
	var raw struct {
		Number     int     `json:"number"`
		Body       *string `json:"body"`
		Base, Head side
	}
	if !utf8.Valid(data) || json.Unmarshal(data, &raw) != nil || raw.Number != id.Number || raw.Base.Repo == nil || raw.Head.Repo == nil {
		return Metadata{}, errors.New("invalid metadata or deleted fork")
	}
	description := ""
	if raw.Body != nil {
		description = *raw.Body
	}
	if !utf8.ValidString(description) || len(description) > descriptionMaxBytes {
		return Metadata{}, errors.New("invalid pull request description")
	}
	m := Metadata{Identity: id, BaseRepository: raw.Base.Repo.FullName, HeadRepository: raw.Head.Repo.FullName, BaseSHA: raw.Base.SHA, HeadSHA: raw.Head.SHA, Description: description}
	if !repositoryPattern.MatchString(m.BaseRepository) || !repositoryPattern.MatchString(m.HeadRepository) || !strings.EqualFold(m.BaseRepository, id.Repository) || !shaPattern.MatchString(m.BaseSHA) || !shaPattern.MatchString(m.HeadSHA) {
		return Metadata{}, errors.New("unsupported repository or revision metadata")
	}
	return m, nil
}

func (g *GH) ListPullRequests(ctx context.Context, repository string) ([]PullRequest, error) {
	if _, err := ParseIdentity("1", repository); err != nil {
		return nil, err
	}
	parts := strings.Split(repository, "/")
	const query = `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pullRequests(first:100,states:OPEN,orderBy:{field:CREATED_AT,direction:DESC}){nodes{number title createdAt author{login} commits(last:1){nodes{commit{author{name user{login}} statusCheckRollup{state}}}}}}}}`
	data, err := g.call(ctx, "api", "--hostname", "github.com", "graphql", "-f", "query="+query, "-F", "owner="+parts[0], "-F", "name="+parts[1])
	if err != nil {
		return nil, fmt.Errorf("GitHub pull request list unavailable (check authentication and connectivity): %w", err)
	}
	var raw struct {
		Data struct {
			Repository *struct {
				PullRequests struct {
					Nodes []struct {
						Number    int       `json:"number"`
						Title     string    `json:"title"`
						CreatedAt time.Time `json:"createdAt"`
						Author    *struct {
							Login string `json:"login"`
						} `json:"author"`
						Commits struct {
							Nodes []struct {
								Commit struct {
									Author *struct {
										Name string `json:"name"`
										User *struct {
											Login string `json:"login"`
										} `json:"user"`
									} `json:"author"`
									StatusCheckRollup *struct {
										State string `json:"state"`
									} `json:"statusCheckRollup"`
								} `json:"commit"`
							} `json:"nodes"`
						} `json:"commits"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			} `json:"repository"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Data.Repository == nil || len(raw.Errors) > 0 || len(raw.Data.Repository.PullRequests.Nodes) > 100 {
		return nil, errors.New("invalid pull request list")
	}
	prs := make([]PullRequest, 0, len(raw.Data.Repository.PullRequests.Nodes))
	seen := map[int]bool{}
	for _, item := range raw.Data.Repository.PullRequests.Nodes {
		if item.Number <= 0 || seen[item.Number] || item.CreatedAt.IsZero() || !validListText(item.Title) || len(item.Commits.Nodes) > 1 {
			return nil, errors.New("invalid pull request list")
		}
		pr := PullRequest{Identity: Identity{Repository: repository, Number: item.Number}, Title: item.Title, OpenedAt: item.CreatedAt, Checks: ChecksUnknown}
		if item.Author != nil {
			pr.Author = item.Author.Login
		}
		if len(item.Commits.Nodes) == 1 {
			commit := item.Commits.Nodes[0].Commit
			if commit.Author != nil {
				pr.LastModifier = commit.Author.Name
				if commit.Author.User != nil {
					pr.LastModifier = commit.Author.User.Login
				}
			}
			if commit.StatusCheckRollup != nil {
				switch commit.StatusCheckRollup.State {
				case "SUCCESS":
					pr.Checks = ChecksPassed
				case "ERROR", "FAILURE":
					pr.Checks = ChecksFailed
				case "EXPECTED", "PENDING":
					pr.Checks = ChecksPending
				default:
					return nil, errors.New("invalid pull request check status")
				}
			}
		}
		if !validListText(pr.Author) || !validListText(pr.LastModifier) {
			return nil, errors.New("invalid pull request list")
		}
		seen[item.Number] = true
		prs = append(prs, pr)
	}
	return prs, nil
}

func validListText(s string) bool {
	return utf8.ValidString(s) && len(s) <= 512 && !strings.ContainsAny(s, "\x00\r\n")
}

func (g *GH) Token(ctx context.Context) (string, error) {
	b, e := g.call(ctx, "auth", "token", "--hostname", "github.com")
	if e != nil {
		return "", fmt.Errorf("%w: %w", ErrAuthentication, e)
	}
	token := strings.TrimSpace(string(b))
	if token == "" || strings.ContainsAny(token, "\r\n\x00") {
		return "", ErrAuthentication
	}
	return token, nil
}
