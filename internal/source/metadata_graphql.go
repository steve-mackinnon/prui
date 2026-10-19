package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Query only canonical PR metadata; each invocation remains a fresh revision
// read. Ref OIDs survive deleted refs, while missing fork repositories fail closed.
const captureMetadataQuery = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){pullRequest(number:$number){number body baseRefName baseRefOid headRefOid baseRepository{nameWithOwner} headRepository{nameWithOwner}}}}`

func (g *GH) graphqlMetadata(ctx context.Context, id Identity) (Metadata, error) {
	parts := strings.SplitN(id.Repository, "/", 2)
	type variables struct {
		Owner  string `json:"owner"`
		Repo   string `json:"repo"`
		Number int    `json:"number"`
	}
	payload, err := json.Marshal(struct {
		Query     string    `json:"query"`
		Variables variables `json:"variables"`
	}{captureMetadataQuery, variables{parts[0], parts[1], id.Number}})
	if err != nil {
		return Metadata{}, ErrCommand
	}
	legacyUsed := false
	data, err := g.readCapture(ctx, http.MethodPost, "graphql", payload, func(c context.Context) ([]byte, error) {
		legacyUsed = true
		return g.call(c, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("repos/%s/pulls/%d", id.Repository, id.Number))
	})
	if err != nil {
		return Metadata{}, fmt.Errorf("GitHub metadata unavailable (check authentication and connectivity): %w", err)
	}
	if legacyUsed {
		return decodeRESTMetadata(id, data)
	}
	type repository struct{ NameWithOwner string }
	var response struct {
		Errors []json.RawMessage
		Data   struct {
			Repository *struct {
				PullRequest *struct {
					Number                         int
					Body, BaseRefName              *string
					BaseRefOid, HeadRefOid         string
					BaseRepository, HeadRepository *repository
				}
			}
		}
	}
	if !utf8.Valid(data) || json.Unmarshal(data, &response) != nil || len(response.Errors) > 0 || response.Data.Repository == nil || response.Data.Repository.PullRequest == nil {
		return Metadata{}, errors.New("invalid metadata or deleted fork")
	}
	pr := response.Data.Repository.PullRequest
	if pr.Number != id.Number || pr.Body == nil || pr.BaseRefName == nil || pr.BaseRepository == nil || pr.HeadRepository == nil {
		return Metadata{}, errors.New("invalid metadata or deleted fork")
	}
	return validateMetadata(Metadata{Identity: id, TargetBranch: *pr.BaseRefName, Description: *pr.Body, BaseSHA: pr.BaseRefOid, HeadSHA: pr.HeadRefOid, BaseRepository: pr.BaseRepository.NameWithOwner, HeadRepository: pr.HeadRepository.NameWithOwner})
}
