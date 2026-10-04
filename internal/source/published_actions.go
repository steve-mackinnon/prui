package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// PublishedComment contains canonical mutable fields without claiming any new
// code-anchor provenance. Callers retain the existing immutable anchors.
type PublishedComment struct {
	ID           int64
	Body, Author string
}
type PublishedCommentEditor interface {
	EditPublishedComment(context.Context, Identity, int64, bool, string) (PublishedComment, error)
}
type ThreadResolver interface {
	SetThreadResolved(context.Context, Identity, string, bool) (Discussion, error)
}

func unknownPublishedWrite(err error) error {
	return fmt.Errorf("%w: %w", ErrCommentDeliveryUnknown, safeReviewCommentError(err))
}
func (g *GH) EditPublishedComment(ctx context.Context, id Identity, cid int64, general bool, body string) (PublishedComment, error) {
	if !validCommentAction(id, cid) {
		return PublishedComment{}, errors.New("invalid published comment identity")
	}
	if err := ValidateGeneralComment(body); err != nil {
		return PublishedComment{}, err
	}
	path := "pulls/comments"
	if general {
		path = "issues/comments"
	}
	payload, _ := json.Marshal(map[string]string{"body": body})
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "PATCH", "--input", "-", fmt.Sprintf("repos/%s/%s/%d", id.Repository, path, cid))
	if err != nil {
		return PublishedComment{}, unknownPublishedWrite(err)
	}
	var raw remoteConversationEvent
	if len(data) > 1<<20 || !utf8.Valid(data) || json.Unmarshal(data, &raw) != nil || raw.ID != cid || raw.Body != body || raw.User == nil || raw.User.Login == "" || !validDiscussionField(raw.User.Login, 256, false) {
		return PublishedComment{}, unknownPublishedWrite(errors.New("invalid edited comment response"))
	}
	return PublishedComment{ID: raw.ID, Body: raw.Body, Author: raw.User.Login}, nil
}
func (g *GH) SetThreadResolved(ctx context.Context, id Identity, tid string, resolved bool) (Discussion, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return Discussion{}, err
	}
	if tid == "" || !validDiscussionField(tid, 256, false) {
		return Discussion{}, errors.New("invalid thread identity")
	}
	name := "resolveReviewThread"
	if !resolved {
		name = "unresolveReviewThread"
	}
	query := fmt.Sprintf(`mutation($id:ID!){action:%s(input:{threadId:$id}){thread{id isResolved viewerCanResolve viewerCanUnresolve}}}`, name)
	payload, _ := json.Marshal(map[string]any{"query": query, "variables": map[string]string{"id": tid}})
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "graphql", "--input", "-")
	if err != nil {
		return Discussion{}, unknownPublishedWrite(err)
	}
	var raw struct {
		Errors []json.RawMessage
		Data   struct {
			Action *struct {
				Thread *struct {
					ID                                               string
					IsResolved, ViewerCanResolve, ViewerCanUnresolve *bool
				}
			}
		}
	}
	if len(data) > 1<<20 || !utf8.Valid(data) || json.Unmarshal(data, &raw) != nil || len(raw.Errors) > 0 || raw.Data.Action == nil || raw.Data.Action.Thread == nil {
		return Discussion{}, unknownPublishedWrite(errors.New("invalid thread mutation response"))
	}
	t := raw.Data.Action.Thread
	if t.ID != tid || t.IsResolved == nil || *t.IsResolved != resolved || t.ViewerCanResolve == nil || t.ViewerCanUnresolve == nil {
		return Discussion{}, unknownPublishedWrite(errors.New("invalid canonical thread"))
	}
	return Discussion{ID: t.ID, Resolved: t.IsResolved, CanResolve: t.ViewerCanResolve, CanUnresolve: t.ViewerCanUnresolve}, nil
}
