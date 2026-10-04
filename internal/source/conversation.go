package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"
)

// Conversation events are live overlay data, never comparison or draft storage.
type ConversationEvent struct {
	ID        string
	Kind      string
	Decision  string
	Author    string
	Body      string
	URL       string
	CreatedAt time.Time
}
type ConversationSnapshot struct {
	Events   []ConversationEvent
	Complete bool
	Reason   string
}
type ConversationReader interface {
	ListConversation(context.Context, Identity) (ConversationSnapshot, error)
}
type GeneralCommentWriter interface {
	CreateGeneralComment(context.Context, Identity, string) (ConversationEvent, error)
}
type remoteConversationEvent struct {
	ID                   int64
	Body, State, HTMLURL string
	CreatedAt            time.Time `json:"created_at"`
	SubmittedAt          time.Time `json:"submitted_at"`
	URL                  string    `json:"html_url"`
	User                 *struct{ Login string }
}

func normalizeConversation(raw remoteConversationEvent, kind string) (ConversationEvent, error) {
	t := raw.CreatedAt
	if kind == "Review" {
		t = raw.SubmittedAt
	}
	author := "[deleted]"
	if raw.User != nil {
		author = raw.User.Login
	}
	if raw.ID <= 0 || t.IsZero() || author == "" || !validDiscussionField(author, 256, false) || !validDiscussionField(raw.Body, 65536, true) {
		return ConversationEvent{}, errors.New("invalid conversation event")
	}
	if kind == "Review" {
		switch raw.State {
		case "APPROVED", "CHANGES_REQUESTED", "COMMENTED", "DISMISSED":
		default:
			return ConversationEvent{}, errors.New("invalid submitted review decision")
		}
	}
	return ConversationEvent{ID: kind + ":" + strconv.FormatInt(raw.ID, 10), Kind: kind, Decision: raw.State, Author: author, Body: raw.Body, URL: discussionURL(raw.URL), CreatedAt: t}, nil
}

// ListConversation bounds each REST connection to 500 events and the combined
// response to 4 MiB. A failed or truncated connection never means empty success.
func (g *GH) ListConversation(ctx context.Context, id Identity) (ConversationSnapshot, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return ConversationSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := ConversationSnapshot{Events: []ConversationEvent{}, Complete: true}
	used := 0
	seen := map[string]bool{}
	for _, connection := range []struct{ kind, path string }{{"PR comment", fmt.Sprintf("repos/%s/issues/%d/comments", id.Repository, id.Number)}, {"Review", fmt.Sprintf("repos/%s/pulls/%d/reviews", id.Repository, id.Number)}} {
		for page := 1; page <= 5; page++ {
			data, err := g.call(ctx, "api", "--hostname", "github.com", fmt.Sprintf("%s?per_page=100&page=%d", connection.path, page))
			used += len(data)
			var rows []remoteConversationEvent
			if err != nil || used > 4<<20 || !utf8.Valid(data) || json.Unmarshal(data, &rows) != nil || rows == nil || len(rows) > 100 {
				out.Complete = false
				out.Reason = "Conversation unavailable or incomplete"
				break
			}
			for _, raw := range rows {
				// Pending reviews have no submitted timestamp and are not published activity.
				if connection.kind == "Review" && raw.State == "PENDING" {
					continue
				}
				event, e := normalizeConversation(raw, connection.kind)
				if e != nil || seen[event.ID] {
					out.Complete = false
					out.Reason = "Invalid or duplicate conversation event omitted"
					continue
				}
				seen[event.ID] = true
				out.Events = append(out.Events, event)
			}
			if len(rows) < 100 {
				break
			}
			if page == 5 {
				out.Complete = false
				out.Reason = "Conversation event limit reached"
			}
		}
	}
	sort.Slice(out.Events, func(i, j int) bool {
		a, b := out.Events[i], out.Events[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	if len(out.Events) == 0 && !out.Complete {
		return out, errors.New(out.Reason)
	}
	return out, nil
}
func (g *GH) CreateGeneralComment(ctx context.Context, id Identity, body string) (ConversationEvent, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return ConversationEvent{}, err
	}
	if body == "" || !validDiscussionField(body, 65536, true) {
		return ConversationEvent{}, errors.New("invalid PR comment")
	}
	payload, _ := json.Marshal(map[string]string{"body": body})
	data, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "--method", "POST", "--input", "-", fmt.Sprintf("repos/%s/issues/%d/comments", id.Repository, id.Number))
	if err != nil {
		return ConversationEvent{}, safeReviewCommentError(err)
	}
	var raw remoteConversationEvent
	if !utf8.Valid(data) || json.Unmarshal(data, &raw) != nil {
		return ConversationEvent{}, errors.New("invalid PR comment response")
	}
	return normalizeConversation(raw, "PR comment")
}
