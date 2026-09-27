package guide

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"sync/atomic"
)

const (
	fantasyRequestByteLimit  = 2 << 20
	fantasyResponseByteLimit = 4 << 20
)

// NewBoundedHTTPClient gives provider SDKs the application's upload and
// download limits. It clones base so the caller's redirect and transport
// settings are not changed. Every provider request must use this client.
func NewBoundedHTTPClient(base *http.Client) *http.Client {
	var client http.Client
	if base != nil {
		client = *base
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = boundedGuideTransport{base: transport}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("provider redirect refused")
	}
	return &client
}

// newSingleRequestHTTPClient is invocation scoped. Even if a provider SDK
// changes its retry defaults, the same confirmed guide action cannot upload
// source a second time.
func newSingleRequestHTTPClient(base *http.Client) *http.Client {
	client := NewBoundedHTTPClient(base)
	client.Transport = &singleRequestTransport{base: client.Transport}
	return client
}

type singleRequestTransport struct {
	base http.RoundTripper
	used atomic.Bool
}

func (t *singleRequestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !t.used.CompareAndSwap(false, true) {
		return nil, errors.New("guide request already sent")
	}
	return t.base.RoundTrip(r)
}

type boundedGuideTransport struct {
	base http.RoundTripper
}

func (t boundedGuideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil || req.URL.Host == "" || req.URL.User != nil {
		return nil, errors.New("invalid provider request URL")
	}
	if req.URL.Scheme != "https" && (req.URL.Scheme != "http" || !guideLoopback(req.URL.Hostname())) {
		return nil, errors.New("provider request requires HTTPS")
	}
	copy := req.Clone(req.Context())
	if req.Body != nil {
		if req.ContentLength > fantasyRequestByteLimit {
			_ = req.Body.Close()
			return nil, errors.New("request exceeds the byte budget")
		}
		body, err := io.ReadAll(io.LimitReader(req.Body, fantasyRequestByteLimit+1))
		_ = req.Body.Close()
		if err != nil {
			return nil, errors.New("provider request could not be read")
		}
		if len(body) > fantasyRequestByteLimit {
			return nil, errors.New("request exceeds the byte budget")
		}
		copy.Body = io.NopCloser(bytes.NewReader(body))
		copy.ContentLength = int64(len(body))
		copy.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	resp, err := t.base.RoundTrip(copy)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, errors.New("provider response has no body")
	}
	if resp.ContentLength > fantasyResponseByteLimit {
		_ = resp.Body.Close()
		return nil, errors.New("response exceeds the byte limit")
	}
	// Read before returning to the SDK, so even an SDK that ignores a read
	// error cannot accept an oversized provider response.
	body, err := io.ReadAll(io.LimitReader(resp.Body, fantasyResponseByteLimit+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, errors.New("provider response could not be read")
	}
	if len(body) > fantasyResponseByteLimit {
		return nil, errors.New("response exceeds the byte limit")
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	return resp, nil
}

func (t boundedGuideTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func guideLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
