package source

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Discussion is readable even when neither coordinate can be placed in a diff.
// Unknown status is represented by nil, independently for each status.
type Discussion struct {
	Retained         bool
	Kind             string
	Decision         string
	CreatedAt        time.Time
	ID               string
	OriginalCommitID string
	OriginalAnchor   *ReviewCommentTarget
	CurrentAnchor    *ReviewCommentTarget
	Outdated         *bool
	Resolved         *bool
	Comments         []ReviewComment
	DiffHunk         string
	URL              string
}
type DiscussionSnapshot struct {
	Events   []ConversationEvent
	Timeline bool
	Threads  []Discussion
	Complete bool
	Reason   string
}
type DiscussionReader interface {
	ListDiscussions(context.Context, Identity) (DiscussionSnapshot, error)
}

type discussionPageInfo struct {
	HasNextPage *bool
	EndCursor   string
}

func (p *discussionPageInfo) more() bool { return p != nil && p.HasNextPage != nil && *p.HasNextPage }

type discussionComment struct {
	FullDatabaseID         json.RawMessage `json:"fullDatabaseId"`
	Body, URL, DiffHunk    string
	CreatedAt              time.Time
	Author                 *struct{ Login string }
	Commit, OriginalCommit *struct{ OID string }
	ReplyTo                *struct{ FullDatabaseID json.RawMessage }
}
type discussionComments struct {
	Nodes    []*discussionComment
	PageInfo *discussionPageInfo
}
type remoteDiscussion struct {
	ID, Path, DiffSide, SubjectType                  string
	Line, OriginalLine, StartLine, OriginalStartLine *int
	IsOutdated, IsResolved                           *bool
	Comments                                         discussionComments
}

const discussionCommentFields = `fullDatabaseId body url diffHunk createdAt author{login} commit{oid} originalCommit{oid} replyTo{fullDatabaseId}`
const discussionFields = `id path diffSide subjectType line originalLine startLine originalStartLine isOutdated isResolved comments(first:20){nodes{` + discussionCommentFields + `} pageInfo{hasNextPage endCursor}}`
const discussionsQuery = `query($owner:String!,$name:String!,$number:Int!,$cursor:String){repository(owner:$owner,name:$name){pullRequest(number:$number){reviewThreads(first:100,after:$cursor){nodes{` + discussionFields + `} pageInfo{hasNextPage endCursor}}}}}`
const discussionRepliesQuery = `query($id:ID!,$cursor:String){node(id:$id){... on PullRequestReviewThread{comments(first:20,after:$cursor){nodes{` + discussionCommentFields + `} pageInfo{hasNextPage endCursor}}}}}`

