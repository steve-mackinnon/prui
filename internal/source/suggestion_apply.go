package source

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// SuggestionApplication is the exact prepared remote write retained privately
// before dispatch. Content is never placed in argv, snapshot data or diagnostics.
type SuggestionApplication struct {
	Metadata                    Metadata
	Target                      ReviewCommentTarget
	CommentID                   int64
	Branch, Before, Replacement string
	Content                     string `json:"-"`
	CommentBody                 string
	Payload                     json.RawMessage
}
type SuggestionResult struct{ SHA, URL string }
type SuggestionApplier interface {
	PrepareSuggestion(context.Context, Metadata, ReviewComment, string) (SuggestionApplication, error)
	ApplySuggestion(context.Context, SuggestionApplication) (SuggestionResult, error)
	SuggestionOutcome(context.Context, SuggestionApplication) (bool, error)
}

func ValidateSuggestionApplication(a SuggestionApplication) error {
	if ValidateReviewCommentTarget(a.Target) != nil || a.Target.Identity != a.Metadata.Identity || a.Target.CommitID != a.Metadata.HeadSHA || a.Target.Side != "RIGHT" || a.Target.SubjectType != "" || !repositoryPattern.MatchString(a.Metadata.HeadRepository) || !shaPattern.MatchString(a.Metadata.BaseSHA) || !repositoryPattern.MatchString(a.Metadata.BaseRepository) || a.Branch == "" || strings.ContainsAny(a.Branch, "\x00\n\r") || len(a.Content) > 512<<10 || !utf8.ValidString(a.Content) || strings.ContainsRune(a.Content, 0) || a.CommentID <= 0 || len(a.Before) > 64<<10 || len(a.CommentBody) > 128<<10 || a.Target.Path == "" || strings.HasPrefix(a.Target.Path, "/") || strings.Contains(a.Target.Path, "../") {
		return errors.New("invalid suggestion application")
	}
	parsed, err := ParseSuggestion(a.CommentBody)
	if err != nil || parsed != a.Replacement {
		return errors.New("prepared suggestion body mismatch")
	}
	if _, err := SuggestionBody(a.Replacement); err != nil {
		return err
	}
	if !bytes.Equal(a.Payload, suggestionPayload(a)) {
		return errors.New("invalid prepared suggestion payload")
	}
	return nil
}
func (g *GH) suggestionBranch(ctx context.Context, m Metadata) (string, error) {
	current, err := g.Metadata(ctx, m.Identity)
	if err != nil {
		return "", err
	}
	if !SamePinnedRevision(current, m) {
		return "", errors.New("stale suggestion: open a new comparison")
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", fmt.Sprintf("repos/%s/pulls/%d", m.Identity.Repository, m.Identity.Number))
	if err != nil {
		return "", err
	}
	var raw struct {
		State string
		Head  struct {
			Ref, SHA string
			Repo     struct {
				FullName string `json:"full_name"`
			}
		}
	}
	if json.Unmarshal(data, &raw) != nil || raw.State != "open" || raw.Head.SHA != m.HeadSHA || raw.Head.Repo.FullName != m.HeadRepository || raw.Head.Ref == "" {
		return "", errors.New("stale or closed suggestion pull request")
	}
	data, err = g.call(ctx, "api", "--hostname", "github.com", "repos/"+m.HeadRepository)
	if err != nil {
		return "", err
	}
	var repo struct{ Permissions struct{ Push bool } }
	if json.Unmarshal(data, &repo) != nil || !repo.Permissions.Push {
		return "", errors.New("suggestion requires write permission on the head repository; fork maintainer application unsupported")
	}
	return raw.Head.Ref, nil
}
func (g *GH) suggestionContent(ctx context.Context, repo, sha, path string) (string, error) {
	parts := strings.Split(repo, "/")
	query := `query($owner:String!,$name:String!,$expression:String!,$path:String!){repository(owner:$owner,name:$name){object(expression:$expression){... on Commit{file(path:$path){mode type}}}}}`
	payload, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]string{"owner": parts[0], "name": parts[1], "expression": sha, "path": path}})
	modeData, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "graphql", "--input", "-")
	if err != nil {
		return "", err
	}
	var entry struct {
		Errors []json.RawMessage
		Data   struct {
			Repository struct {
				Object struct {
					File struct {
						Mode int
						Type string
					}
				}
			}
		}
	}
	if json.Unmarshal(modeData, &entry) != nil || len(entry.Errors) > 0 || entry.Data.Repository.Object.File.Mode != 33188 || entry.Data.Repository.Object.File.Type != "blob" {
		return "", errors.New("suggestion requires an ordinary non-executable file; symlinks and other modes unsupported")
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", "repos/"+repo+"/contents/"+url.PathEscape(path)+"?ref="+url.QueryEscape(sha))
	if err != nil {
		return "", err
	}
	var raw struct {
		Type, Encoding, Content, Path string
		Size                          int
	}
	if json.Unmarshal(data, &raw) != nil || raw.Type != "file" || raw.Encoding != "base64" || raw.Path != path || raw.Size > 512<<10 {
		return "", errors.New("suggestion source unavailable or unsupported")
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(raw.Content, "\n", ""))
	if err != nil || len(b) != raw.Size {
		return "", errors.New("invalid suggestion source")
	}
	return string(b), nil
}
func (g *GH) PrepareSuggestion(ctx context.Context, m Metadata, c ReviewComment, before string) (SuggestionApplication, error) {
	m.Description = ""
	a := SuggestionApplication{Metadata: m, Target: c.Target, CommentID: c.ID, Before: before, CommentBody: c.Body}
	var err error
	a.Replacement, err = ParseSuggestion(c.Body)
	if err != nil {
		return a, err
	}
	if c.CurrentAnchor != nil && *c.CurrentAnchor != c.Target {
		return a, errors.New("stale suggestion anchor")
	}
	if c.Target.CommitID != m.HeadSHA || c.Target.Side != "RIGHT" || c.Target.SubjectType != "" {
		return a, errors.New("unsupported or stale suggestion anchor")
	}
	if err = g.verifySuggestionComment(ctx, a); err != nil {
		return a, err
	}
	a.Branch, err = g.suggestionBranch(ctx, m)
	if err != nil {
		return a, err
	}
	content, err := g.suggestionContent(ctx, m.HeadRepository, m.HeadSHA, c.Target.Path)
	if err != nil {
		return a, err
	}
	a.Content, err = ReplaceSuggestion(content, c.Target, before, a.Replacement)
	if err != nil {
		return a, err
	}
	a.Payload = suggestionPayload(a)
	return a, ValidateSuggestionApplication(a)
}
func (g *GH) ApplySuggestion(ctx context.Context, a SuggestionApplication) (SuggestionResult, error) {
	if err := ValidateSuggestionApplication(a); err != nil {
		return SuggestionResult{}, err
	}
	if err := g.verifySuggestionComment(ctx, a); err != nil {
		return SuggestionResult{}, err
	}
	branch, err := g.suggestionBranch(ctx, a.Metadata)
	if err != nil {
		return SuggestionResult{}, err
	}
	if branch != a.Branch {
		return SuggestionResult{}, errors.New("suggestion branch changed")
	}
	current, err := g.suggestionContent(ctx, a.Metadata.HeadRepository, a.Metadata.HeadSHA, a.Target.Path)
	if err != nil {
		return SuggestionResult{}, err
	}
	content, err := ReplaceSuggestion(current, a.Target, a.Before, a.Replacement)
	if err != nil || content != a.Content {
		return SuggestionResult{}, errors.New("suggestion prepared payload conflicts")
	}
	payload := a.Payload
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "graphql", "--input", "-")
	if err != nil {
		return SuggestionResult{}, errors.Join(ErrCommentDeliveryUnknown, errors.New("suggestion commit outcome uncertain"))
	}
	var raw struct {
		Errors []json.RawMessage
		Data   struct {
			CreateCommitOnBranch struct {
				Commit struct{ OID, URL string }
				Ref    struct{ Target struct{ OID string } }
			}
		}
	}
	if json.Unmarshal(data, &raw) != nil || len(raw.Errors) > 0 || !shaPattern.MatchString(raw.Data.CreateCommitOnBranch.Commit.OID) || raw.Data.CreateCommitOnBranch.Commit.OID != raw.Data.CreateCommitOnBranch.Ref.Target.OID {
		return SuggestionResult{}, errors.Join(ErrCommentDeliveryUnknown, errors.New("suggestion commit response unavailable"))
	}
	commit := raw.Data.CreateCommitOnBranch.Commit
	if commit.OID == a.Metadata.HeadSHA || commit.URL != "https://github.com/"+a.Metadata.HeadRepository+"/commit/"+commit.OID {
		return SuggestionResult{}, errors.Join(ErrCommentDeliveryUnknown, errors.New("invalid canonical suggestion commit"))
	}
	return SuggestionResult{commit.OID, commit.URL}, nil
}
func (g *GH) SuggestionOutcome(ctx context.Context, a SuggestionApplication) (bool, error) {
	if err := ValidateSuggestionApplication(a); err != nil {
		return false, err
	}
	m, err := g.Metadata(ctx, a.Metadata.Identity)
	if err != nil {
		return false, err
	}
	if m.HeadRepository != a.Metadata.HeadRepository {
		return false, errors.New("head repository changed; outcome unknown")
	}
	if m.HeadSHA == a.Metadata.HeadSHA {
		return false, errors.New("unchanged head cannot prove an uncertain commit was never applied; retain attempt or explicitly discard")
	}
	content, err := g.suggestionContent(ctx, m.HeadRepository, m.HeadSHA, a.Target.Path)
	if err != nil {
		return false, err
	}
	if content == a.Content {
		return true, nil
	}
	return false, errors.New("branch changed; suggestion outcome cannot be proven")
}

