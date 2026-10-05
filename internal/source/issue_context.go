package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// IssueContext is mutable provider context. Body is only a candidate-link input
// and is never persisted or supplied to AI analysis by this feature.
type IssueContext struct {
	Identity                    Identity
	HeadSHA                     string
	CapturedAt                  time.Time
	Complete                    bool
	Reason                      string
	ReviewDecision              string // authoritative aggregate; empty means unknown
	Requested                   []ContextReviewer
	Reviews                     []ContextReview
	Labels                      []string
	Issues                      []ContextIssue
	Linear                      []ContextIssue
	LinearWorkspace, LinearAuth string
	LinearReason                string
	Body                        string `json:"-"`
}
type ContextReviewer struct{ Kind, Name string }
type ContextReview struct{ Author, Decision string }
type ContextIssue struct{ Identifier, Title, Status, Description, URL string }
type IssueContextReader interface {
	ReadIssueContext(context.Context, Identity) (IssueContext, error)
}

const issueContextQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){headRefOid body reviewDecision reviewRequests(first:100){nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug organization{login}}}} pageInfo{hasNextPage}} latestOpinionatedReviews(first:100){nodes{author{login} state} pageInfo{hasNextPage}} labels(first:100){nodes{name} pageInfo{hasNextPage}} closingIssuesReferences(first:100){nodes{number title state url repository{nameWithOwner}} pageInfo{hasNextPage}}}}}`

type contextConnection struct {
	Nodes    []json.RawMessage
	PageInfo *struct{ HasNextPage *bool }
}

func (g *GH) ReadIssueContext(ctx context.Context, id Identity) (IssueContext, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return IssueContext{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	parts := strings.Split(id.Repository, "/")
	payload, _ := json.Marshal(map[string]any{"query": issueContextQuery, "variables": map[string]any{"owner": parts[0], "name": parts[1], "number": id.Number}})
	b, err := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "graphql", "--input", "-")
	if err != nil {
		return IssueContext{}, safeReviewCommentError(err)
	}
	var response struct {
		Errors []json.RawMessage
		Data   struct {
			Repository *struct {
				PullRequest *struct {
					HeadRefOID, Body                                                          string
					ReviewDecision                                                            *string
					ReviewRequests, LatestOpinionatedReviews, Labels, ClosingIssuesReferences *contextConnection
				}
			}
		}
	}
	if len(b) > 1<<20 || !utf8.Valid(b) || json.Unmarshal(b, &response) != nil || response.Data.Repository == nil || response.Data.Repository.PullRequest == nil {
		return IssueContext{}, errors.New("invalid GitHub issue context")
	}
	p := response.Data.Repository.PullRequest
	if !shaPattern.MatchString(p.HeadRefOID) || !validDiscussionField(p.Body, 1<<20, true) {
		return IssueContext{}, errors.New("invalid GitHub issue context")
	}
	out := IssueContext{Identity: id, HeadSHA: p.HeadRefOID, CapturedAt: time.Now().UTC(), Complete: true, Body: p.Body}
	partial := func() {
		out.Complete = false
		out.Reason = "GitHub context partial: missing, invalid, or additional records"
	}
	if len(response.Errors) > 0 {
		partial()
	}
	if p.ReviewDecision != nil {
		switch *p.ReviewDecision {
		case "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED":
			out.ReviewDecision = *p.ReviewDecision
		default:
			partial()
		}
	} else {
		partial()
	}
	read := func(c *contextConnection, parse func(json.RawMessage) bool) {
		if c == nil || c.PageInfo == nil || c.PageInfo.HasNextPage == nil || c.Nodes == nil || len(c.Nodes) > 100 {
			partial()
			return
		}
		if *c.PageInfo.HasNextPage {
			partial()
		}
		for _, n := range c.Nodes {
			if !parse(n) {
				partial()
			}
		}
	}
	read(p.ReviewRequests, func(n json.RawMessage) bool {
		var r struct {
			RequestedReviewer *struct {
				Type         string `json:"__typename"`
				Login, Slug  string
				Organization *struct{ Login string }
			}
		}
		if json.Unmarshal(n, &r) != nil || r.RequestedReviewer == nil {
			return false
		}
		v := r.RequestedReviewer
		name := v.Login
		if v.Type == "Team" && v.Organization != nil {
			name = v.Organization.Login + "/" + v.Slug
			if v.Organization.Login == "" || v.Slug == "" {
				return false
			}
		} else if v.Type != "User" {
			return false
		}
		if name == "" || !validDiscussionField(name, 512, false) {
			return false
		}
		out.Requested = append(out.Requested, ContextReviewer{Kind: v.Type, Name: name})
		return true
	})
	read(p.LatestOpinionatedReviews, func(n json.RawMessage) bool {
		var r struct {
			Author *struct{ Login string }
			State  string
		}
		if json.Unmarshal(n, &r) != nil || r.Author == nil || r.Author.Login == "" || !validDiscussionField(r.Author.Login, 256, false) {
			return false
		}
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
		default:
			return false
		}
		out.Reviews = append(out.Reviews, ContextReview{Author: r.Author.Login, Decision: r.State})
		return true
	})
	read(p.Labels, func(n json.RawMessage) bool {
		var r struct{ Name string }
		if json.Unmarshal(n, &r) != nil || r.Name == "" || !validDiscussionField(r.Name, 256, false) {
			return false
		}
		out.Labels = append(out.Labels, r.Name)
		return true
	})
	read(p.ClosingIssuesReferences, func(n json.RawMessage) bool {
		var r struct {
			Number            int
			Title, State, URL string
			Repository        *struct{ NameWithOwner string }
		}
		if json.Unmarshal(n, &r) != nil || r.Repository == nil || !repositoryPattern.MatchString(r.Repository.NameWithOwner) || r.Number <= 0 || r.Title == "" || !validDiscussionField(r.Title, 4096, true) || (r.State != "OPEN" && r.State != "CLOSED") {
			return false
		}
		expected := fmt.Sprintf("https://github.com/%s/issues/%d", r.Repository.NameWithOwner, r.Number)
		if r.URL != expected {
			return false
		}
		out.Issues = append(out.Issues, ContextIssue{Identifier: fmt.Sprintf("%s#%d", r.Repository.NameWithOwner, r.Number), Title: r.Title, Status: r.State, URL: expected})
		return true
	})
	return out, nil
}

// ValidateIssueContext checks the cache boundary independently of API parsing.
func ValidateIssueContext(c IssueContext) bool {
	if _, err := ParseIdentity(strconv.Itoa(c.Identity.Number), c.Identity.Repository); err != nil {
		return false
	}
	if !shaPattern.MatchString(c.HeadSHA) || c.CapturedAt.IsZero() || c.CapturedAt.UnixNano() <= 0 || len(c.Requested) > 100 || len(c.Reviews) > 100 || len(c.Labels) > 100 || len(c.Issues) > 100 || len(c.Linear) > 10 || !validDiscussionField(c.Reason, 4096, true) || !validDiscussionField(c.LinearReason, 4096, true) || c.Body != "" {
		return false
	}
	if c.Complete && c.Reason != "" || !c.Complete && c.Reason == "" {
		return false
	}
	if !validDiscussionField(c.LinearWorkspace, 100, false) || (c.LinearWorkspace == "") != (c.LinearAuth == "") {
		return false
	}
	switch c.LinearAuth {
	case "", "api_key", "oauth":
	default:
		return false
	}
	switch c.ReviewDecision {
	case "", "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED":
	default:
		return false
	}
	for _, r := range c.Requested {
		if (r.Kind != "User" && r.Kind != "Team") || r.Name == "" || !validDiscussionField(r.Name, 512, false) {
			return false
		}
	}
	for _, r := range c.Reviews {
		if r.Author == "" || !validDiscussionField(r.Author, 256, false) {
			return false
		}
		switch r.Decision {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
		default:
			return false
		}
	}
	for _, l := range c.Labels {
		if l == "" || !validDiscussionField(l, 256, false) {
			return false
		}
	}
	for _, group := range [][]ContextIssue{c.Issues, c.Linear} {
		for _, v := range group {
			if v.Identifier == "" || v.Title == "" || v.Status == "" || !validDiscussionField(v.Identifier, 512, false) || !validDiscussionField(v.Title, 4096, true) || !validDiscussionField(v.Status, 256, false) || !validDiscussionField(v.Description, 65536, true) || !validContextURL(v.URL) {
				return false
			}
		}
	}
	for _, v := range c.Issues {
		parts := strings.Split(v.Identifier, "#")
		if len(parts) != 2 || !repositoryPattern.MatchString(parts[0]) {
			return false
		}
		n, e := strconv.Atoi(parts[1])
		if e != nil || n <= 0 || strconv.Itoa(n) != parts[1] || v.URL != fmt.Sprintf("https://github.com/%s/issues/%d", parts[0], n) {
			return false
		}
	}
	for _, v := range c.Linear {
		u, _ := url.Parse(v.URL)
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
		if u.Host != "linear.app" || len(parts) < 3 || len(parts) > 4 || parts[1] != "issue" || parts[2] != v.Identifier || c.LinearWorkspace != "" && parts[0] != c.LinearWorkspace {
			return false
		}
	}
	return true
}
func validContextURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && len(s) <= 4096 && validDiscussionField(s, 4096, false) && u.Scheme == "https" && (u.Host == "github.com" || u.Host == "linear.app") && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.RawPath == ""
}