func validDiscussionField(s string, max int, controls bool) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	if !controls {
		for _, r := range s {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}
func discussionURL(s string) string {
	u, e := url.Parse(s)
	if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || !validDiscussionField(s, 8192, false) {
		return ""
	}
	return s
}
func discussionID(raw json.RawMessage) (int64, error) {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		s = string(raw)
	}
	id, e := strconv.ParseInt(s, 10, 64)
	if e != nil || id <= 0 {
		return 0, errors.New("invalid discussion comment identity")
	}
	return id, nil
}
func discussionAnchor(id Identity, sha, path, side string, line, start *int, subject string) *ReviewCommentTarget {
	if !shaPattern.MatchString(sha) || !validDiscussionField(path, 4096, false) || path == "" || (side != "LEFT" && side != "RIGHT") || line == nil || *line <= 0 || start != nil || subject != "LINE" {
		return nil
	}
	return &ReviewCommentTarget{Identity: id, CommitID: sha, Path: path, Side: side, Line: *line}
}
func normalizeDiscussion(raw *remoteDiscussion, id Identity) (Discussion, time.Time, error) {
	if raw == nil || raw.ID == "" || !validDiscussionField(raw.ID, 256, false) || !validDiscussionField(raw.Path, 4096, false) || raw.Path == "" || len(raw.Comments.Nodes) == 0 || raw.Comments.PageInfo == nil || raw.Comments.PageInfo.HasNextPage == nil || (raw.DiffSide != "LEFT" && raw.DiffSide != "RIGHT") || (raw.SubjectType != "LINE" && raw.SubjectType != "FILE") {
		return Discussion{}, time.Time{}, errors.New("invalid discussion")
	}
	for _, line := range []*int{raw.Line, raw.OriginalLine, raw.StartLine, raw.OriginalStartLine} {
		if line != nil && *line <= 0 {
			return Discussion{}, time.Time{}, errors.New("invalid discussion line")
		}
	}
	d := Discussion{ID: raw.ID, Outdated: raw.IsOutdated, Resolved: raw.IsResolved}
	seen := map[int64]bool{}
	var created time.Time
	for i, c := range raw.Comments.Nodes {
		if c == nil || !validDiscussionField(c.Body, 65536, true) || !validDiscussionField(c.DiffHunk, 65536, true) {
			return Discussion{}, created, errors.New("invalid discussion comment")
		}
		cid, e := discussionID(c.FullDatabaseID)
		if e != nil || seen[cid] {
			return Discussion{}, created, errors.New("invalid discussion comment")
		}
		seen[cid] = true
		author := "[deleted]"
		if c.Author != nil {
			author = c.Author.Login
			if author == "" || !validDiscussionField(author, 256, false) {
				return Discussion{}, created, errors.New("invalid discussion author")
			}
		}
		parent := int64(0)
		if c.ReplyTo != nil {
			parent, e = discussionID(c.ReplyTo.FullDatabaseID)
			if e != nil || parent == cid {
				return Discussion{}, created, errors.New("invalid discussion parent")
			}
		}
		sha, original := "", ""
		if c.Commit != nil {
			sha = c.Commit.OID
			if !shaPattern.MatchString(sha) {
				return Discussion{}, created, errors.New("invalid discussion commit")
			}
		}
		if c.OriginalCommit != nil {
			original = c.OriginalCommit.OID
			if !shaPattern.MatchString(original) {
				return Discussion{}, created, errors.New("invalid original commit")
			}
		}
		comment := ReviewComment{CreatedAt: c.CreatedAt, ID: cid, ParentID: parent, Author: author, Body: c.Body, URL: discussionURL(c.URL), DiffHunk: c.DiffHunk}
		if i == 0 && parent == 0 {
			created = c.CreatedAt
			d.OriginalCommitID = original
			d.OriginalAnchor = discussionAnchor(id, original, raw.Path, raw.DiffSide, raw.OriginalLine, raw.OriginalStartLine, raw.SubjectType)
			d.CurrentAnchor = discussionAnchor(id, sha, raw.Path, raw.DiffSide, raw.Line, raw.StartLine, raw.SubjectType)
			d.URL = comment.URL
			d.DiffHunk = c.DiffHunk
		}
		comment.OriginalAnchor = d.OriginalAnchor
		if d.CurrentAnchor != nil {
			comment.Target = *d.CurrentAnchor
		}
		d.Comments = append(d.Comments, comment)
	}
	return d, created, nil
}

