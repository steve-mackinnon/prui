package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

type readRunner func(context.Context, Request) ([]byte, error)

func (r readRunner) Run(ctx context.Context, req Request) ([]byte, error) { return r(ctx, req) }

type readRoundTripper func(*http.Request) (*http.Response, error)

func (r readRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }

func TestGitHubReadFreshCredentialsAndFixedRequest(t *testing.T) {
	credentials, requests := 0, 0
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, req Request) ([]byte, error) {
		if strings.Join(req.Args, " ") != "auth token --hostname github.com" {
			t.Fatalf("unexpected CLI request: %v", req.Args)
		}
		credentials++
		return []byte(strings.Repeat("x", credentials)), nil
	})}
	g.readClient = &http.Client{Transport: readRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != "GET" || req.URL.String() != "https://api.github.com/repos/owner/repo/pulls/1" || req.Header.Get("Authorization") != "Bearer "+strings.Repeat("x", requests) || req.Header.Get("Accept") != "application/vnd.github+json" || req.Header.Get("User-Agent") != "prui" || req.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
			t.Fatal("incorrect read request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}
	for range 2 {
		data, err := g.readGET(context.Background(), "repos/owner/repo/pulls/1")
		if err != nil || string(data) != "{}" {
			t.Fatalf("read = %q, %v", data, err)
		}
	}
	if credentials != 2 || requests != 2 {
		t.Fatal("credentials or response reused")
	}
}

func TestGitHubReadBoundsAndNoFallbackAfterDispatch(t *testing.T) {
	for _, tc := range []struct {
		name               string
		status, size       int
		transportErr, want error
	}{
		{"limit", 200, (1 << 20) + 1, nil, ErrLimit},
		{"status", 403, 20, nil, ErrCommand},
		{"transport", 0, 0, errors.New("secret transport detail"), ErrCommand},
		{"exact limit", 200, 1 << 20, nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, requests := 0, 0
			g := &GH{Limits: Defaults(), Runner: readRunner(func(context.Context, Request) ([]byte, error) { calls++; return []byte("token"), nil })}
			g.readClient = &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
				requests++
				if tc.transportErr != nil {
					return nil, tc.transportErr
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", tc.size)))}, nil
			})}
			_, err := g.readGET(context.Background(), "repos/owner/repo/pulls/1")
			if !errors.Is(err, tc.want) || calls != 1 || requests != 1 {
				t.Fatalf("err %v, CLI %d, HTTP %d", err, calls, requests)
			}
		})
	}
}

func TestGitHubReadAuthenticationCompatibilityAndCancellation(t *testing.T) {
	for _, failure := range []error{ErrCommand, ErrLimit, context.Canceled, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			calls := 0
			g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, req Request) ([]byte, error) {
				calls++
				if req.Args[0] == "auth" {
					return nil, failure
				}
				return []byte("legacy"), nil
			})}
			g.readClient = &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
				t.Fatal("HTTP after failed authentication")
				return nil, ErrCommand
			})}
			data, err := g.readGET(context.Background(), "repos/owner/repo/pulls/1")
			if failure == ErrCommand {
				if err != nil || string(data) != "legacy" || calls != 2 {
					t.Fatalf("legacy read = %q, %v, %d", data, err, calls)
				}
			} else if !errors.Is(err, failure) || calls != 1 {
				t.Fatalf("failure = %v, calls %d", err, calls)
			}
		})
	}
	g := &GH{Limits: Defaults(), Runner: readRunner(func(context.Context, Request) ([]byte, error) { t.Fatal("CLI after cancellation"); return nil, nil })}
	g.readClient = newGitHubReadClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.readGET(ctx, "repos/owner/repo/pulls/1"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestGitHubReadDeadlineIncludesAuthentication(t *testing.T) {
	g := &GH{Limits: Defaults(), Runner: readRunner(func(ctx context.Context, _ Request) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() })}
	g.Limits.Operation = time.Millisecond
	g.readClient = newGitHubReadClient()
	if _, err := g.readGET(context.Background(), "repos/owner/repo/pulls/1"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestGitHubReadConnectionReuseAndValidation(t *testing.T) {
	base, head := strings.Repeat("a", 40), strings.Repeat("b", 40)
	metadata := `{"number":1,"body":"description","base":{"sha":"` + base + `","repo":{"full_name":"owner/repo"}},"head":{"sha":"` + head + `","repo":{"full_name":"owner/repo"}}}`
	graphqlMetadata := `{"data":{"repository":{"pullRequest":{"number":1,"body":"description","baseRefName":"","baseRefOid":"` + base + `","headRefOid":"` + head + `","baseRepository":{"nameWithOwner":"owner/repo"},"headRepository":{"nameWithOwner":"owner/repo"}}}}}`
	commits := `[{"sha":"` + head + `","commit":{"message":"subject\nbody","author":{"name":"author"}}}]`
	var connections []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		connections = append(connections, req.RemoteAddr)
		if req.URL.RawQuery != "" {
			_, _ = io.WriteString(w, commits)
		} else {
			if req.Method != "POST" || req.URL.Path != "/graphql" {
				t.Error("incorrect metadata request")
			}
			_, _ = io.WriteString(w, graphqlMetadata)
		}
	}))
	defer server.Close()
	client := newGitHubReadClient()
	transport := client.Transport.(*http.Transport)
	defer transport.CloseIdleConnections()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig = &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "api.github.com:443" {
			return nil, errors.New("unexpected destination")
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	runner := readRunner(func(_ context.Context, req Request) ([]byte, error) {
		if req.Args[0] == "auth" {
			return []byte("token"), nil
		}
		if strings.Contains(req.Args[len(req.Args)-1], "/commits?") {
			return []byte(commits), nil
		}
		return []byte(metadata), nil
	})
	g := &GH{Runner: runner, Limits: Defaults(), readClient: client}
	legacy := &GH{Runner: runner, Limits: Defaults()}
	id := Identity{"owner/repo", 1}
	for range 2 {
		got, err := g.Metadata(context.Background(), id)
		want, wantErr := legacy.Metadata(context.Background(), id)
		if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("metadata differs: %v, %v", err, wantErr)
		}
	}
	got, err := g.ListPullRequestCommits(context.Background(), id)
	want, wantErr := legacy.ListPullRequestCommits(context.Background(), id)
	if err != nil || wantErr != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("commits differ: %v, %v", err, wantErr)
	}
	if len(connections) != 3 || connections[0] != connections[1] || connections[1] != connections[2] {
		t.Fatalf("connections not reused: %v", connections)
	}
}

