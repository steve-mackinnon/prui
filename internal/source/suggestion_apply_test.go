package source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSuggestionRemoteCommitProtocol(t *testing.T) {
	for _, scenario := range []string{"success", "fork", "permission", "edited", "deleted", "confirm-edit", "push", "confirm-push", "race", "conflict", "mode", "timeout", "bad-response"} {
		t.Run(scenario, func(t *testing.T) {
			head, base, newHead := strings.Repeat("b", 40), strings.Repeat("a", 40), strings.Repeat("c", 40)
			repo := "owner/repo"
			if scenario == "fork" {
				repo = "fork/repo"
			}
			m := Metadata{Identity: Identity{"owner/repo", 1}, BaseRepository: "owner/repo", HeadRepository: repo, BaseSHA: base, HeadSHA: head}
			target := ReviewCommentTarget{Identity: m.Identity, CommitID: head, Path: "dir/a.go", Side: "RIGHT", StartLine: 2, StartSide: "RIGHT", Line: 3}
			body, _ := SuggestionBody("replacement")
			c := ReviewComment{ID: 9, Target: target, CurrentAnchor: &target, Body: body}
			prepared := false
			writes := 0
			gh := &GH{Executable: "gh", Limits: Defaults()}
			gh.Runner = listRunner(func(_ context.Context, q Request) ([]byte, error) {
				args := strings.Join(q.Args, " ")
				if strings.Contains(args, "pulls/comments/9") {
					if scenario == "deleted" {
						return nil, ErrCommand
					}
					remoteBody := body
					if scenario == "edited" || scenario == "confirm-edit" && prepared {
						remoteBody = "changed"
					}
					b, _ := json.Marshal(map[string]any{"id": 9, "body": remoteBody, "commit_id": head, "original_commit_id": head, "path": target.Path, "side": "RIGHT", "line": 3, "start_line": 2, "start_side": "RIGHT", "user": map[string]string{"login": "reviewer"}})
					return b, nil
				}
				if strings.Contains(args, "pulls/1") {
					sha := head
					if scenario == "push" || scenario == "confirm-push" && prepared {
						sha = newHead
					}
					return []byte(fmt.Sprintf(`{"number":1,"state":"open","base":{"sha":%q,"repo":{"full_name":"owner/repo"}},"head":{"sha":%q,"ref":"feature","repo":{"full_name":%q}}}`, base, sha, repo)), nil
				}
				if strings.Contains(args, "graphql") {
					var payload struct {
						Query     string
						Variables map[string]json.RawMessage
					}
					if json.Unmarshal(q.Stdin, &payload) != nil {
						t.Fatal("missing JSON stdin")
					}
					if strings.Contains(payload.Query, "createCommitOnBranch") {
						writes++
						if strings.Contains(args, "replacement") {
							t.Fatal("source in argv")
						}
						var input struct {
							Branch          struct{ RepositoryNameWithOwner, RefName string }
							ExpectedHeadOid string
							FileChanges     struct {
								Additions []struct{ Path, Contents string }
							}
						}
						if json.Unmarshal(payload.Variables["input"], &input) != nil {
							t.Fatal("input")
						}
						if input.Branch.RepositoryNameWithOwner != repo || input.Branch.RefName != "feature" || input.ExpectedHeadOid != head || len(input.FileChanges.Additions) != 1 || input.FileChanges.Additions[0].Path != target.Path {
							t.Fatalf("wrong write %+v", input)
						}
						content, _ := base64.StdEncoding.DecodeString(input.FileChanges.Additions[0].Contents)
						if string(content) != "first\nreplacement\nlast\n" {
							t.Fatal(string(content))
						}
						if scenario == "timeout" || scenario == "race" {
							return nil, ErrCommand
						}
						if scenario == "bad-response" {
							return []byte(`{"errors":[{"message":"untrusted"}]}`), nil
						}
						return []byte(fmt.Sprintf(`{"data":{"createCommitOnBranch":{"commit":{"oid":%q,"url":"https://github.com/%s/commit/%s"},"ref":{"target":{"oid":%q}}}}}`, newHead, repo, newHead, newHead)), nil
					}
					mode := 33188
					if scenario == "mode" {
						mode = 40960
					}
					return []byte(fmt.Sprintf(`{"data":{"repository":{"object":{"file":{"mode":%d,"type":"blob"}}}}}`, mode)), nil
				}
				if strings.Contains(args, "contents/") {
					content := "first\ntwo\nthree\nlast\n"
					if scenario == "conflict" {
						content = "first\nchanged\nthree\nlast\n"
					}
					b, _ := json.Marshal(map[string]any{"type": "file", "encoding": "base64", "path": target.Path, "size": len(content), "content": base64.StdEncoding.EncodeToString([]byte(content))})
					return b, nil
				}
				if strings.HasSuffix(args, "repos/"+repo) {
					return []byte(fmt.Sprintf(`{"permissions":{"push":%t}}`, scenario != "permission")), nil
				}
				t.Fatal("unexpected request", args)
				return nil, nil
			})
			a, err := gh.PrepareSuggestion(context.Background(), m, c, "two\nthree")
			switch scenario {
			case "permission", "edited", "deleted", "push", "conflict", "mode":
				if err == nil || writes != 0 {
					t.Fatal("preflight failed to block", err, writes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			prepared = true
			if len(a.Payload) == 0 {
				t.Fatal("attempt payload absent")
			}
			result, err := gh.ApplySuggestion(context.Background(), a)
			switch scenario {
			case "confirm-edit", "confirm-push":
				if err == nil || writes != 0 {
					t.Fatal("confirmation race wrote", err, writes)
				}
			case "timeout", "race", "bad-response":
				if !errors.Is(err, ErrCommentDeliveryUnknown) || writes != 1 {
					t.Fatal("uncertain outcome", err, writes)
				}
			default:
				if err != nil || result.SHA != newHead || writes != 1 {
					t.Fatal(result, err, writes)
				}
			}
		})
	}
}

func TestSuggestionOutcomeAndPreparedPayloadIntegrity(t *testing.T) {
	head, base, newHead := strings.Repeat("b", 40), strings.Repeat("a", 40), strings.Repeat("c", 40)
	m := Metadata{Identity: Identity{"owner/repo", 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: base, HeadSHA: head}
	body, _ := SuggestionBody("new")
	a := SuggestionApplication{Metadata: m, Target: ReviewCommentTarget{Identity: m.Identity, CommitID: head, Path: "a", Side: "RIGHT", Line: 1}, CommentID: 1, Branch: "feature", Before: "old", Replacement: "new", Content: "new\n", CommentBody: body}
	a.Payload = suggestionPayload(a)
	if ValidateSuggestionApplication(a) != nil {
		t.Fatal("valid payload refused")
	}
	changed := a
	changed.Content = "different"
	if ValidateSuggestionApplication(changed) == nil {
		t.Fatal("altered attempt accepted")
	}
	for _, scenario := range []string{"unchanged", "applied", "other-push", "force-back"} {
		t.Run(scenario, func(t *testing.T) {
			sha := newHead
			if scenario == "unchanged" || scenario == "force-back" {
				sha = head
			}
			content := a.Content
			if scenario == "other-push" {
				content = "new\nother\n"
			}
			gh := &GH{Executable: "gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, q Request) ([]byte, error) {
				args := strings.Join(q.Args, " ")
				if strings.Contains(args, "graphql") {
					return []byte(`{"data":{"repository":{"object":{"file":{"mode":33188,"type":"blob"}}}}}`), nil
				}
				if strings.Contains(args, "contents/") {
					b, _ := json.Marshal(map[string]any{"type": "file", "encoding": "base64", "path": "a", "size": len(content), "content": base64.StdEncoding.EncodeToString([]byte(content))})
					return b, nil
				}
				return []byte(fmt.Sprintf(`{"number":1,"base":{"sha":%q,"repo":{"full_name":"owner/repo"}},"head":{"sha":%q,"repo":{"full_name":"owner/repo"}}}`, base, sha)), nil
			})}
			found, err := gh.SuggestionOutcome(context.Background(), a)
			if scenario == "applied" {
				if !found || err != nil {
					t.Fatal(found, err)
				}
			} else if found || err == nil {
				t.Fatal("uncertain absence falsely proven", found, err)
			}
		})
	}
}

func TestSuggestionLargeAttemptJSONRoundTrip(t *testing.T) {
	m := Metadata{Identity: Identity{"owner/repo", 1}, BaseRepository: "owner/repo", HeadRepository: "owner/repo", BaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40)}
	replacement := strings.Repeat("r", 64<<10)
	body, _ := SuggestionBody(replacement)
	a := SuggestionApplication{Metadata: m, Target: ReviewCommentTarget{Identity: m.Identity, CommitID: m.HeadSHA, Path: "a", Side: "RIGHT", Line: 1}, CommentID: 1, Branch: "feature", Before: strings.Repeat("b", 64<<10), Replacement: replacement, CommentBody: body, Content: strings.Repeat("s", 512<<10)}
	a.Payload = suggestionPayload(a)
	encoded, err := json.Marshal(map[string]any{"Version": 3, "Attempt": "suggestion", "Attempted": map[string]any{"Kind": "suggestion", "Application": a}})
	if err != nil || len(encoded) > 1<<20 {
		t.Fatal("maximum attempt does not fit private draft", len(encoded), err)
	}
	var decoded struct {
		Attempted struct{ Application SuggestionApplication }
	}
	if json.Unmarshal(encoded, &decoded) != nil || ValidateSuggestionApplication(decoded.Attempted.Application) != nil || decoded.Attempted.Application.Content != a.Content || string(decoded.Attempted.Application.Payload) != string(a.Payload) {
		t.Fatal("exact payload lost on restart")
	}
}
