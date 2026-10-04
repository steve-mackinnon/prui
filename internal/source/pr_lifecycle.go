package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Lifecycle is live, ephemeral authorization evidence. It never changes code pins.
type Lifecycle struct {
	Identity                                                          Identity
	NodeID, HeadSHA, BaseSHA, State, Permission                       string
	Draft, Verified, PolicyKnown                                      bool
	CanUpdate, CanClose, CanReopen, CanAutoMerge, CanDisableAutoMerge bool
	AutoMergeAllowed, AutoMerge, QueueRequired, Queued                bool
	QueueState, QueueMethod                                           string
	Methods, Problems                                                 []string
	Readiness                                                         Readiness
}
type LifecycleAction struct {
	Kind, Method string
	Expected     Lifecycle
}
type LifecycleReader interface {
	ReadLifecycle(context.Context, Identity) (Lifecycle, error)
}
type LifecycleWriter interface {
	WriteLifecycle(context.Context, LifecycleAction) error
}

func (s Lifecycle) ValidateAction(a LifecycleAction) error {
	e := a.Expected
	if !s.Verified || !e.Verified || s.Identity != e.Identity || s.NodeID == "" || s.NodeID != e.NodeID || !shaPattern.MatchString(s.HeadSHA) || !shaPattern.MatchString(s.BaseSHA) || s.HeadSHA != e.HeadSHA || s.BaseSHA != e.BaseSHA || s.State != e.State || s.Draft != e.Draft || s.AutoMerge != e.AutoMerge || s.Queued != e.Queued {
		return errors.New("PR head, base or lifecycle state changed/unverified; refresh and confirm again")
	}
	write := s.Permission == "WRITE" || s.Permission == "MAINTAIN" || s.Permission == "ADMIN"
	open := s.State == "OPEN"
	switch a.Kind {
	case "merge", "enable-auto", "enqueue":
		if s.QueueRequired != e.QueueRequired || s.QueueMethod != e.QueueMethod {
			return errors.New("queue applicability or method changed; refresh and confirm again")
		}
		if !write || !open || s.Draft || !s.PolicyKnown {
			return errors.New("merge permission, open non-draft PR and known policy required")
		}
		if a.Kind == "merge" || (a.Kind == "enable-auto" && !s.QueueRequired) {
			found := false
			for _, m := range s.Methods {
				if m == a.Method {
					found = true
				}
			}
			if !found {
				return errors.New("merge method disabled by repository")
			}
		}
		if a.Kind == "merge" {
			r := s.Readiness
			if s.QueueRequired || s.Queued || r.Identity != s.Identity || r.BaseSHA != s.BaseSHA || !r.Ready(s.HeadSHA) {
				return errors.New("current readiness blocks direct merge; required queue or blockers must be resolved")
			}
		}
		if a.Kind == "enable-auto" && (!s.AutoMergeAllowed || !s.CanAutoMerge || s.AutoMerge) {
			return errors.New("auto-merge permission or policy unavailable")
		}
		if a.Kind == "enqueue" && (!s.QueueRequired || s.Queued || s.Readiness.Mergeable != "MERGEABLE") {
			return errors.New("queue unavailable, already queued or mergeability unknown/conflicting")
		}
	case "disable-auto":
		if !s.CanDisableAutoMerge || !open || !s.AutoMerge {
			return errors.New("auto-merge disable permission/state unavailable")
		}
	case "dequeue":
		if !write || !open || !s.Queued || !s.QueueRequired {
			return errors.New("queue removal permission/state unavailable")
		}
	case "draft", "ready":
		if !s.CanUpdate || !open || (a.Kind == "draft" && s.Draft) || (a.Kind == "ready" && !s.Draft) {
			return errors.New("draft/ready permission/state unavailable")
		}
	case "close":
		if !s.CanClose || !open {
			return errors.New("close permission/state unavailable")
		}
	case "reopen":
		if !s.CanReopen || s.State != "CLOSED" {
			return errors.New("reopen permission/state unavailable")
		}
	default:
		return errors.New("unknown lifecycle action")
	}
	return nil
}

