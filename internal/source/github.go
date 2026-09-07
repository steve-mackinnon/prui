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
type GitHub interface {
	Metadata(context.Context, Identity) (Metadata, error)
	Token(context.Context) (string, error)
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
	return g.Runner.Run(ctx, Request{Program: g.Executable, Args: args, Env: userEnvironment(), Dir: g.Dir, Limit: 1 << 20, Timeout: g.Limits.Operation})
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
