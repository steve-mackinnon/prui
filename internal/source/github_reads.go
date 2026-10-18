package source

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// Only the two validated PR capture endpoints use this client. Connections are
// reused; credentials and responses are not. All other operations retain gh.
func newGitHubReadClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Match the CLI's restricted environment, which excludes proxy settings.
	transport.Proxy = nil
	transport.MaxIdleConns = 4
	transport.MaxIdleConnsPerHost = 4
	transport.MaxConnsPerHost = 4
	transport.IdleConnTimeout = 90 * time.Second
	transport.MaxResponseHeaderBytes = 64 << 10
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Repository moves may redirect within the API. Credentials must never
			// follow a different origin, downgrade, or unbounded redirect chain.
			if len(via) >= 10 || req.URL.Scheme != "https" || req.URL.Host != "api.github.com" || req.URL.User != nil {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
}

func (g *GH) readGET(parent context.Context, endpoint string) ([]byte, error) {
	legacy := func(ctx context.Context) ([]byte, error) {
		return g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", endpoint)
	}
	if g.readClient == nil {
		return legacy(parent)
	}
	if g.Limits.Operation <= 0 {
		return nil, ErrCommand
	}
	ctx, cancel := context.WithTimeout(parent, g.Limits.Operation)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Resolve the active CLI account on every read, including account switches.
	// Do not keep a token that could disagree with a subsequent CLI write.
	token, err := g.Token(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrLimit) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Preserve CLI authentication behavior when token export is unavailable.
		// No HTTP request has been attempted; never fall back after dispatch.
		return legacy(ctx)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/"+endpoint, nil)
	if err != nil {
		return nil, ErrCommand
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "prui")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.readClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrCommand
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(data) > 1<<20 {
		return nil, ErrLimit
	}
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, ErrCommand
	}
	return data, nil
}
