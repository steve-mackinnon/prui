package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type listRunner func(context.Context, Request) ([]byte, error)

func (r listRunner) Run(ctx context.Context, request Request) ([]byte, error) { return r(ctx, request) }

func TestPinnedIdentity(t *testing.T) {
	for _, tc := range []struct {
		input, repo string
		valid       bool
	}{
		{"https://github.com/owner/repo/pull/42", "", true}, {"42", "owner/repo", true},
		{"42", "", false}, {"https://evil.test/o/r/pull/1", "", false},
		{"https://github.com/o/r/pull/1?x=y", "", false}, {"-1", "o/r", false},
		{"https://user@github.com/o/r/pull/1", "", false}, {"1", "../repo", false},
	} {
		_, err := ParseIdentity(tc.input, tc.repo)
		if (err == nil) != tc.valid {
			t.Errorf("%q: %v", tc.input, err)
		}
	}
}

func TestSafetyProcessBounds(t *testing.T) {
	r := NewRunner()
	_, err := r.Run(context.Background(), Request{Program: "/bin/sh", Args: []string{"-c", "while :; do printf 1234567890; done"}, Limit: 100, Timeout: time.Second})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("output limit: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Run(ctx, Request{Program: "/bin/sleep", Args: []string{"10"}, Limit: 100, Timeout: time.Second})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	dir := t.TempDir()
	_, err = r.Run(context.Background(), Request{Program: "/bin/sh", Args: []string{"-c", "printf 1234567890 > data; sleep 10"}, Dir: dir, Limit: 100, Timeout: time.Second, StorageDir: dir, StorageLimit: 1})
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("storage limit: %v", err)
	}
}

func TestProcessRunnerProvidesBoundedStdin(t *testing.T) {
	out, err := NewRunner().Run(context.Background(), Request{
		Program: "/bin/sh", Args: []string{"-c", "IFS= read -r line; printf %s \"$line\""},
		Stdin: []byte("comment body\n"), Limit: 100, Timeout: time.Second,
	})
	if err != nil || string(out) != "comment body" {
		t.Fatalf("stdin output = %q, error = %v", out, err)
	}
}

func TestSafetyCredentialEnvironment(t *testing.T) {
	env := gitEnvironment(t.TempDir(), "synthetic-token")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "synthetic-token") || !strings.Contains(joined, "AUTHORIZATION: basic ") {
		t.Fatal("credential transport must be encoded environment-only header")
	}
	if !strings.Contains(joined, "GIT_CONFIG_NOSYSTEM=1") || !strings.Contains(joined, "GIT_NO_LAZY_FETCH=1") {
		t.Fatal("missing isolation")
	}
}

func TestSafetyRejectObjectSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, ".git", "objects", "ab")); err != nil {
		t.Fatal(err)
	}
	_, err := NewView(context.Background(), dir, NewRunner(), Defaults())
	if err == nil {
		t.Fatal("object symlink accepted")
	}
}

func TestGitHubListPullRequestsReadOnlyContract(t *testing.T) {
	calls := 0
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		calls++
		if len(request.Args) != 10 || !reflect.DeepEqual(request.Args[:4], []string{"api", "--hostname", "github.com", "graphql"}) || request.Args[4] != "-f" || !strings.Contains(request.Args[5], "pullRequests(first:100,states:OPEN") || !reflect.DeepEqual(request.Args[6:], []string{"-F", "owner=owner", "-F", "name=repo"}) {
			t.Fatal("unexpected GitHub operation", request.Args)
		}
		return []byte(`{"data":{"repository":{"pullRequests":{"nodes":[{"number":42,"title":"Add list","createdAt":"2026-09-23T12:00:00Z","author":{"login":"alice"},"commits":{"nodes":[{"commit":{"author":{"name":"Bob","user":{"login":"bob"}},"statusCheckRollup":{"state":"FAILURE"}}}]}}]}}}}`), nil
	})}
	prs, err := g.ListPullRequests(context.Background(), "owner/repo")
	if err != nil || len(prs) != 1 || prs[0].Identity != (Identity{Repository: "owner/repo", Number: 42}) || prs[0].Title != "Add list" || prs[0].Author != "alice" || prs[0].LastModifier != "bob" || prs[0].OpenedAt.IsZero() || prs[0].Checks != ChecksFailed || calls != 1 {
		t.Fatal(prs, err)
	}
	for _, body := range []string{`{}`, `{"errors":[{"message":"bad"}]}`, `{"data":{"repository":{"pullRequests":{"nodes":[{"number":0,"title":"bad","createdAt":"2026-09-23T12:00:00Z"}]}}}}`, `{"data":{"repository":{"pullRequests":{"nodes":[{"number":1,"title":"bad\n","createdAt":"2026-09-23T12:00:00Z"}]}}}}`} {
		g.Runner = listRunner(func(context.Context, Request) ([]byte, error) { return []byte(body), nil })
		if _, err := g.ListPullRequests(context.Background(), "owner/repo"); err == nil {
			t.Fatal("accepted invalid list", body)
		}
	}
}

