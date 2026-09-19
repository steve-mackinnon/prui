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
	"unicode/utf8"
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

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
}

type PullRequest struct {
	Identity Identity
	Title    string
}

type GitHub interface {
	Metadata(context.Context, Identity) (Metadata, error)
	ListPullRequests(context.Context, string) ([]PullRequest, error)
	Token(context.Context) (string, error)
}

// ReviewComment describes one explicitly requested, line-anchored pull request
// review comment. It is intentionally separate from GitHub, whose operations
// are otherwise read-only.
type ReviewComment struct {
	Target ReviewCommentTarget
	Body   string
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
	CreateReviewComment(context.Context, ReviewComment) error
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

func (g *GH) CreateReviewComment(ctx context.Context, comment ReviewComment) error {
	if err := validateReviewComment(comment); err != nil {
		return err
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
		return errors.New("could not prepare review comment")
	}
	_, err = g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/pulls/%d/comments", comment.Target.Identity.Repository, comment.Target.Identity.Number))
	if err != nil {
		return safeReviewCommentError(err)
	}
	return nil
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
		Number     int `json:"number"`
		Base, Head side
	}
	if json.Unmarshal(data, &raw) != nil || raw.Number != id.Number || raw.Base.Repo == nil || raw.Head.Repo == nil {
		return Metadata{}, errors.New("invalid metadata or deleted fork")
	}
	m := Metadata{id, raw.Base.Repo.FullName, raw.Head.Repo.FullName, raw.Base.SHA, raw.Head.SHA}
	if !repositoryPattern.MatchString(m.BaseRepository) || !repositoryPattern.MatchString(m.HeadRepository) || !strings.EqualFold(m.BaseRepository, id.Repository) || !shaPattern.MatchString(m.BaseSHA) || !shaPattern.MatchString(m.HeadSHA) {
		return Metadata{}, errors.New("unsupported repository or revision metadata")
	}
	return m, nil
}

func (g *GH) ListPullRequests(ctx context.Context, repository string) ([]PullRequest, error) {
	if _, err := ParseIdentity("1", repository); err != nil {
		return nil, err
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("repos/%s/pulls?state=open&per_page=100", repository))
	if err != nil {
		return nil, fmt.Errorf("GitHub pull request list unavailable (check authentication and connectivity): %w", err)
	}
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) > 100 {
		return nil, errors.New("invalid pull request list")
	}
	prs := make([]PullRequest, 0, len(raw))
	seen := map[int]bool{}
	for _, pr := range raw {
		if pr.Number <= 0 || seen[pr.Number] || strings.ContainsAny(pr.Title, "\x00\r\n") {
			return nil, errors.New("invalid pull request list")
		}
		seen[pr.Number] = true
		prs = append(prs, PullRequest{Identity: Identity{Repository: repository, Number: pr.Number}, Title: pr.Title})
	}
	return prs, nil
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
