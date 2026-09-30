package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// PullRequestCommitReader is optional so read-only clients need not support capture.
type PullRequestCommitReader interface {
	ListPullRequestCommits(context.Context, Identity) ([]PullRequestCommit, error)
}

type PullRequestCommit struct{ SHA, Subject, Author string }

// ValidatePullRequestCommits validates normalized metadata without retaining bodies.
func ValidatePullRequestCommits(entries []PullRequestCommit) error {
	if len(entries) > 100 {
		return ErrLimit
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if !shaPattern.MatchString(e.SHA) || seen[e.SHA] || !utf8.ValidString(e.Subject) || !utf8.ValidString(e.Author) || len(e.Subject) > 4096 || len(e.Author) > 256 || strings.ContainsAny(e.Subject, "\r\n") {
			return errors.New("invalid commit metadata")
		}
		seen[e.SHA] = true
	}
	return nil
}

func (g *GH) ListPullRequestCommits(ctx context.Context, id Identity) ([]PullRequestCommit, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return nil, errors.New("invalid commit list identity")
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("repos/%s/pulls/%d/commits?per_page=100&page=1", id.Repository, id.Number))
	if err != nil {
		return nil, err
	}
	var records []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"commit"`
	}
	if !utf8.Valid(data) || json.Unmarshal(data, &records) != nil || records == nil {
		return nil, errors.New("invalid commit list response")
	}
	entries := make([]PullRequestCommit, len(records))
	for i, r := range records {
		subject, _, _ := strings.Cut(r.Commit.Message, "\n")
		entries[i] = PullRequestCommit{r.SHA, strings.TrimSuffix(subject, "\r"), r.Commit.Author.Name}
	}
	if err := ValidatePullRequestCommits(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CommitParents checks object identity and membership against immutable ancestry.
func (v *View) CommitParents(ctx context.Context, sha, head string) ([]string, error) {
	if !shaPattern.MatchString(sha) || !shaPattern.MatchString(head) {
		return nil, errors.New("invalid commit identity")
	}
	if _, err := v.Git(ctx, 100, "cat-file", "-e", sha+"^{commit}"); err != nil {
		return nil, err
	}
	if _, err := v.Git(ctx, 100, "merge-base", "--is-ancestor", sha, head); err != nil {
		return nil, err
	}
	data, err := v.Git(ctx, 8192, "rev-list", "--parents", "--max-count=1", sha, "--")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(data))
	if len(ids) == 0 || ids[0] != sha {
		return nil, errors.New("invalid commit ancestry")
	}
	for _, parent := range ids[1:] {
		if !shaPattern.MatchString(parent) {
			return nil, errors.New("invalid parent identity")
		}
	}
	if len(ids) > 1 {
		if _, err := v.Git(ctx, 100, "cat-file", "-e", ids[1]+"^{commit}"); err != nil {
			return nil, err
		}
	}
	return ids[1:], nil
}

// EmptyTree materializes the standard empty tree only in isolated storage.
func (v *View) EmptyTree(ctx context.Context) (string, error) {
	data, err := v.Git(ctx, 100, "hash-object", "-t", "tree", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	oid := strings.TrimSpace(string(data))
	if !shaPattern.MatchString(oid) {
		return "", errors.New("invalid empty tree identity")
	}
	return oid, nil
}
