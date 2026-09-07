package source

import (
	"context"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSafetyCredentialTransportRealGit(t *testing.T) {
	received := make(chan string, 10)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	dir := t.TempDir()
	cert := filepath.Join(dir, "ca.pem")
	if e := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	env := gitEnvironment(dir, "synthetic-token")
	// Only this test redirects the host-scoped header to a loopback TLS server.
	for i, s := range env {
		env[i] = strings.ReplaceAll(s, "http.https://github.com/.extraHeader", "http."+server.URL+"/.extraHeader")
	}
	git, e := trustedExecutable("git")
	if e != nil {
		t.Fatal(e)
	}
	_, e = NewRunner().Run(context.Background(), Request{Program: git, Args: []string{"-c", "http.sslCAInfo=" + cert, "ls-remote", server.URL + "/fixture.git"}, Env: env, Dir: dir, Limit: 1000, Timeout: 5 * time.Second})
	if e == nil {
		t.Fatal("unauthorized response accepted")
	}
	select {
	case auth := <-received:
		if auth != "basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:synthetic-token")) {
			t.Fatal("host-scoped credential header missing")
		}
	default:
		t.Fatal("Git did not reach local TLS fixture")
	}
}
