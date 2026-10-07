package source

import (
	"context"
	"errors"
	"fmt"
)

// SourceBlob reads an immutable Git blob, with no checkout or branch fallback.
func (g *GH) SourceBlob(ctx context.Context, repository, oid string, limit int) ([]byte, error) {
	if !repositoryPattern.MatchString(repository) || !shaPattern.MatchString(oid) || limit <= 0 || limit > 1<<20 {
		return nil, errors.New("invalid source blob request")
	}
	return g.Runner.Run(ctx, Request{Program: g.Executable, Args: []string{"api", "--hostname", "github.com", "--method", "GET", "-H", "Accept: application/vnd.github.raw+json", fmt.Sprintf("repos/%s/git/blobs/%s", repository, oid)}, Env: userEnvironment(), Dir: g.Dir, Limit: limit, Timeout: g.Limits.Operation})
}