// ListDiscussions explicitly paginates both connections under a single budget.
// Partial snapshots are usable but never imply that absent threads do not exist.
func (g *GH) ListDiscussions(ctx context.Context, id Identity) (DiscussionSnapshot, error) {
	if _, e := ParseIdentity(strconv.Itoa(id.Number), id.Repository); e != nil {
		return DiscussionSnapshot{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := DiscussionSnapshot{Complete: true}
	used, total := 0, 0
	request := func(query string, vars map[string]any) ([]byte, error) {
		payload, _ := json.Marshal(map[string]any{"query": query, "variables": vars})
		data, e := g.callWithStdin(ctx, payload, "api", "--hostname", "github.com", "graphql", "--input", "-")
		used += len(data)
		if e != nil {
			return nil, safeReviewCommentError(e)
		}
		if used > 4<<20 {
			return nil, ErrLimit
		}
		if !utf8.Valid(data) {
			return nil, errors.New("invalid discussions response")
		}
		return data, nil
	}
	times := map[string]time.Time{}
	finalize := func() {
		sort.SliceStable(out.Threads, func(i, j int) bool {
			a, b := out.Threads[i].ID, out.Threads[j].ID
			if times[a].Equal(times[b]) {
				return a < b
			}
			return times[a].Before(times[b])
		})
	}
	partial := func(reason string, e error) (DiscussionSnapshot, error) {
		out.Complete = false
		out.Reason = reason
		finalize()
		if len(out.Threads) == 0 {
			return out, e
		}
		return out, nil
	}
	parts := strings.Split(id.Repository, "/")
	cursor := ""
	seen := map[string]bool{}
	canonicalComments := map[int64]bool{}
	for page := 0; page < 5; page++ {
		data, e := request(discussionsQuery, map[string]any{"owner": parts[0], "name": parts[1], "number": id.Number, "cursor": nullableCursor(cursor)})
		if e != nil {
			return partial("Discussions unavailable or incomplete", e)
		}
		var response struct {
			Errors []json.RawMessage
			Data   struct {
				Repository *struct {
					PullRequest *struct {
						ReviewThreads *struct {
							Nodes    []*remoteDiscussion
							PageInfo *discussionPageInfo
						}
					}
				}
			}
		}
		if json.Unmarshal(data, &response) != nil || response.Data.Repository == nil || response.Data.Repository.PullRequest == nil || response.Data.Repository.PullRequest.ReviewThreads == nil {
			return partial("Invalid discussions response", errors.New("invalid discussions response"))
		}
		connection := response.Data.Repository.PullRequest.ReviewThreads
		if connection.PageInfo == nil || connection.PageInfo.HasNextPage == nil {
			return partial("Invalid discussion pagination", errors.New("invalid discussion pagination"))
		}
		if len(connection.Nodes) > 100 {
			return partial("Discussion thread limit reached", ErrLimit)
		}
		for _, raw := range connection.Nodes {
			if raw == nil || raw.Comments.PageInfo == nil || raw.Comments.PageInfo.HasNextPage == nil || seen[raw.ID] {
				out.Complete = false
				out.Reason = "Invalid or duplicate discussion omitted"
				continue
			}
			nestedSeen := map[string]bool{}
			nestedFailure := false
			for raw.Comments.PageInfo.more() && len(raw.Comments.Nodes) < 100 && total+len(raw.Comments.Nodes) < 2000 {
				next := raw.Comments.PageInfo.EndCursor
				if next == "" || !validDiscussionField(next, 4096, false) || nestedSeen[next] {
					nestedFailure = true
					break
				}
				nestedSeen[next] = true
				b, err := request(discussionRepliesQuery, map[string]any{"id": raw.ID, "cursor": next})
				if err != nil {
					nestedFailure = true
					break
				}
				var r struct {
					Errors []json.RawMessage
					Data   struct {
						Node *struct{ Comments discussionComments }
					}
				}
				if json.Unmarshal(b, &r) != nil || r.Data.Node == nil || r.Data.Node.Comments.PageInfo == nil || r.Data.Node.Comments.PageInfo.HasNextPage == nil || len(r.Errors) > 0 || len(r.Data.Node.Comments.Nodes) > 20 {
					nestedFailure = true
					break
				}
				raw.Comments.Nodes = append(raw.Comments.Nodes, r.Data.Node.Comments.Nodes...)
				raw.Comments.PageInfo = r.Data.Node.Comments.PageInfo
			}
			if len(raw.Comments.Nodes) > 100 {
				raw.Comments.Nodes = raw.Comments.Nodes[:100]
				nestedFailure = true
			}
			if total+len(raw.Comments.Nodes) > 2000 {
				return partial("Discussion comment limit reached", ErrLimit)
			}
			d, t, err := normalizeDiscussion(raw, id)
			if err != nil {
				out.Complete = false
				out.Reason = "Invalid discussion omitted"
				continue
			}
			parentsComplete := true
			commentIDs := map[int64]bool{}
			for _, c := range d.Comments {
				commentIDs[c.ID] = true
			}
			for _, c := range d.Comments {
				if c.ParentID != 0 && !commentIDs[c.ParentID] {
					parentsComplete = false
				}
			}
			if nestedFailure || raw.Comments.PageInfo.more() || d.Comments[0].ParentID != 0 || !parentsComplete {
				out.Complete = false
				out.Reason = "Some discussion replies or roots unavailable"
			}
			duplicate := false
			for _, c := range d.Comments {
				if canonicalComments[c.ID] {
					duplicate = true
					break
				}
			}
			if duplicate {
				out.Complete = false
				out.Reason = "Duplicate discussion identity omitted"
				continue
			}
			for _, c := range d.Comments {
				canonicalComments[c.ID] = true
			}
			total += len(d.Comments)
			seen[d.ID] = true
			times[d.ID] = t
			out.Threads = append(out.Threads, d)
		}
		if len(response.Errors) > 0 {
			out.Complete = false
			out.Reason = "Some discussions unavailable"
		}
		if !connection.PageInfo.more() {
			break
		}
		next := connection.PageInfo.EndCursor
		if next == "" || next == cursor || !validDiscussionField(next, 4096, false) {
			return partial("Invalid discussion pagination", errors.New("invalid discussion pagination"))
		}
		cursor = next
		if page == 4 {
			out.Complete = false
			out.Reason = "Discussion thread limit reached"
		}
	}
	finalize()
	return out, nil
}
func nullableCursor(s string) any {
	if s == "" {
		return nil
	}
	return s
}
