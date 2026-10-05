package issuecontext

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"prui/internal/source"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const maxLinearIssues = 10

var issuePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,31}-[1-9][0-9]{0,9}$`)

// Only canonical HTTPS links are candidates; these never become trusted titles
// or statuses. Whitespace/Markdown delimiters bound scanning, not arbitrary URLs.
var candidatePattern = regexp.MustCompile(`https://[^\s<>"'\x60()\[\]]+`)

func LinearIdentity(link, workspace string) (string, bool) {
	if len(link) > 4096 || strings.IndexFunc(link, unicode.IsControl) >= 0 {
		return "", false
	}
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host != "linear.app" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 3 || len(parts) > 4 || parts[0] != workspace || parts[1] != "issue" || !issuePattern.MatchString(parts[2]) {
		return "", false
	}
	if len(parts) == 4 && parts[3] == "" {
		return "", false
	}
	return parts[2], true
}
func Candidates(body, workspace string) ([]string, bool) {
	seen := map[string]bool{}
	out := []string{}
	// The GitHub parser already caps body size; defend independent callers too.
	if len(body) > 1<<20 {
		return out, true
	}
	for _, link := range candidatePattern.FindAllString(body, -1) {
		id, ok := LinearIdentity(link, workspace)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		if len(out) == maxLinearIssues {
			return out, true
		}
		out = append(out, id)
	}
	return out, false
}

type Linear struct {
	Config    Config
	Getenv    func(string) string
	Transport http.RoundTripper
}

func (l Linear) Read(ctx context.Context, repository, body string) ([]source.ContextIssue, string) {
	if ctx.Err() != nil {
		return nil, "Linear context canceled"
	}
	c := l.Config
	if !c.Enabled {
		return nil, "Linear context unavailable: integration not configured"
	}
	if !c.valid() || !c.authorized(repository) {
		return nil, "Linear context unavailable: repository not authorized"
	}
	ids, more := Candidates(body, c.Workspace)
	if len(ids) == 0 {
		return nil, "No canonical Linear issue links detected in PR description"
	}
	if l.Getenv == nil {
		return nil, "Linear context unavailable: credential missing"
	}
	key := l.Getenv(c.CredentialEnv)
	if key == "" || len(key) > 8192 || strings.IndexFunc(key, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return nil, "Linear context unavailable: credential missing or invalid"
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	transport := l.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	out := []source.ContextIssue{}
	reason := ""
	budget := 0
	if more {
		reason = "Linear context partial: candidate limit reached"
	}
	for _, id := range ids {
		issue, used, err := readLinearIssue(ctx, client, key, c.Auth, id, c.Workspace, (2<<20)-budget)
		budget += used
		if err != nil || budget > 2<<20 {
			reason = "Linear context partial or unavailable: request failed or invalid response"
			if budget > 2<<20 || ctx.Err() != nil {
				break
			}
			continue
		}
		out = append(out, issue)
	}
	return out, reason
}
func readLinearIssue(ctx context.Context, client *http.Client, key, auth, id, workspace string, remaining int) (source.ContextIssue, int, error) {
	fail := errors.New("Linear issue unavailable")
	b, _ := json.Marshal(map[string]any{"query": `query($id:String!){issue(id:$id){identifier title description url state{name}}}`, "variables": map[string]string{"id": id}})
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/graphql", bytes.NewReader(b))
	if err != nil {
		return source.ContextIssue{}, 0, fail
	}
	secret := key
	r.Header.Set("Content-Type", "application/json")
	if auth == "oauth" {
		key = "Bearer " + key
	}
	r.Header.Set("Authorization", key)
	resp, err := client.Do(r)
	if err != nil {
		return source.ContextIssue{}, 0, fail
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return source.ContextIssue{}, 0, fail
	}
	limit := min(1<<20, max(0, remaining))
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil || len(data) > limit || !utf8.Valid(data) {
		return source.ContextIssue{}, len(data), fail
	}
	var p struct {
		Errors []json.RawMessage
		Data   struct {
			Issue *struct {
				Identifier, Title, Description, URL string
				State                               *struct{ Name string }
			}
		}
	}
	if json.Unmarshal(data, &p) != nil || len(p.Errors) > 0 || p.Data.Issue == nil {
		return source.ContextIssue{}, len(data), fail
	}
	v := p.Data.Issue
	linked, ok := LinearIdentity(v.URL, workspace)
	if strings.Contains(v.Title, secret) || strings.Contains(v.Description, secret) || strings.Contains(v.URL, secret) || (v.State != nil && strings.Contains(v.State.Name, secret)) || v.Identifier != id || !ok || linked != id || v.Title == "" || len(v.Title) > 4096 || len(v.Description) > 65536 || v.State == nil || v.State.Name == "" || len(v.State.Name) > 256 || strings.IndexFunc(v.State.Name, unicode.IsControl) >= 0 {
		return source.ContextIssue{}, len(data), fail
	}
	return source.ContextIssue{Identifier: v.Identifier, Title: v.Title, Status: v.State.Name, Description: v.Description, URL: v.URL}, len(data), nil
}
