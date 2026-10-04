package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Readiness is ephemeral remote evidence, independent of immutable code pins.
// Ready is conservative evidence, never authorization to merge. A future writer
// must fetch a new snapshot and verify the expected head immediately before its write.
type Readiness struct {
	ViewerPermission                                                 string
	Identity                                                         Identity
	HeadSHA, BaseSHA, BaseRef                                        string
	ObservedAt                                                       time.Time
	Checks                                                           []ReadinessCheck
	Reviews                                                          []ReadinessReview
	RequiredReviews                                                  int
	ReviewDecision, Mergeable, MergeState, State                     string
	Draft                                                            bool
	ChecksComplete, ReviewsComplete, RequirementsKnown, HeadVerified bool
	Problems, Blockers                                               []string
}
type ReadinessCheck struct {
	ID, Name, Kind, SHA, State, Conclusion, URL string
	AppID                                       int64
	Required                                    string // required, optional, unknown
}
type ReadinessReview struct {
	ID                         int64
	Author, Decision, SHA, URL string
	SubmittedAt                time.Time
}
type ReadinessReader interface {
	ReadReadiness(context.Context, Identity) (Readiness, error)
}

func (r Readiness) Ready(expectedHead string) bool {
	if !shaPattern.MatchString(expectedHead) || !shaPattern.MatchString(r.BaseSHA) || r.RequiredReviews < 0 {
		return false
	}
	if !r.HeadVerified || r.HeadSHA != expectedHead || !r.ChecksComplete || !r.ReviewsComplete || !r.RequirementsKnown || len(r.Problems) > 0 || len(r.Blockers) > 0 || r.State != "OPEN" || r.Draft || r.Mergeable != "MERGEABLE" || r.MergeState != "CLEAN" {
		return false
	}
	if r.RequiredReviews > 0 && r.ReviewDecision != "APPROVED" {
		return false
	}
	for _, c := range r.Checks {
		if c.SHA != r.HeadSHA {
			return false
		}
		switch c.Required {
		case "required":
			if c.State != "success" {
				return false
			}
		case "optional":
		default:
			return false
		}
	}
	return true
}

type readinessPull struct {
	Head  struct{ SHA string }
	Base  struct{ SHA, Ref string }
	State string
	Draft *bool
}
type readinessFacts struct {
	HeadRefOid, BaseRefOid, ReviewDecision, Mergeable, MergeStateStatus, State string
	IsDraft                                                                    *bool
}
type requiredContext struct {
	Context string
	AppID   int64 `json:"app_id"`
}
type readinessRequirements struct {
	contexts              []requiredContext
	reviews               int
	known                 bool
	queueRequired, linear bool
	queueMethod           string
	methods               []string
}

func (g *GH) readinessJSON(ctx context.Context, path string, out any) error {
	b, err := g.call(ctx, "api", "--hostname", "github.com", path)
	if err != nil {
		return errors.New("GitHub data unavailable (permissions, transport, or resource)")
	}
	if !utf8.Valid(b) || json.Unmarshal(b, out) != nil {
		return errors.New("invalid GitHub readiness data")
	}
	return nil
}
func (g *GH) readinessPull(ctx context.Context, id Identity) (readinessPull, error) {
	var p readinessPull
	err := g.readinessJSON(ctx, fmt.Sprintf("repos/%s/pulls/%d", id.Repository, id.Number), &p)
	if err == nil && (!shaPattern.MatchString(p.Head.SHA) || !shaPattern.MatchString(p.Base.SHA) || p.Draft == nil || (p.State != "open" && p.State != "closed") || !validDiscussionField(p.Base.Ref, 1024, false) || p.Base.Ref == "") {
		err = errors.New("missing PR revision data")
	}
	return p, err
}