func TestGitHubListPullRequestsCheckStates(t *testing.T) {
	for _, tc := range []struct {
		state string
		want  CheckStatus
	}{
		{"SUCCESS", ChecksPassed}, {"FAILURE", ChecksFailed}, {"ERROR", ChecksFailed},
		{"PENDING", ChecksPending}, {"EXPECTED", ChecksPending},
	} {
		t.Run(tc.state, func(t *testing.T) {
			g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
				return []byte(fmt.Sprintf(`{"data":{"repository":{"pullRequests":{"nodes":[{"number":1,"title":"Change","createdAt":"2026-09-23T12:00:00Z","commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":%q}}}]}}]}}}}`, tc.state)), nil
			})}
			prs, err := g.ListPullRequests(context.Background(), "owner/repo")
			if err != nil || len(prs) != 1 || prs[0].Checks != tc.want {
				t.Fatalf("status %s: %v, %v", tc.state, prs, err)
			}
		})
	}
}

func TestGitHubCreateReviewCommentUsesJSONStdin(t *testing.T) {
	const body = "Please handle the edge case.\n\nThanks!"
	calls := 0
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		calls++
		want := []string{"api", "--hostname", "github.com", "--method", "POST", "--input", "-", "repos/owner/repo/pulls/42/comments"}
		if !reflect.DeepEqual(request.Args, want) {
			t.Fatal("unexpected GitHub operation", request.Args)
		}
		if strings.Contains(strings.Join(request.Args, "\x00"), body) {
			t.Fatal("comment body leaked into process arguments")
		}
		var payload map[string]any
		if err := json.Unmarshal(request.Stdin, &payload); err != nil {
			t.Fatal(err)
		}
		wantPayload := map[string]any{
			"body": body, "commit_id": "0123456789abcdef0123456789abcdef01234567",
			"path": "internal/source/github.go", "line": float64(17), "side": "RIGHT",
		}
		if !reflect.DeepEqual(payload, wantPayload) {
			t.Fatalf("stdin payload = %#v, want %#v", payload, wantPayload)
		}
		return []byte(`{"id":7,"user":{"login":"reviewer"},"body":"Please handle the edge case.\n\nThanks!","commit_id":"0123456789abcdef0123456789abcdef01234567","path":"internal/source/github.go","side":"RIGHT","line":17}`), nil
	})}
	comment := ReviewComment{Target: ReviewCommentTarget{
		Identity: Identity{Repository: "owner/repo", Number: 42}, CommitID: "0123456789abcdef0123456789abcdef01234567",
		Path: "internal/source/github.go", Side: "RIGHT", Line: 17,
	}, Body: body}
	created, err := g.CreateReviewComment(context.Background(), comment)
	if err != nil || calls != 1 || created.ID != 7 || created.Author != "reviewer" || created.Target != comment.Target || created.Body != comment.Body {
		t.Fatal(created, err, calls)
	}
}

func TestCreatePullRequestReviewSendsPendingCommentsTogether(t *testing.T) {
	id := Identity{Repository: "owner/repo", Number: 42}
	sha := "0123456789abcdef0123456789abcdef01234567"
	comment := ReviewComment{Target: ReviewCommentTarget{Identity: id, CommitID: sha, Path: "a.go", Side: "RIGHT", Line: 9}, Body: "Please fix this."}
	called := 0
	g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		called++
		if !reflect.DeepEqual(request.Args, []string{"api", "--hostname", "github.com", "--method", "POST", "--input", "-", "repos/owner/repo/pulls/42/reviews"}) {
			t.Fatalf("unexpected args: %#v", request.Args)
		}
		if strings.Contains(strings.Join(request.Args, "\x00"), comment.Body) {
			t.Fatal("comment leaked into args")
		}
		var payload struct {
			CommitID string `json:"commit_id"`
			Event    string `json:"event"`
			Body     string `json:"body"`
			Comments []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Side string `json:"side"`
				Body string `json:"body"`
			} `json:"comments"`
		}
		if err := json.Unmarshal(request.Stdin, &payload); err != nil || payload.CommitID != sha || payload.Event != "REQUEST_CHANGES" || payload.Body != "Summary" || len(payload.Comments) != 1 || payload.Comments[0].Path != "a.go" || payload.Comments[0].Line != 9 || payload.Comments[0].Side != "RIGHT" || payload.Comments[0].Body != comment.Body {
			t.Fatalf("payload = %#v, err = %v", payload, err)
		}
		return []byte(`{"id":123,"state":"CHANGES_REQUESTED"}`), nil
	})}
	if err := g.CreatePullRequestReview(context.Background(), PullRequestReview{Identity: id, CommitID: sha, Event: "REQUEST_CHANGES", Body: "Summary", Comments: []ReviewComment{comment}}); err != nil || called != 1 {
		t.Fatal(err, called)
	}
	if err := g.CreatePullRequestReview(context.Background(), PullRequestReview{Identity: id, CommitID: sha, Event: "COMMENT", Comments: []ReviewComment{comment}}); err == nil || called != 1 {
		t.Fatal("invalid review reached GitHub", err, called)
	}
}

