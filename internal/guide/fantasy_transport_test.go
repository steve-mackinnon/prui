package guide

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSingleGuideHTTPClientBlocksSecondRequest(t *testing.T) {
	requests := 0
	base := &http.Client{Transport: guideRoundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}
	client := newSingleRequestHTTPClient(base)
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/messages", strings.NewReader("synthetic source"))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if attempt == 0 && (err != nil || resp.StatusCode != http.StatusServiceUnavailable) {
			t.Fatalf("first request: response=%#v error=%v", resp, err)
		}
		if attempt == 1 && err == nil {
			t.Fatal("second request was sent")
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
	if requests != 1 {
		t.Fatalf("outbound requests = %d, want one", requests)
	}
}

type guideRoundTripFunc func(*http.Request) (*http.Response, error)

func (f guideRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestBoundedHTTPClientAllowsBoundedLoopbackRequest(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || !bytes.Equal(body, []byte("guide input")) {
			t.Errorf("unexpected request: %s %q", r.Method, body)
		}
		_, _ = w.Write([]byte("guide result"))
	}))
	defer server.Close()

	client := NewBoundedHTTPClient(nil)
	resp, err := client.Post(server.URL, "text/plain", strings.NewReader("guide input"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != "guide result" || count.Load() != 1 {
		t.Fatalf("response %q, err %v, requests %d", got, err, count.Load())
	}
}

func TestBoundedHTTPClientRejectsLargeRequestBeforeUpload(t *testing.T) {
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		count.Add(1)
	}))
	defer server.Close()

	for _, knownLength := range []bool{true, false} {
		body := strings.NewReader(strings.Repeat("x", fantasyRequestByteLimit+1))
		req, err := http.NewRequest(http.MethodPost, server.URL, body)
		if err != nil {
			t.Fatal(err)
		}
		if !knownLength {
			req.ContentLength = -1
		}
		resp, err := NewBoundedHTTPClient(nil).Do(req)
		if resp != nil {
			resp.Body.Close()
			t.Fatal("oversized request returned a response")
		}
		if err == nil || !strings.Contains(err.Error(), "request exceeds the byte budget") {
			t.Fatalf("knownLength=%v: got %v", knownLength, err)
		}
	}
	if count.Load() != 0 {
		t.Fatalf("uploaded oversized request %d times", count.Load())
	}
}

func TestBoundedHTTPClientRejectsLargeResponse(t *testing.T) {
	for _, knownLength := range []bool{true, false} {
		t.Run(map[bool]string{true: "known", false: "streamed"}[knownLength], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if knownLength {
					w.Header().Set("Content-Length", "4194305")
				} else {
					w.(http.Flusher).Flush()
				}
				_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", fantasyResponseByteLimit+1)))
			}))
			defer server.Close()
			resp, err := NewBoundedHTTPClient(nil).Get(server.URL)
			if resp != nil {
				resp.Body.Close()
				t.Fatal("oversized response reached caller")
			}
			if err == nil || !strings.Contains(err.Error(), "response exceeds the byte limit") {
				t.Fatal("oversized response was accepted", err)
			}
		})
	}
}

func TestBoundedHTTPClientRefusesRedirect(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	base := &http.Client{}
	client := NewBoundedHTTPClient(base)
	resp, err := client.Post(source.URL, "text/plain", strings.NewReader("source and credential"))
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "provider redirect refused") {
		t.Fatalf("redirect was accepted: response=%v err=%v", resp, err)
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect received source")
	}
	if base.CheckRedirect != nil || base.Transport != nil {
		t.Fatal("caller HTTP client was mutated")
	}
}

func TestBoundedHTTPClientRequiresHTTPSForRemoteHost(t *testing.T) {
	transport := &countingGuideTransport{}
	client := NewBoundedHTTPClient(&http.Client{Transport: transport})
	for _, rawURL := range []string{
		"http://example.com/v1", "http://localhost.evil.example/v1", "http://192.0.2.1/v1",
		"https://user:password@example.com/v1",
	} {
		resp, err := client.Get(rawURL)
		if resp != nil {
			resp.Body.Close()
			t.Fatalf("unsafe URL reached provider: %s", rawURL)
		}
		if err == nil {
			t.Fatalf("unsafe URL accepted: %s", rawURL)
		}
	}
	if transport.count.Load() != 0 {
		t.Fatalf("unsafe URL reached underlying transport %d times", transport.count.Load())
	}
}

type countingGuideTransport struct{ count atomic.Int32 }

func (t *countingGuideTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.count.Add(1)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}