// ReadReadiness reads both classic statuses and checks at the observed head,
// effective branch rules (including inherited rules), classic protection, and
// current review decisions. Every list has a 500-record / 5-page bound; reaching
// it remains incomplete. No automatic retries, mutations, storage or upload.
func (g *GH) ReadReadiness(ctx context.Context, id Identity) (Readiness, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return Readiness{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := g.readinessPull(ctx, id)
	if err != nil {
		return Readiness{}, err
	}
	r := Readiness{Identity: id, HeadSHA: p.Head.SHA, BaseSHA: p.Base.SHA, BaseRef: p.Base.Ref, ObservedAt: time.Now().UTC(), RequiredReviews: -1, State: strings.ToUpper(p.State), Draft: *p.Draft}
	req := g.readinessRequirements(ctx, id, p.Base.Ref, &r)
	r.RequirementsKnown = req.known
	if req.known || req.reviews > 0 {
		r.RequiredReviews = req.reviews
	}
	g.readinessChecks(ctx, &r)
	classifyReadinessChecks(&r, req)
	g.readinessReviews(ctx, &r)
	parts := strings.Split(id.Repository, "/")
	query := `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){viewerPermission pullRequest(number:$number){headRefOid baseRefOid reviewDecision mergeable mergeStateStatus state isDraft}}}`
	b, e := g.call(ctx, "api", "--hostname", "github.com", "graphql", "-f", "query="+query, "-f", "owner="+parts[0], "-f", "repo="+parts[1], "-F", "number="+strconv.Itoa(id.Number))
	revisionsChanged := false
	var graph struct {
		Data struct {
			Repository struct {
				PullRequest      *readinessFacts
				ViewerPermission string
			}
		}
		Errors []json.RawMessage
	}
	if e != nil || !utf8.Valid(b) || json.Unmarshal(b, &graph) != nil || len(graph.Errors) > 0 || graph.Data.Repository.PullRequest == nil {
		r.Problems = append(r.Problems, "Current review decision and merge blockers unavailable")
	} else {
		r.ViewerPermission = graph.Data.Repository.ViewerPermission
		f := graph.Data.Repository.PullRequest
		if f.HeadRefOid != r.HeadSHA || f.BaseRefOid != r.BaseSHA || (f.State != r.State && !(r.State == "CLOSED" && f.State == "MERGED")) || f.IsDraft == nil || *f.IsDraft != r.Draft {
			revisionsChanged = true
			r.Problems = append(r.Problems, "PR revisions/state/draft changed or unavailable during readiness retrieval")
		} else {
			r.ReviewDecision = f.ReviewDecision
			r.Mergeable = f.Mergeable
			r.MergeState = f.MergeStateStatus
			r.State = f.State
			r.Draft = *f.IsDraft
		}
	}
	after, e := g.readinessPull(ctx, id)
	r.HeadVerified = !revisionsChanged && e == nil && after.Head.SHA == r.HeadSHA && after.Base.SHA == r.BaseSHA && after.Base.Ref == r.BaseRef && after.State == p.State && after.Draft != nil && *after.Draft == *p.Draft
	if !r.HeadVerified {
		r.Problems = append(r.Problems, "Head/base freshness unverified or changed during retrieval")
	}
	if r.Draft {
		r.Blockers = append(r.Blockers, "Draft pull request")
	}
	if r.State != "OPEN" {
		r.Blockers = append(r.Blockers, "Pull request is not open")
	}
	if r.Mergeable == "CONFLICTING" {
		r.Blockers = append(r.Blockers, "Merge conflicts")
	}
	if r.MergeState != "" && r.MergeState != "CLEAN" {
		r.Blockers = append(r.Blockers, "GitHub merge state: "+r.MergeState)
	}
	if r.RequiredReviews > 0 && r.ReviewDecision != "APPROVED" {
		r.Blockers = append(r.Blockers, "Required review decision: "+nonemptyReadiness(r.ReviewDecision, "unknown"))
	}
	for _, c := range r.Checks {
		if c.Required == "required" && c.State != "success" {
			r.Blockers = append(r.Blockers, "Required check "+c.Name+": "+c.State)
		}
	}
	return r, nil
}
func nonemptyReadiness(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (g *GH) readinessRequirements(ctx context.Context, id Identity, branch string, r *Readiness) readinessRequirements {
	return g.readinessRequirementsWithPolicy(ctx, id, branch, r, false)
}

func (g *GH) readinessRequirementsWithPolicy(ctx context.Context, id Identity, branch string, r *Readiness, richKnown bool) readinessRequirements {
	q := readinessRequirements{known: true, methods: []string{"MERGE", "SQUASH", "REBASE"}}
	path := fmt.Sprintf("repos/%s/branches/%s", id.Repository, url.PathEscape(branch))
	var b struct{ Protected *bool }
	if g.readinessJSON(ctx, path, &b) != nil || b.Protected == nil {
		q.known = false
		r.Problems = append(r.Problems, "Classic branch protection unavailable")
	} else if *b.Protected {
		var protection struct {
			RequiredStatusChecks *struct {
				Contexts []string
				Checks   []requiredContext
				Strict   bool
			}
			RequiredPullRequestReviews *struct {
				RequiredApprovingReviewCount *int `json:"required_approving_review_count"`
				RequireCodeOwnerReviews      bool `json:"require_code_owner_reviews"`
				DismissStaleReviews          bool `json:"dismiss_stale_reviews"`
				RequireLastPushApproval      bool `json:"require_last_push_approval"`
			}
		}
		// Explicit tags are needed for GitHub's snake_case protection fields.
		var raw map[string]json.RawMessage
		if g.readinessJSON(ctx, path+"/protection", &raw) != nil || raw == nil {
			q.known = false
			r.Problems = append(r.Problems, "Protected branch requirements unavailable")
		} else {
			if _, ok := raw["required_status_checks"]; !ok {
				q.known = false
			}
			if _, ok := raw["required_pull_request_reviews"]; !ok {
				q.known = false
			}
			if json.Unmarshal(raw["required_status_checks"], &protection.RequiredStatusChecks) != nil {
				q.known = false
			}
			if json.Unmarshal(raw["required_pull_request_reviews"], &protection.RequiredPullRequestReviews) != nil {
				q.known = false
			}
			for _, key := range []string{"required_signatures", "required_conversation_resolution", "required_linear_history", "lock_branch"} {
				var policy struct{ Enabled *bool }
				if json.Unmarshal(raw[key], &policy) != nil || policy.Enabled == nil {
					q.known = false
					r.Problems = append(r.Problems, "Policy unavailable: "+key)
				} else if *policy.Enabled {
					if key == "required_linear_history" {
						q.linear = true
					}
					r.Blockers = append(r.Blockers, "Branch policy: "+key+" (verify on GitHub)")
				}
			}
			if data, present := raw["restrictions"]; !present {
				q.known = false
				r.Problems = append(r.Problems, "Branch push restrictions unavailable")
			} else if string(data) != "null" {
				var restrictions struct{ Users, Teams, Apps []json.RawMessage }
				if json.Unmarshal(data, &restrictions) != nil || restrictions.Users == nil || restrictions.Teams == nil || restrictions.Apps == nil {
					q.known = false
					r.Problems = append(r.Problems, "Branch push restrictions malformed")
				} else {
					r.Blockers = append(r.Blockers, "Branch push restrictions (verify merge authorization on GitHub)")
				}
			}
			if v := protection.RequiredStatusChecks; v != nil {
				if !readinessBooleans(raw["required_status_checks"], "strict") {
					q.known = false
				}
				if v.Checks == nil && v.Contexts == nil {
					q.known = false
				}
				q.contexts = append(q.contexts, v.Checks...)
				for _, s := range v.Contexts {
					found := false
					for _, c := range v.Checks {
						if c.Context == s {
							found = true
						}
					}
					if !found {
						q.contexts = append(q.contexts, requiredContext{s, -1})
					}
				}
				if v.Strict {
					r.Blockers = append(r.Blockers, "Strict status checks require an up-to-date branch; verify on GitHub")
				}
			}
			if v := protection.RequiredPullRequestReviews; v != nil {
				if !readinessBooleans(raw["required_pull_request_reviews"], "require_code_owner_reviews", "dismiss_stale_reviews", "require_last_push_approval") {
					q.known = false
				}
				if v.RequiredApprovingReviewCount == nil || *v.RequiredApprovingReviewCount < 0 || *v.RequiredApprovingReviewCount > 6 {
					q.known = false
				} else {
					q.reviews = *v.RequiredApprovingReviewCount
				}
				if v.RequireCodeOwnerReviews || v.RequireLastPushApproval {
					r.Blockers = append(r.Blockers, "Additional owner/last-push approval requirements; verify on GitHub")
				}
			}

		}
	}
	type rule struct {
		Type       string
		Parameters json.RawMessage
	}
	for page := 1; page <= 5; page++ {
		var rules []rule
		e := g.readinessJSON(ctx, fmt.Sprintf("repos/%s/rules/branches/%s?per_page=100&page=%d", id.Repository, url.PathEscape(branch), page), &rules)
		if e != nil || rules == nil || len(rules) > 100 {
			q.known = false
			r.Problems = append(r.Problems, "Effective branch rules unavailable (including inherited requirements)")
			break
		}
		for _, rule := range rules {
			if richKnown && !lifecycleRuleValid(rule.Type, rule.Parameters) {
				q.known = false
			}
			switch rule.Type {
			case "required_status_checks":
				var v struct {
					RequiredStatusChecks []struct {
						Context       string
						IntegrationID *int64 `json:"integration_id"`
					} `json:"required_status_checks"`
					Strict bool `json:"strict_required_status_checks_policy"`
				}
				if json.Unmarshal(rule.Parameters, &v) != nil || v.RequiredStatusChecks == nil || !readinessBooleans(rule.Parameters, "strict_required_status_checks_policy") {
					q.known = false
					continue
				}
				for _, c := range v.RequiredStatusChecks {
					app := int64(-1)
					if c.IntegrationID != nil {
						app = *c.IntegrationID
					}
					q.contexts = append(q.contexts, requiredContext{c.Context, app})
				}
				if v.Strict {
					r.Blockers = append(r.Blockers, "Rules require an up-to-date branch; verify on GitHub")
				}
			case "pull_request":
				if richKnown {
					allowed, valid := lifecycleAllowedMethods(rule.Parameters)
					if !valid {
						q.known = false
					}
					if allowed != nil {
						var methods []string
						for _, m := range q.methods {
							for _, a := range allowed {
								if m == a {
									methods = append(methods, m)
									break
								}
							}
						}
						q.methods = methods
					}
				}
				var v struct {
					Count   *int `json:"required_approving_review_count"`
					Owners  bool `json:"require_code_owner_review"`
					Last    bool `json:"require_last_push_approval"`
					Threads bool `json:"required_review_thread_resolution"`
				}
				if json.Unmarshal(rule.Parameters, &v) != nil || v.Count == nil || *v.Count < 0 || *v.Count > 6 || !readinessBooleans(rule.Parameters, "require_code_owner_review", "require_last_push_approval", "required_review_thread_resolution", "dismiss_stale_reviews_on_push") {
					q.known = false
					continue
				}
				q.reviews = max(q.reviews, *v.Count)
				if v.Owners || v.Last || v.Threads {
					r.Blockers = append(r.Blockers, "Rules require owner/last-push approval or resolved threads; verify on GitHub")
				}
			default:
				if rule.Type == "merge_queue" {
					q.queueRequired = true
					var queue struct {
						Method string `json:"merge_method"`
					}
					if json.Unmarshal(rule.Parameters, &queue) == nil {
						q.queueMethod = queue.Method
					}
				}
				if rule.Type == "required_linear_history" {
					q.linear = true
				}
				if !richKnown || !lifecycleRuleValid(rule.Type, rule.Parameters) {
					q.known = false
				}
				r.Blockers = append(r.Blockers, "Effective rule: "+rule.Type+" (verify on GitHub)")
			}
		}
		if len(rules) < 100 {
			break
		}
		if page == 5 {
			q.known = false
			r.Problems = append(r.Problems, "Effective rules pagination limit")
		}
	}
	for _, c := range q.contexts {
		if c.Context == "" || !validDiscussionField(c.Context, 1024, false) || (c.AppID < -1 || c.AppID == 0) {
			q.known = false
		}
	}
	if !q.known {
		r.Problems = append(r.Problems, "Required versus optional classification unknown")
	}
	return q
}

func readinessCheckState(status, conclusion string) string {
	if status != "completed" {
		switch status {
		case "queued", "in_progress", "pending", "waiting", "requested":
			return "pending"
		default:
			return "unknown"
		}
	}
	switch conclusion {
	case "success":
		return "success"
	case "failure", "timed_out", "action_required", "startup_failure":
		return "failure"
	default:
		return "unknown"
	}
}
func (g *GH) readinessChecks(ctx context.Context, r *Readiness) {
	r.ChecksComplete = true
	for _, kind := range []string{"status", "check"} {
		received, total := 0, -1
		for page := 1; page <= 5; page++ {
			path := fmt.Sprintf("repos/%s/commits/%s/statuses?per_page=100&page=%d", r.Identity.Repository, r.HeadSHA, page)
			if kind == "check" {
				path = fmt.Sprintf("repos/%s/commits/%s/check-runs?filter=latest&per_page=100&page=%d", r.Identity.Repository, r.HeadSHA, page)
			}
			var rows []struct {
				ID                                       int64
				Name, Context, State, Status, Conclusion string
				SHA                                      string `json:"head_sha"`
				URL                                      string `json:"target_url"`
				Details                                  string `json:"details_url"`
				HTML                                     string `json:"html_url"`
				App                                      struct{ ID int64 }
			}
			var raw json.RawMessage
			e := g.readinessJSON(ctx, path, &raw)
			if kind == "check" {
				var v struct {
					CheckRuns  json.RawMessage `json:"check_runs"`
					TotalCount *int            `json:"total_count"`
				}
				if json.Unmarshal(raw, &v) != nil {
					e = errors.New("invalid checks")
				}
				if v.TotalCount == nil || *v.TotalCount < 0 || (total >= 0 && total != *v.TotalCount) {
					r.ChecksComplete = false
					r.Problems = append(r.Problems, "Check run total unavailable or changed during pagination")
				} else {
					total = *v.TotalCount
				}
				raw = v.CheckRuns
			}
			if e != nil || json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) > 100 {
				r.ChecksComplete = false
				r.Problems = append(r.Problems, kind+" results unavailable or incomplete")
				break
			}
			received += len(rows)
			for _, v := range rows {
				c := ReadinessCheck{ID: fmt.Sprintf("%s:%d", kind, v.ID), Kind: kind, SHA: r.HeadSHA, Required: "unknown", Name: v.Context, State: v.State, URL: readinessURL(v.URL)}
				if kind == "check" {
					c.Name = v.Name
					c.SHA = v.SHA
					c.AppID = v.App.ID
					c.Conclusion = v.Conclusion
					c.State = readinessCheckState(v.Status, v.Conclusion)
					c.URL = readinessURL(nonemptyReadiness(v.Details, v.HTML))
				} else {
					switch c.State {
					case "success":
					case "failure", "error":
						c.State = "failure"
					case "pending":
					default:
						c.State = "unknown"
					}
				}
				if !validDiscussionField(c.Name, 1024, false) || c.Name == "" || v.ID <= 0 || c.SHA != r.HeadSHA || (kind == "check" && c.AppID <= 0) {
					r.ChecksComplete = false
					r.Problems = append(r.Problems, "Invalid or wrong-head check omitted")
					continue
				}
				r.Checks = append(r.Checks, c)
			}
			if len(rows) < 100 {
				if kind == "check" && total != received {
					r.ChecksComplete = false
					r.Problems = append(r.Problems, "Check run count does not match retrieved pages")
				}
				break
			}
			if page == 5 {
				r.ChecksComplete = false
				r.Problems = append(r.Problems, kind+" pagination limit")
			}
		}
	}
	// Classic statuses are newest first; keep only the current value per context.
	seen := map[string]bool{}
	ids := map[string]bool{}
	checks := r.Checks[:0]
	for _, c := range r.Checks {
		if ids[c.ID] {
			r.ChecksComplete = false
			r.Problems = append(r.Problems, "Duplicate check identity across pages")
			continue
		}
		ids[c.ID] = true
		key := c.ID
		if c.Kind == "status" {
			key = "status:" + c.Name
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		checks = append(checks, c)
	}
	r.Checks = checks
}
func classifyReadinessChecks(r *Readiness, q readinessRequirements) {
	for i := range r.Checks {
		c := &r.Checks[i]
		if q.known {
			c.Required = "optional"
		}
		for _, req := range q.contexts {
			if c.Name == req.Context && (req.AppID < 0 || req.AppID == c.AppID) {
				c.Required = "required"
			}
		}
	}
	seen := map[string]bool{}
	for _, req := range q.contexts {
		key := fmt.Sprintf("%s:%d", req.Context, req.AppID)
		if seen[key] {
			continue
		}
		seen[key] = true
		found := false
		for _, c := range r.Checks {
			if c.Name == req.Context && (req.AppID < 0 || req.AppID == c.AppID) {
				found = true
			}
		}
		if !found {
			r.Checks = append(r.Checks, ReadinessCheck{ID: "required:" + key, Name: req.Context, AppID: req.AppID, Kind: "missing", SHA: r.HeadSHA, State: "unknown", Required: "required"})
		}
	}
	sort.SliceStable(r.Checks, func(i, j int) bool { return r.Checks[i].Name < r.Checks[j].Name })
}
func (g *GH) readinessReviews(ctx context.Context, r *Readiness) {
	r.ReviewsComplete = true
	latest := map[string]ReadinessReview{}
	for page := 1; page <= 5; page++ {
		var rows []struct {
			ID        int64
			State     string
			SHA       string    `json:"commit_id"`
			URL       string    `json:"html_url"`
			Submitted time.Time `json:"submitted_at"`
			User      *struct{ Login string }
		}
		e := g.readinessJSON(ctx, fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100&page=%d", r.Identity.Repository, r.Identity.Number, page), &rows)
		if e != nil || rows == nil || len(rows) > 100 {
			r.ReviewsComplete = false
			r.Problems = append(r.Problems, "Review decisions unavailable or incomplete")
			break
		}
		for _, v := range rows {
			if v.State == "PENDING" {
				continue
			}
			if v.User == nil || !validDiscussionField(v.User.Login, 256, false) || v.User.Login == "" || v.ID <= 0 || v.Submitted.IsZero() || !shaPattern.MatchString(v.SHA) {
				r.ReviewsComplete = false
				continue
			}
			switch v.State {
			case "APPROVED", "CHANGES_REQUESTED", "DISMISSED", "COMMENTED":
			default:
				r.ReviewsComplete = false
				continue
			}
			previous, ok := latest[v.User.Login]
			if ok && (v.Submitted.Before(previous.SubmittedAt) || (v.Submitted.Equal(previous.SubmittedAt) && v.ID < previous.ID)) {
				continue
			}
			// A comment-only review does not erase the reviewer's prior decision.
			if ok && v.State == "COMMENTED" {
				continue
			}
			latest[v.User.Login] = ReadinessReview{v.ID, v.User.Login, v.State, v.SHA, readinessURL(v.URL), v.Submitted}
		}
		if len(rows) < 100 {
			break
		}
		if page == 5 {
			r.ReviewsComplete = false
			r.Problems = append(r.Problems, "Reviews pagination limit")
		}
	}
	for _, v := range latest {
		r.Reviews = append(r.Reviews, v)
	}
	sort.Slice(r.Reviews, func(i, j int) bool { return r.Reviews[i].Author < r.Reviews[j].Author })
}

// Check providers may link to rich output outside github.com. Display only;
// never fetch it or forward credentials. Reject credentials/control characters.
func readinessURL(raw string) string {
	if !validDiscussionField(raw, 4096, false) {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return ""
	}
	return raw
}

func readinessBooleans(raw json.RawMessage, fields ...string) bool {
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return false
	}
	for _, key := range fields {
		var value *bool
		if json.Unmarshal(values[key], &value) != nil || value == nil {
			return false
		}
	}
	return true
}
func (c *requiredContext) UnmarshalJSON(raw []byte) error {
	var v struct {
		Context string
		AppID   *int64 `json:"app_id"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	c.Context = v.Context
	c.AppID = -1
	if v.AppID != nil {
		c.AppID = *v.AppID
	}
	return nil
}