func TestGitHubListReviewCommentsUsesBoundedEndpointAndRejectsMalformedAnchors(t *testing.T) {
	id := Identity{Repository: "owner/repo", Number: 42}
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		want := []string{"api", "--hostname", "github.com", "--method", "GET", "repos/owner/repo/pulls/42/comments?per_page=100&page=1"}
		if !reflect.DeepEqual(request.Args, want) || request.Stdin != nil {
			t.Fatalf("request = %#v", request)
		}
		return []byte(`[{"id":7,"user":{"login":"reviewer"},"body":"body","commit_id":"0123456789abcdef0123456789abcdef01234567","path":"a.go","side":"RIGHT","line":2}]`), nil
	})}
	comments, err := g.ListReviewComments(context.Background(), id)
	if err != nil || len(comments) != 1 || comments[0].ID != 7 || comments[0].Author != "reviewer" || comments[0].Target.Path != "a.go" {
		t.Fatal(comments, err)
	}
	for _, raw := range []string{
		`[{"id":0,"user":{"login":"x"},"body":"x","commit_id":"0123456789abcdef0123456789abcdef01234567","path":"a","side":"RIGHT","line":1}]`,
		`[{"id":1,"user":{"login":"x"},"body":"x","commit_id":"bad","path":"a","side":"RIGHT","line":1}]`,
		`[{"id":1,"user":{"login":"x"},"body":"x","commit_id":"0123456789abcdef0123456789abcdef01234567","path":"a","side":"BOTH","line":1}]`,
	} {
		g.Runner = listRunner(func(context.Context, Request) ([]byte, error) { return []byte(raw), nil })
		if _, err := g.ListReviewComments(context.Background(), id); err == nil {
			t.Fatal("accepted malformed review comment", raw)
		}
	}
}

func TestGitHubCreateReviewCommentRejectsInvalidRequestBeforeCallingGH(t *testing.T) {
	valid := ReviewComment{Target: ReviewCommentTarget{
		Identity: Identity{Repository: "owner/repo", Number: 42}, CommitID: "0123456789abcdef0123456789abcdef01234567",
		Path: "internal/source/github.go", Side: "RIGHT", Line: 17,
	}, Body: "Please handle this."}
	for name, mutate := range map[string]func(*ReviewComment){
		"identity":  func(c *ReviewComment) { c.Target.Identity = Identity{Repository: "../repo", Number: 42} },
		"sha":       func(c *ReviewComment) { c.Target.CommitID = "not-a-sha" },
		"path":      func(c *ReviewComment) { c.Target.Path = "" },
		"path utf8": func(c *ReviewComment) { c.Target.Path = string([]byte{0xff}) },
		"side":      func(c *ReviewComment) { c.Target.Side = "BOTH" },
		"line":      func(c *ReviewComment) { c.Target.Line = 0 },
		"body":      func(c *ReviewComment) { c.Body = "" },
		"body utf8": func(c *ReviewComment) { c.Body = string([]byte{0xff}) },
	} {
		t.Run(name, func(t *testing.T) {
			comment := valid
			mutate(&comment)
			called := false
			g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
				called = true
				return nil, nil
			})}
			if _, err := g.CreateReviewComment(context.Background(), comment); err == nil {
				t.Fatal("accepted invalid comment")
			}
			if called {
				t.Fatal("called gh for invalid comment")
			}
		})
	}
}

func TestGitHubCreateReviewCommentDoesNotExposeBodyInErrors(t *testing.T) {
	const body = "private comment text"
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
		return nil, errors.New(body)
	})}
	_, err := g.CreateReviewComment(context.Background(), ReviewComment{Target: ReviewCommentTarget{
		Identity: Identity{Repository: "owner/repo", Number: 42}, CommitID: "0123456789abcdef0123456789abcdef01234567",
		Path: "internal/source/github.go", Side: "LEFT", Line: 17,
	}, Body: body})
	if err == nil || strings.Contains(err.Error(), body) {
		t.Fatalf("comment body leaked in error: %v", err)
	}
}