func TestGitHubReadRedirectPolicy(t *testing.T) {
	client := newGitHubReadClient()
	for _, tc := range []struct {
		url     string
		allowed bool
	}{
		{"https://api.github.com/repositories/1/pulls/1", true},
		{"http://api.github.com/repos/owner/repo/pulls/1", false},
		{"https://github.com/repos/owner/repo/pulls/1", false},
		{"https://api.github.com.attacker.invalid/", false},
		{"https://user@api.github.com/", false},
	} {
		req, err := http.NewRequest(http.MethodGet, tc.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := client.CheckRedirect(req, nil); (got == nil) != tc.allowed {
			t.Fatalf("redirect %s: %v", tc.url, got)
		}
		if err := client.CheckRedirect(req, make([]*http.Request, 10)); err == nil {
			t.Fatal("unbounded redirects")
		}
	}
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || transport.MaxResponseHeaderBytes != 64<<10 || transport.MaxConnsPerHost != 4 {
		t.Fatal("unbounded transport or environment proxy")
	}
}

func TestGitHubReadDoesNotChangeWritesOrCustomRunners(t *testing.T) {
	for _, client := range []*http.Client{nil, {Transport: readRoundTripper(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP write"); return nil, ErrCommand })}} {
		g := &GH{readClient: client, Limits: Defaults(), Runner: readRunner(func(_ context.Context, req Request) ([]byte, error) {
			if strings.Join(req.Args, " ") != "api --method POST --input - endpoint" || string(req.Stdin) != "payload" {
				t.Fatal("changed CLI write")
			}
			return []byte("result"), nil
		})}
		if data, err := g.callWithStdin(context.Background(), []byte("payload"), "api", "--method", "POST", "--input", "-", "endpoint"); err != nil || string(data) != "result" {
			t.Fatal(err)
		}
	}
	g := &GH{Limits: Defaults(), Runner: readRunner(func(_ context.Context, req Request) ([]byte, error) {
		if strings.Join(req.Args, " ") != "api --hostname github.com --method GET repos/owner/repo/pulls/1" {
			t.Fatal("changed custom runner request")
		}
		return []byte("legacy"), nil
	})}
	if data, err := g.readGET(context.Background(), "repos/owner/repo/pulls/1"); err != nil || string(data) != "legacy" {
		t.Fatal(err)
	}
}

type interruptedReadBody struct {
	cancel context.CancelFunc
	closed bool
}

func (b *interruptedReadBody) Read([]byte) (int, error) {
	if b.cancel != nil {
		b.cancel()
	}
	return 0, io.ErrUnexpectedEOF
}

func (b *interruptedReadBody) Close() error { b.closed = true; return nil }

func TestGitHubReadInterruptedBodyIsClosedWithoutRetry(t *testing.T) {
	for _, cancelBody := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		body := &interruptedReadBody{}
		want := ErrCommand
		if cancelBody {
			body.cancel = cancel
			want = context.Canceled
		}
		calls := 0
		g := &GH{Limits: Defaults(), Runner: readRunner(func(context.Context, Request) ([]byte, error) { return []byte("token"), nil })}
		g.readClient = &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: body}, nil
		})}
		data, err := g.readGET(ctx, "repos/owner/repo/pulls/1")
		cancel()
		if !errors.Is(err, want) || len(data) != 0 || calls != 1 || !body.closed {
			t.Fatalf("interrupted read = %q, %v, calls %d, closed %v", data, err, calls, body.closed)
		}
	}
}

func TestGitHubReadPreservesResponseValidation(t *testing.T) {
	for _, response := range []string{"null", "{}", "[]", "invalid", string([]byte{0xff}), `{"number":1}`} {
		runner := readRunner(func(_ context.Context, req Request) ([]byte, error) {
			if req.Args[0] == "auth" {
				return []byte("token"), nil
			}
			return []byte(response), nil
		})
		g := &GH{Runner: runner, Limits: Defaults(), readClient: &http.Client{Transport: readRoundTripper(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})}}
		legacy := &GH{Runner: runner, Limits: Defaults()}
		id := Identity{"owner/repo", 1}
		_, got := g.Metadata(context.Background(), id)
		_, want := legacy.Metadata(context.Background(), id)
		if got == nil || want == nil || got.Error() != want.Error() {
			t.Fatalf("metadata validation changed: %v, %v", got, want)
		}
		_, got = g.ListPullRequestCommits(context.Background(), id)
		_, want = legacy.ListPullRequestCommits(context.Background(), id)
		if (got == nil) != (want == nil) || (got != nil && got.Error() != want.Error()) {
			t.Fatalf("commit validation changed: %v, %v", got, want)
		}
	}
}