func suggestionPayload(a SuggestionApplication) []byte {
	input := map[string]any{"branch": map[string]string{"repositoryNameWithOwner": a.Metadata.HeadRepository, "refName": a.Branch}, "expectedHeadOid": a.Metadata.HeadSHA, "message": map[string]string{"headline": fmt.Sprintf("Apply review suggestion #%d", a.CommentID)}, "fileChanges": map[string]any{"additions": []map[string]string{{"path": a.Target.Path, "contents": base64.StdEncoding.EncodeToString([]byte(a.Content))}}}}
	payload, _ := json.Marshal(map[string]any{"query": `mutation($input:CreateCommitOnBranchInput!){createCommitOnBranch(input:$input){commit{oid url} ref{target{oid}}}}`, "variables": map[string]any{"input": input}})
	return payload
}

func (g *GH) verifySuggestionComment(ctx context.Context, a SuggestionApplication) error {
	if !validCommentAction(a.Metadata.Identity, a.CommentID) {
		return errors.New("invalid remote suggestion identity")
	}
	data, err := g.call(ctx, "api", "--hostname", "github.com", fmt.Sprintf("repos/%s/pulls/comments/%d", a.Metadata.Identity.Repository, a.CommentID))
	if err != nil {
		return err
	}
	c, err := parseReviewComment(data, a.Metadata.Identity)
	if err != nil || c.ID != a.CommentID || c.ParentID != 0 || c.CurrentAnchor == nil || c.Target != a.Target || *c.CurrentAnchor != a.Target || c.Body != a.CommentBody {
		return errors.New("suggestion changed, outdated, or unsupported; refresh discussions")
	}
	return nil
}

// UnmarshalJSON reconstructs the prepared result from the exact retained mutation,
// avoiding a second persisted copy of full-file source.
func (a *SuggestionApplication) UnmarshalJSON(data []byte) error {
	type applicationAlias SuggestionApplication
	var decoded applicationAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var payload struct {
		Variables struct {
			Input struct {
				FileChanges struct{ Additions []struct{ Contents string } }
			}
		}
	}
	if json.Unmarshal(decoded.Payload, &payload) != nil || len(payload.Variables.Input.FileChanges.Additions) != 1 {
		return errors.New("invalid retained suggestion mutation")
	}
	content, err := base64.StdEncoding.DecodeString(payload.Variables.Input.FileChanges.Additions[0].Contents)
	if err != nil {
		return errors.New("invalid retained suggestion contents")
	}
	decoded.Content = string(content)
	*a = SuggestionApplication(decoded)
	return nil
}