func TestGitHubReviewCommentActionsUseBoundedJSONContracts(t *testing.T) {
	id := Identity{Repository: "owner/repo", Number: 42}
	const body = "private reply"
	calls := 0
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
		calls++
		switch calls {
		case 1:
			if !reflect.DeepEqual(request.Args, []string{"api", "--hostname", "github.com", "--method", "GET", "user"}) {
				t.Fatal(request.Args)
			}
			return []byte(`{"login":"reviewer"}`), nil
		case 2:
			if !reflect.DeepEqual(request.Args, []string{"api", "--hostname", "github.com", "--method", "POST", "--input", "-", "repos/owner/repo/pulls/42/comments/7/replies"}) || strings.Contains(strings.Join(request.Args, "\x00"), body) {
				t.Fatal(request)
			}
			if string(request.Stdin) != `{"body":"private reply"}` {
				t.Fatal(string(request.Stdin))
			}
			return []byte(`{"id":8,"in_reply_to_id":7,"user":{"login":"reviewer"},"body":"private reply","commit_id":"0123456789abcdef0123456789abcdef01234567","path":"a.go","side":"RIGHT","line":2}`), nil
		case 3:
			if !reflect.DeepEqual(request.Args, []string{"api", "--hostname", "github.com", "--method", "POST", "--input", "-", "repos/owner/repo/pulls/comments/7/reactions"}) || string(request.Stdin) != `{"content":"+1"}` {
				t.Fatal(request)
			}
			return []byte(`{"id":9,"content":"+1","user":{"login":"reviewer"}}`), nil
		case 4:
			if !reflect.DeepEqual(request.Args, []string{"api", "--hostname", "github.com", "--method", "DELETE", "repos/owner/repo/pulls/comments/7"}) || request.Stdin != nil {
				t.Fatal(request)
			}
			return nil, nil
		}
		return nil, errors.New("unexpected")
	})}
	viewer, err := g.Viewer(context.Background())
	if err != nil || viewer.Login != "reviewer" {
		t.Fatal(viewer, err)
	}
	reply, err := g.ReplyToReviewComment(context.Background(), id, 7, body)
	if err != nil || reply.ID != 8 || reply.Body != body {
		t.Fatal(reply, err)
	}
	reaction, err := g.AddReviewCommentReaction(context.Background(), id, 7, "+1")
	if err != nil || reaction.Content != "+1" || reaction.ID != 9 {
		t.Fatal(reaction, err)
	}
	if err := g.DeleteReviewComment(context.Background(), id, 7); err != nil {
		t.Fatal(err)
	}
}

func TestGitHubReviewCommentActionsRejectInvalidInputWithoutCallingGH(t *testing.T) {
	g := GH{Executable: "trusted-gh", Dir: t.TempDir(), Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) { t.Fatal("called gh"); return nil, nil })}
	id := Identity{Repository: "owner/repo", Number: 42}
	for _, call := range []func() error{
		func() error { _, err := g.ReplyToReviewComment(context.Background(), id, 0, "body"); return err },
		func() error {
			_, err := g.ReplyToReviewComment(context.Background(), id, 7, string([]byte{0xff}))
			return err
		},
		func() error { _, err := g.AddReviewCommentReaction(context.Background(), id, 7, "shrug"); return err },
		func() error {
			return g.DeleteReviewComment(context.Background(), Identity{Repository: "bad/repo/extra", Number: 42}, 7)
		},
	} {
		if call() == nil {
			t.Fatal("accepted invalid comment action")
		}
	}
}

func TestGitHubListPullRequestsViewerReview(t *testing.T) {
	for _, state := range []string{"APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED", "PENDING", ""} {
		t.Run(state, func(t *testing.T) {
			review := "null"
			if state != "" {
				review = fmt.Sprintf(`{"state":%q}`, state)
			}
			g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, request Request) ([]byte, error) {
				if !strings.Contains(request.Args[5], "viewerLatestReview{state}") {
					t.Fatal("missing viewer review query")
				}
				return []byte(fmt.Sprintf(`{"data":{"repository":{"pullRequests":{"nodes":[{"number":1,"title":"Change","createdAt":"2026-09-23T12:00:00Z","viewerLatestReview":%s}]}}}}`, review)), nil
			})}
			prs, err := g.ListPullRequests(context.Background(), "owner/repo")
			if err != nil || len(prs) != 1 || prs[0].ViewerReview != state {
				t.Fatal(prs, err)
			}
		})
	}
}
