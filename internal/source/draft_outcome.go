package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// DraftOutcomeReader reconciles an attempted write through bounded read-only
// requests. A truncated or malformed history never proves absence.
type DraftOutcomeReader interface {
	ReviewDraftOutcome(context.Context, PullRequestReview) (bool, error)
	CommentDraftOutcome(context.Context, ReviewComment, int64) (bool, error)
}

func (g *GH) draftHistory(ctx context.Context, endpoint string) ([]json.RawMessage, error) {
	var records []json.RawMessage
	for page := 1; page <= 10; page++ {
		data, err := g.call(ctx, "api", "--hostname", "github.com", "--method", "GET", fmt.Sprintf("%s?per_page=100&page=%d", endpoint, page))
		if err != nil {
			return nil, safeReviewCommentError(err)
		}
		var items []json.RawMessage
		if !utf8.Valid(data) || json.Unmarshal(data, &items) != nil || items == nil || len(items) > 100 {
			return nil, errors.New("invalid draft outcome history")
		}
		records = append(records, items...)
		if len(items) < 100 {
			return records, nil
		}
	}
	return nil, errors.New("draft outcome history incomplete; check GitHub manually")
}
func (g *GH) CommentDraftOutcome(ctx context.Context, want ReviewComment, parentID int64) (bool, error) {
	if err := validateReviewComment(want); err != nil {
		return false, err
	}
	viewer, err := g.Viewer(ctx)
	if err != nil {
		return false, err
	}
	items, err := g.draftHistory(ctx, fmt.Sprintf("repos/%s/pulls/%d/comments", want.Target.Identity.Repository, want.Target.Identity.Number))
	if err != nil {
		return false, err
	}
	found := false
	for _, item := range items {
		c, _, err := parseRemoteReviewComment(item, want.Target.Identity)
		if err != nil {
			return false, err
		}
		anchorMatches := c.Target == want.Target || c.OriginalAnchor != nil && *c.OriginalAnchor == want.Target
		if strings.EqualFold(c.Author, viewer.Login) && c.Body == want.Body && c.ParentID == parentID && anchorMatches {
			found = true
		}
	}
	return found, nil
}
func (g *GH) ReviewDraftOutcome(ctx context.Context, want PullRequestReview) (bool, error) {
	if err := ValidatePullRequestReview(want); err != nil {
		return false, err
	}
	viewer, err := g.Viewer(ctx)
	if err != nil {
		return false, err
	}
	endpoint := fmt.Sprintf("repos/%s/pulls/%d/reviews", want.Identity.Repository, want.Identity.Number)
	items, err := g.draftHistory(ctx, endpoint)
	if err != nil {
		return false, err
	}
	event := map[string]string{"COMMENT": "COMMENTED", "APPROVE": "APPROVED", "REQUEST_CHANGES": "CHANGES_REQUESTED"}[want.Event]
	for _, item := range items {
		var r struct {
			ID          int64
			Body, State *string
			CommitID    string `json:"commit_id"`
			User        *struct{ Login string }
		}
		if json.Unmarshal(item, &r) != nil || r.ID <= 0 || r.User == nil || r.User.Login == "" || r.Body == nil || r.State == nil || !validDiscussionField(*r.Body, 65536, true) || !validDiscussionField(r.User.Login, 256, false) || !validReviewOutcomeState(*r.State) || !shaPattern.MatchString(r.CommitID) {
			return false, errors.New("invalid review outcome history")
		}
		if !strings.EqualFold(r.User.Login, viewer.Login) || *r.Body != want.Body || *r.State != event || r.CommitID != want.CommitID {
			continue
		}
		comments, err := g.draftHistory(ctx, endpoint+"/"+strconv.FormatInt(r.ID, 10)+"/comments")
		if err != nil {
			return false, err
		}
		if len(comments) != len(want.Comments) {
			continue
		}
		unmatched := append([]ReviewComment(nil), want.Comments...)
		for _, raw := range comments {
			c, _, err := parseRemoteReviewComment(raw, want.Identity)
			if err != nil {
				return false, err
			}
			for i, w := range unmatched {
				if c.Body == w.Body && (c.Target == w.Target || c.OriginalAnchor != nil && *c.OriginalAnchor == w.Target) {
					unmatched = append(unmatched[:i], unmatched[i+1:]...)
					break
				}
			}
		}
		if len(unmatched) == 0 {
			return true, nil
		}
	}
	return false, nil
}

func validReviewOutcomeState(state string) bool {
	switch state {
	case "COMMENTED", "APPROVED", "CHANGES_REQUESTED", "DISMISSED", "PENDING":
		return true
	}
	return false
}
