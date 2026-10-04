package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Inbox is mutable triage evidence. It never changes a frozen review snapshot.
type InboxOptions struct {
	View, Repository, Author, Review, State, Draft, Requests, Activity string
}
type InboxItem struct {
	PullRequest                                           PullRequest
	UpdatedAt                                             time.Time
	HeadSHA, State, ReviewDecision                        string
	Draft, PersonalRequest, TeamRequest, RequestsComplete bool
	Teams                                                 []string
	Activity                                              string // unknown, read, changed (relative to explicit mark-read)
}
type Inbox struct {
	Viewer     string
	Items      []InboxItem
	ObservedAt time.Time
	Complete   bool
	Problems   []string
	Cached     bool
	Requests   int
}
type InboxReader interface {
	ReadInbox(context.Context, InboxOptions) (Inbox, error)
}

var loginPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,38}$`)

func (o InboxOptions) Validate() error {
	allowed := func(v string, values ...string) bool {
		for _, x := range values {
			if v == x {
				return true
			}
		}
		return false
	}
	if !allowed(o.View, "requested", "authored", "participated") || !allowed(o.State, "", "open", "closed", "all") || !allowed(o.Draft, "", "all", "yes", "no") || !allowed(o.Review, "", "all", "none", "required", "approved", "changes_requested") || !allowed(o.Requests, "", "all", "personal", "team") || !allowed(o.Activity, "", "all", "changed", "unknown", "read") {
		return errors.New("invalid inbox view or filter")
	}
	if o.Repository != "" && !repositoryPattern.MatchString(o.Repository) || o.Author != "" && !loginPattern.MatchString(o.Author) {
		return errors.New("invalid inbox repository or author")
	}
	return nil
}
func (o InboxOptions) query(kind string) string {
	q := "is:pr sort:updated-desc "
	switch o.View {
	case "authored":
		q += "author:@me"
	case "participated":
		if kind == "reviewed" {
			q += "reviewed-by:@me"
		} else {
			q += "involves:@me"
		}
	case "requested":
		if kind == "team" {
			q += "team-review-requested-user:@me"
		} else {
			q += "user-review-requested:@me"
		}
	}
	if o.View != "requested" {
		if o.Requests == "personal" {
			q += " user-review-requested:@me"
		} else if o.Requests == "team" {
			q += " team-review-requested-user:@me"
		}
	}
	if o.Repository != "" {
		q += " repo:" + o.Repository
	}
	if o.Author != "" {
		q += " author:" + o.Author
	}
	if o.State == "" || o.State == "open" {
		q += " is:open"
	} else if o.State == "closed" {
		q += " is:closed"
	}
	if o.Draft == "yes" {
		q += " draft:true"
	} else if o.Draft == "no" {
		q += " draft:false"
	}
	if o.Review != "" && o.Review != "all" {
		q += " review:" + o.Review
	}
	return q
}

const inboxQuery = `query($q:String!,$cursor:String){viewer{login} search(query:$q,type:ISSUE,first:100,after:$cursor){issueCount pageInfo{hasNextPage endCursor} nodes{... on PullRequest{number title createdAt updatedAt headRefOid state isDraft reviewDecision repository{nameWithOwner} author{login} viewerLatestReview{state} reviews(first:1){totalCount} reviewRequests(first:100){pageInfo{hasNextPage} nodes{requestedReviewer{__typename ... on User{login} ... on Team{slug organization{login}}}}}}}}}`

type inboxPageInfo struct {
	HasNextPage *bool
	EndCursor   string
}
type inboxReviewer struct {
	RequestedReviewer *struct {
		Typename     string `json:"__typename"`
		Login, Slug  string
		Organization struct{ Login string }
	}
}
type inboxNode struct {
	Number                   int
	Title, HeadRefOid, State string
	ReviewDecision           *string
	CreatedAt, UpdatedAt     time.Time
	IsDraft                  *bool
	Repository               struct{ NameWithOwner string }
	Author                   struct{ Login string }
	ViewerLatestReview       *struct{ State string }
	Reviews                  *struct{ TotalCount *int }
	ReviewRequests           *struct {
		PageInfo *inboxPageInfo
		Nodes    *[]inboxReviewer
	}
}

func (g *GH) ReadInbox(ctx context.Context, o InboxOptions) (Inbox, error) {
	result := Inbox{ObservedAt: time.Now().UTC(), Complete: true}
	if err := o.Validate(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	kinds := []string{"personal"}
	if o.View == "participated" {
		kinds = append(kinds, "reviewed")
	}
	if o.View == "requested" {
		switch o.Requests {
		case "team":
			kinds = []string{"team"}
		case "", "all":
			kinds = append(kinds, "team")
		}
	}
	defer func() {
		sort.SliceStable(result.Items, func(i, j int) bool { return result.Items[i].UpdatedAt.After(result.Items[j].UpdatedAt) })
	}()
	seen := map[Identity]int{}
	for _, kind := range kinds {
		cursor := ""
		cursors := map[string]bool{}
		scopeSeen := map[Identity]bool{}
		for page := 0; page < 10; page++ {
			args := []string{"api", "--hostname", "github.com", "graphql", "-f", "query=" + inboxQuery, "-f", "q=" + o.query(kind)}
			if cursor != "" {
				args = append(args, "-f", "cursor="+cursor)
			}
			result.Requests++
			data, err := g.call(ctx, args...)
			if err != nil {
				result.Complete = false
				result.Problems = append(result.Problems, "inbox search unavailable; authentication, connectivity or access may be limited")
				return result, nil
			}
			var raw struct {
				Data struct {
					Viewer struct{ Login string }
					Search *struct {
						IssueCount *int
						PageInfo   *inboxPageInfo
						Nodes      *[]inboxNode
					}
				}
				Errors []json.RawMessage
			}
			if json.Unmarshal(data, &raw) != nil || raw.Data.Search == nil || !loginPattern.MatchString(raw.Data.Viewer.Login) || len(raw.Errors) > 0 || raw.Data.Search.Nodes == nil || raw.Data.Search.PageInfo == nil || raw.Data.Search.PageInfo.HasNextPage == nil || raw.Data.Search.IssueCount == nil || *raw.Data.Search.IssueCount < 0 || len(*raw.Data.Search.Nodes) > 100 {
				result.Complete = false
				result.Problems = append(result.Problems, "invalid or partial GitHub search response")
				return result, nil
			}
			if result.Viewer != "" && result.Viewer != raw.Data.Viewer.Login {
				return Inbox{}, errors.New("inbox account changed during pagination")
			}
			result.Viewer = raw.Data.Viewer.Login
			for _, n := range *raw.Data.Search.Nodes {
				id, err := ParseIdentity(strconv.Itoa(n.Number), n.Repository.NameWithOwner)
				if err != nil || !validListText(n.Title) || !validListText(n.Author.Login) || n.CreatedAt.IsZero() || n.UpdatedAt.IsZero() || !shaPattern.MatchString(n.HeadRefOid) || (n.State != "OPEN" && n.State != "CLOSED" && n.State != "MERGED") || n.IsDraft == nil || (n.ReviewDecision != nil && *n.ReviewDecision != "APPROVED" && *n.ReviewDecision != "CHANGES_REQUESTED" && *n.ReviewDecision != "REVIEW_REQUIRED") {
					result.Complete = false
					result.Problems = append(result.Problems, "invalid search item omitted")
					continue
				}
				id.Repository = strings.ToLower(id.Repository)
				if scopeSeen[id] {
					result.Complete = false
					result.Problems = append(result.Problems, "search changed during pagination; duplicate item")
				}
				scopeSeen[id] = true
				item := InboxItem{PullRequest: PullRequest{Identity: id, Title: n.Title, Author: n.Author.Login, OpenedAt: n.CreatedAt, Checks: ChecksUnknown}, UpdatedAt: n.UpdatedAt, HeadSHA: n.HeadRefOid, State: n.State, Draft: *n.IsDraft, ReviewDecision: "UNKNOWN", Activity: "unknown"}
				if !inboxEvidenceMatches(o, result.Viewer, n) {
					result.Complete = false
					result.Problems = append(result.Problems, "search item did not satisfy authoritative filters; omitted")
					continue
				}
				if n.ReviewDecision != nil {
					item.ReviewDecision = *n.ReviewDecision
				}
				if n.ViewerLatestReview != nil {
					item.PullRequest.ViewerReview = n.ViewerLatestReview.State
				}
				var requests []inboxReviewer
				if n.ReviewRequests != nil && n.ReviewRequests.PageInfo != nil && n.ReviewRequests.PageInfo.HasNextPage != nil && n.ReviewRequests.Nodes != nil && len(*n.ReviewRequests.Nodes) <= 100 {
					requests = *n.ReviewRequests.Nodes
					item.RequestsComplete = !*n.ReviewRequests.PageInfo.HasNextPage
				}
				for _, r := range requests {
					v := r.RequestedReviewer
					if v == nil {
						item.RequestsComplete = false
						continue
					}
					if (v.Typename != "User" && v.Typename != "Team") || v.Typename == "User" && !loginPattern.MatchString(v.Login) || v.Typename == "Team" && !repositoryPattern.MatchString(v.Organization.Login+"/"+v.Slug) {
						item.RequestsComplete = false
						continue
					}
					if v.Typename == "User" && strings.EqualFold(v.Login, result.Viewer) {
						item.PersonalRequest = true
					}
					if v.Typename == "Team" && repositoryPattern.MatchString(v.Organization.Login+"/"+v.Slug) {
						item.Teams = append(item.Teams, v.Organization.Login+"/"+v.Slug)
					}
				}
				sort.Strings(item.Teams)
				if !item.RequestsComplete {
					result.Complete = false
					result.Problems = append(result.Problems, "review request details incomplete")
				}
				// Search membership is authoritative, including teams inaccessible to listing.
				if o.View == "requested" || o.Requests == "personal" || o.Requests == "team" {
					if kind == "team" || o.Requests == "team" {
						item.TeamRequest = true
					} else {
						item.PersonalRequest = true
					}
				}
				if index, ok := seen[id]; ok {
					result.Items[index].TeamRequest = result.Items[index].TeamRequest || item.TeamRequest
					result.Items[index].PersonalRequest = result.Items[index].PersonalRequest || item.PersonalRequest
				} else {
					seen[id] = len(result.Items)
					result.Items = append(result.Items, item)
				}
			}
			info := raw.Data.Search.PageInfo
			if !*info.HasNextPage {
				if len(scopeSeen) < *raw.Data.Search.IssueCount {
					result.Complete = false
					result.Problems = append(result.Problems, "search count exceeds returned evidence; incomplete")
				}
				if *raw.Data.Search.IssueCount > 1000 {
					result.Complete = false
					result.Problems = append(result.Problems, "GitHub search capped at 1000 matches per request scope")
				}
				break
			}
			if page == 9 || info.EndCursor == "" || cursors[info.EndCursor] || len(info.EndCursor) > 1024 {
				result.Complete = false
				result.Problems = append(result.Problems, fmt.Sprintf("search pagination incomplete after %d pages", page+1))
				break
			}
			cursor = info.EndCursor
			cursors[cursor] = true
		}
	}
	return result, nil
}

// Search qualifiers choose scope; returned repository, author, state and
// readiness decision still control local filter evidence.
func inboxEvidenceMatches(o InboxOptions, viewer string, n inboxNode) bool {
	if o.Repository != "" && !strings.EqualFold(o.Repository, n.Repository.NameWithOwner) {
		return false
	}
	if o.Author != "" && !strings.EqualFold(o.Author, n.Author.Login) {
		return false
	}
	if o.View == "authored" && !strings.EqualFold(viewer, n.Author.Login) {
		return false
	}
	if (o.State == "open" || o.State == "") && n.State != "OPEN" || o.State == "closed" && n.State == "OPEN" {
		return false
	}
	if o.Draft == "yes" && !*n.IsDraft || o.Draft == "no" && *n.IsDraft {
		return false
	}
	switch o.Review {
	case "approved":
		return n.ReviewDecision != nil && *n.ReviewDecision == "APPROVED"
	case "changes_requested":
		return n.ReviewDecision != nil && *n.ReviewDecision == "CHANGES_REQUESTED"
	case "required":
		return n.ReviewDecision != nil && *n.ReviewDecision == "REVIEW_REQUIRED"
	case "none":
		return n.Reviews != nil && n.Reviews.TotalCount != nil && *n.Reviews.TotalCount == 0
	}
	return true
}