func (g *GH) ReadLifecycle(ctx context.Context, id Identity) (Lifecycle, error) {
	if _, err := ParseIdentity(strconv.Itoa(id.Number), id.Repository); err != nil {
		return Lifecycle{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	r, err := g.ReadReadiness(ctx, id)
	if err != nil {
		return Lifecycle{}, err
	}
	s := Lifecycle{Identity: id, HeadSHA: r.HeadSHA, BaseSHA: r.BaseSHA, State: r.State, Draft: r.Draft, Readiness: r}
	parts := strings.Split(id.Repository, "/")
	query := `query($owner:String!,$repo:String!,$number:Int!,$branch:String!){repository(owner:$owner,name:$repo){viewerPermission mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed mergeQueue(branch:$branch){id} pullRequest(number:$number){id headRefOid baseRefOid state isDraft viewerCanUpdate viewerCanClose viewerCanReopen viewerCanEnableAutoMerge viewerCanDisableAutoMerge autoMergeRequest{enabledAt} mergeQueueEntry{state}}}}`
	b, err := g.call(ctx, "api", "--hostname", "github.com", "graphql", "-f", "query="+query, "-f", "owner="+parts[0], "-f", "repo="+parts[1], "-F", "number="+strconv.Itoa(id.Number), "-f", "branch="+r.BaseRef)
	var graph struct {
		Data struct {
			Repository *struct {
				ViewerPermission                                                             string
				MergeQueue                                                                   *struct{ ID string }
				MergeCommitAllowed, SquashMergeAllowed, RebaseMergeAllowed, AutoMergeAllowed *bool
				PullRequest                                                                  *struct {
					ID, HeadRefOid, BaseRefOid, State                                                                              string
					IsDraft, ViewerCanUpdate, ViewerCanClose, ViewerCanReopen, ViewerCanEnableAutoMerge, ViewerCanDisableAutoMerge *bool
					AutoMergeRequest                                                                                               *struct{ EnabledAt string }
					MergeQueueEntry                                                                                                *struct{ State string }
				}
			}
		}
		Errors []json.RawMessage
	}
	if err != nil || !utf8.Valid(b) || json.Unmarshal(b, &graph) != nil || len(graph.Errors) > 0 || graph.Data.Repository == nil || graph.Data.Repository.PullRequest == nil {
		return s, errors.New("GitHub lifecycle capabilities unavailable")
	}
	repo := graph.Data.Repository
	p := repo.PullRequest
	if repo.MergeCommitAllowed == nil || repo.SquashMergeAllowed == nil || repo.RebaseMergeAllowed == nil || repo.AutoMergeAllowed == nil || p.IsDraft == nil || p.ViewerCanUpdate == nil || p.ViewerCanClose == nil || p.ViewerCanReopen == nil || p.ViewerCanEnableAutoMerge == nil || p.ViewerCanDisableAutoMerge == nil || !lifecycleNullableFieldsPresent(b) || !validDiscussionField(p.ID, 256, false) || p.ID == "" {
		return s, errors.New("incomplete lifecycle permission/policy data")
	}
	s.NodeID = p.ID
	s.Permission = repo.ViewerPermission
	s.CanUpdate = *p.ViewerCanUpdate
	s.CanClose = *p.ViewerCanClose
	s.CanReopen = *p.ViewerCanReopen
	s.CanAutoMerge = *p.ViewerCanEnableAutoMerge
	s.CanDisableAutoMerge = *p.ViewerCanDisableAutoMerge
	s.AutoMergeAllowed = *repo.AutoMergeAllowed
	s.AutoMerge = p.AutoMergeRequest != nil
	s.QueueRequired = repo.MergeQueue != nil
	s.Queued = p.MergeQueueEntry != nil
	if s.Queued {
		s.QueueState = p.MergeQueueEntry.State
	}
	for _, m := range []struct {
		name    string
		allowed bool
	}{{"MERGE", *repo.MergeCommitAllowed}, {"SQUASH", *repo.SquashMergeAllowed}, {"REBASE", *repo.RebaseMergeAllowed}} {
		if m.allowed {
			s.Methods = append(s.Methods, m.name)
		}
	}
	var evidence Readiness
	req := g.readinessRequirementsWithPolicy(ctx, id, r.BaseRef, &evidence, true)
	s.PolicyKnown = req.known && req.queueRequired == s.QueueRequired
	s.QueueMethod = req.queueMethod
	methods := s.Methods[:0]
	for _, method := range s.Methods {
		if req.linear && method == "MERGE" {
			continue
		}
		for _, allowed := range req.methods {
			if method == allowed {
				methods = append(methods, method)
				break
			}
		}
	}
	s.Methods = methods
	for _, problem := range r.Problems {
		if problem != "Required versus optional classification unknown" {
			s.PolicyKnown = false
		}
	}
	if s.PolicyKnown {
		s.Readiness.RequirementsKnown = true
		s.Readiness.RequiredReviews = req.reviews
		s.Readiness.Checks = append([]ReadinessCheck(nil), r.Checks...)
		classifyReadinessChecks(&s.Readiness, req)
		s.Readiness.Problems = nil
		s.Readiness.Blockers = nil
		for _, blocker := range r.Blockers {
			if !lifecyclePolicyAdvisory(blocker) {
				s.Readiness.Blockers = append(s.Readiness.Blockers, blocker)
			}
		}
	}
	if !s.PolicyKnown {
		s.Problems = append(s.Problems, "Policy incomplete or unsupported; merge/auto/queue writes disabled")
	}
	after, e := g.readinessPull(ctx, id)
	s.Verified = r.HeadVerified && p.HeadRefOid == r.HeadSHA && p.BaseRefOid == r.BaseSHA && p.State == r.State && *p.IsDraft == r.Draft && e == nil && after.Head.SHA == r.HeadSHA && after.Base.SHA == r.BaseSHA && after.Base.Ref == r.BaseRef && (after.State == strings.ToLower(r.State) || (r.State == "MERGED" && after.State == "closed")) && after.Draft != nil && *after.Draft == r.Draft
	if !s.Verified {
		s.Problems = append(s.Problems, "Lifecycle revisions/state changed during retrieval")
	}
	return s, nil
}

// WriteLifecycle performs one request, with expected-head binding where the API
// supports it. Callers must freshly revalidate first; no retry or admin bypass.
func (g *GH) WriteLifecycle(ctx context.Context, a LifecycleAction) error {
	if err := a.Expected.ValidateAction(a); err != nil {
		return err
	}
	if _, err := ParseIdentity(strconv.Itoa(a.Expected.Identity.Number), a.Expected.Identity.Repository); err != nil {
		return err
	}
	if !validDiscussionField(a.Expected.NodeID, 256, false) {
		return errors.New("invalid PR node ID")
	}
	mutation, inputType, field := "", "", "pullRequestId"
	switch a.Kind {
	case "merge":
		mutation = "mergePullRequest"
		inputType = "MergePullRequestInput"
	case "enable-auto":
		mutation = "enablePullRequestAutoMerge"
		inputType = "EnablePullRequestAutoMergeInput"
	case "disable-auto":
		mutation = "disablePullRequestAutoMerge"
		inputType = "DisablePullRequestAutoMergeInput"
	case "enqueue":
		mutation = "enqueuePullRequest"
		inputType = "EnqueuePullRequestInput"
	case "dequeue":
		mutation = "dequeuePullRequest"
		inputType = "DequeuePullRequestInput"
		field = "id"
	case "draft":
		mutation = "convertPullRequestToDraft"
		inputType = "ConvertPullRequestToDraftInput"
	case "ready":
		mutation = "markPullRequestReadyForReview"
		inputType = "MarkPullRequestReadyForReviewInput"
	case "close":
		mutation = "closePullRequest"
		inputType = "ClosePullRequestInput"
	case "reopen":
		mutation = "reopenPullRequest"
		inputType = "ReopenPullRequestInput"
	}
	input := map[string]any{field: a.Expected.NodeID}
	if a.Kind == "merge" || a.Kind == "enable-auto" || a.Kind == "enqueue" {
		input["expectedHeadOid"] = a.Expected.HeadSHA
	}
	if a.Kind == "merge" || (a.Kind == "enable-auto" && !a.Expected.QueueRequired) {
		input["mergeMethod"] = a.Method
	}
	query := fmt.Sprintf("mutation($input:%s!){%s(input:$input){clientMutationId}}", inputType, mutation)
	body, err := json.Marshal(map[string]any{"query": query, "variables": map[string]any{"input": input}})
	if err != nil {
		return errors.New("invalid lifecycle request")
	}
	b, err := g.callWithStdin(ctx, body, "api", "--hostname", "github.com", "graphql", "--method", "POST", "--input", "-")
	var response struct {
		Data   map[string]json.RawMessage
		Errors []json.RawMessage
	}
	if err != nil || !utf8.Valid(b) || json.Unmarshal(b, &response) != nil || len(response.Errors) > 0 || len(response.Data[mutation]) == 0 || string(response.Data[mutation]) == "null" {
		return errors.New("lifecycle delivery uncertain; refresh canonical state; do not repeat automatically")
	}
	return nil
}

func (s Lifecycle) ActionObserved(a LifecycleAction) bool {
	if !s.Verified || s.Identity != a.Expected.Identity || s.NodeID != a.Expected.NodeID {
		return false
	}
	switch a.Kind {
	case "merge":
		return s.State == "MERGED"
	case "enable-auto":
		return s.AutoMerge
	case "disable-auto":
		return !s.AutoMerge
	case "enqueue":
		return s.Queued
	case "dequeue":
		return !s.Queued
	case "draft":
		return s.Draft
	case "ready":
		return !s.Draft
	case "close":
		return s.State == "CLOSED"
	case "reopen":
		return s.State == "OPEN"
	}
	return false
}

// LifecycleOutcome distinguishes a denied preflight from an attempted write.
type LifecycleOutcome struct {
	Snapshot                        Lifecycle
	Attempted, Refreshed, Uncertain bool
}

func lifecycleNullableFieldsPresent(b []byte) bool {
	var v struct {
		Data struct{ Repository map[string]json.RawMessage }
	}
	if json.Unmarshal(b, &v) != nil {
		return false
	}
	if _, ok := v.Data.Repository["mergeQueue"]; !ok {
		return false
	}
	var p map[string]json.RawMessage
	if json.Unmarshal(v.Data.Repository["pullRequest"], &p) != nil {
		return false
	}
	for _, name := range []string{"autoMergeRequest", "mergeQueueEntry"} {
		if _, ok := p[name]; !ok {
			return false
		}
	}
	return true
}
