package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func lifecycleEvidence() Lifecycle {
	return Lifecycle{Identity: Identity{Repository: "o/r", Number: 1}, NodeID: "PR_1", HeadSHA: strings.Repeat("a", 40), BaseSHA: strings.Repeat("b", 40), State: "OPEN", Verified: true, PolicyKnown: true, Permission: "WRITE", CanUpdate: true, CanClose: true, CanReopen: true, CanAutoMerge: true, CanDisableAutoMerge: true, AutoMergeAllowed: true, Methods: []string{"MERGE", "SQUASH", "REBASE"}, Readiness: Readiness{Identity: Identity{Repository: "o/r", Number: 1}, BaseSHA: readinessBase, State: "OPEN", RequirementsKnown: true, RequiredReviews: 0, HeadVerified: true, HeadSHA: strings.Repeat("a", 40), ChecksComplete: true, ReviewsComplete: true, Mergeable: "MERGEABLE", MergeState: "CLEAN"}}
}
func TestLifecyclePolicyActions(t *testing.T) {
	for _, action := range []string{"merge", "enable-auto", "disable-auto", "enqueue", "dequeue", "draft", "ready", "close", "reopen"} {
		t.Run(action, func(t *testing.T) {
			s := lifecycleEvidence()
			switch action {
			case "disable-auto":
				s.AutoMerge = true
			case "enqueue":
				s.QueueRequired = true
			case "dequeue":
				s.QueueRequired = true
				s.Queued = true
			case "ready":
				s.Draft = true
			case "reopen":
				s.State = "CLOSED"
			}
			a := LifecycleAction{Kind: action, Method: "SQUASH", Expected: s}
			if err := s.ValidateAction(a); err != nil {
				t.Fatal(err)
			}
			s.Permission = "READ"
			s.CanClose = false
			s.CanReopen = false
			s.CanUpdate = false
			s.CanAutoMerge = false
			s.CanDisableAutoMerge = false
			if err := s.ValidateAction(a); err == nil {
				t.Fatal("permission denial accepted")
			}
		})
	}
}
func TestLifecycleRejectsStaleConfirmation(t *testing.T) {
	old := lifecycleEvidence()
	a := LifecycleAction{Kind: "close", Expected: old}
	for _, change := range []func(*Lifecycle){func(s *Lifecycle) { s.HeadSHA = strings.Repeat("c", 40) }, func(s *Lifecycle) { s.Draft = true }, func(s *Lifecycle) { s.State = "CLOSED" }, func(s *Lifecycle) { s.BaseSHA = strings.Repeat("c", 40) }} {
		s := old
		change(&s)
		if s.ValidateAction(a) == nil {
			t.Fatal("stale confirmation accepted")
		}
	}
}
func TestLifecycleMergePolicyDenials(t *testing.T) {
	for _, change := range []func(*Lifecycle){func(s *Lifecycle) { s.Methods = []string{"MERGE"} }, func(s *Lifecycle) { s.QueueRequired = true }, func(s *Lifecycle) { s.PolicyKnown = false }, func(s *Lifecycle) { s.Readiness.MergeState = "BLOCKED" }, func(s *Lifecycle) { s.Readiness.ChecksComplete = false }} {
		s := lifecycleEvidence()
		a := LifecycleAction{Kind: "merge", Method: "SQUASH", Expected: s}
		change(&s)
		if s.ValidateAction(a) == nil {
			t.Fatal("unsafe merge accepted")
		}
	}
}

func TestLifecycleContradictoryReadiness(t *testing.T) {
	changes := []func(*Readiness){
		func(r *Readiness) { r.Problems = []string{"unknown"} }, func(r *Readiness) { r.Blockers = []string{"Required check ci: failure"} },
		func(r *Readiness) { r.RequirementsKnown = false }, func(r *Readiness) { r.RequiredReviews = -1 }, func(r *Readiness) { r.RequiredReviews = 1; r.ReviewDecision = "CHANGES_REQUESTED" },
		func(r *Readiness) { r.State = "CLOSED" }, func(r *Readiness) { r.Draft = true },
		func(r *Readiness) {
			r.Checks = []ReadinessCheck{{SHA: r.HeadSHA, Required: "required", State: "failure"}}
		},
		func(r *Readiness) {
			r.Checks = []ReadinessCheck{{SHA: r.HeadSHA, Required: "unknown", State: "success"}}
		},
		func(r *Readiness) {
			r.Checks = []ReadinessCheck{{SHA: readinessBase, Required: "required", State: "success"}}
		},
	}
	for i, change := range changes {
		s := lifecycleEvidence()
		a := LifecycleAction{Kind: "merge", Method: "MERGE", Expected: s}
		change(&s.Readiness)
		if s.ValidateAction(a) == nil {
			t.Fatalf("contradiction %d accepted", i)
		}
	}
	s := lifecycleEvidence()
	s.Readiness.MergeState = "BLOCKED"
	s.Readiness.ChecksComplete = false
	if s.ValidateAction(LifecycleAction{Kind: "enable-auto", Method: "SQUASH", Expected: s}) != nil {
		t.Fatal("auto waiting unavailable")
	}
	s.QueueRequired = true
	if s.ValidateAction(LifecycleAction{Kind: "enqueue", Expected: s}) != nil {
		t.Fatal("queue waiting unavailable")
	}
}

type lifecycleTransport struct {
	*readinessFixture
	requests   []Request
	uncertain  bool
	capability map[string]any
}

func lifecycleTransportFixture() *lifecycleTransport {
	return &lifecycleTransport{readinessFixture: newReadinessFixture(), capability: map[string]any{"viewerPermission": "WRITE", "mergeCommitAllowed": true, "squashMergeAllowed": true, "rebaseMergeAllowed": false, "autoMergeAllowed": true, "mergeQueue": nil, "pullRequest": map[string]any{"id": "PR_1", "headRefOid": readinessSHA, "baseRefOid": readinessBase, "state": "OPEN", "isDraft": false, "viewerCanUpdate": true, "viewerCanClose": true, "viewerCanReopen": true, "viewerCanEnableAutoMerge": true, "viewerCanDisableAutoMerge": true, "autoMergeRequest": nil, "mergeQueueEntry": nil, "baseRef": map[string]any{"mergeQueue": nil}}}}
}
func (f *lifecycleTransport) Run(ctx context.Context, q Request) ([]byte, error) {
	if len(q.Stdin) > 0 {
		f.requests = append(f.requests, q)
		if f.uncertain {
			return nil, errors.New("synthetic timeout containing secret diagnostics")
		}
		var body struct{ Query string }
		if json.Unmarshal(q.Stdin, &body) != nil {
			return nil, errors.New("bad JSON")
		}
		for _, name := range []string{"mergePullRequest", "enablePullRequestAutoMerge", "disablePullRequestAutoMerge", "enqueuePullRequest", "dequeuePullRequest", "convertPullRequestToDraft", "markPullRequestReadyForReview", "closePullRequest", "reopenPullRequest"} {
			if strings.Contains(body.Query, name+"(") {
				return []byte(fmt.Sprintf(`{"data":{"%s":{"clientMutationId":null}}}`, name)), nil
			}
		}
		return nil, errors.New("unknown mutation")
	}
	if strings.Contains(strings.Join(q.Args, " "), "mergeCommitAllowed") {
		return json.Marshal(map[string]any{"data": map[string]any{"repository": f.capability}})
	}
	return f.readinessFixture.Run(ctx, q)
}
func TestLifecycleTransportEveryActionAndHeadBinding(t *testing.T) {
	for _, kind := range []string{"merge", "enable-auto", "disable-auto", "enqueue", "dequeue", "draft", "ready", "close", "reopen"} {
		t.Run(kind, func(t *testing.T) {
			f := lifecycleTransportFixture()
			g := &GH{Runner: f, Executable: "synthetic", Limits: Defaults()}
			s := lifecycleEvidence()
			switch kind {
			case "disable-auto":
				s.AutoMerge = true
			case "enqueue":
				s.QueueRequired = true
			case "dequeue":
				s.QueueRequired = true
				s.Queued = true
			case "ready":
				s.Draft = true
			case "reopen":
				s.State = "CLOSED"
			}
			a := LifecycleAction{Kind: kind, Method: "SQUASH", Expected: s}
			if e := g.WriteLifecycle(context.Background(), a); e != nil {
				t.Fatal(e)
			}
			if len(f.requests) != 1 {
				t.Fatal("write count")
			}
			q := f.requests[0]
			args := strings.Join(q.Args, " ")
			if args != "api --hostname github.com graphql --method POST --input -" {
				t.Fatal(args)
			}
			var body struct {
				Variables struct{ Input map[string]any }
			}
			if json.Unmarshal(q.Stdin, &body) != nil {
				t.Fatal("body")
			}
			in := body.Variables.Input
			field := "pullRequestId"
			if kind == "dequeue" {
				field = "id"
			}
			if in[field] != "PR_1" {
				t.Fatal(in)
			}
			if kind == "merge" || kind == "enable-auto" || kind == "enqueue" {
				if in["expectedHeadOid"] != readinessSHA {
					t.Fatal("missing head", in)
				}
			} else if _, ok := in["expectedHeadOid"]; ok {
				t.Fatal("unsupported head binding")
			}
			if in["jump"] != nil || in["bypass"] != nil {
				t.Fatal("bypass", in)
			}
			f.uncertain = true
			if e := g.WriteLifecycle(context.Background(), a); e == nil || strings.Contains(e.Error(), "secret") {
				t.Fatal("unsafe uncertainty", e)
			}
			if len(f.requests) != 2 {
				t.Fatal("automatic retry")
			}
		})
	}
}
func TestLifecycleReadCanonicalAndPartial(t *testing.T) {
	f := lifecycleTransportFixture()
	g := &GH{Runner: f, Executable: "synthetic", Limits: Defaults()}
	s, e := g.ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1})
	if e != nil || !s.Verified || !s.PolicyKnown || !s.Readiness.Ready(readinessSHA) || len(s.Methods) != 2 {
		t.Fatal(s, e)
	}
	f.capability["mergeCommitAllowed"] = nil
	if _, e = g.ReadLifecycle(context.Background(), s.Identity); e == nil {
		t.Fatal("missing capability accepted")
	}
}
func TestLifecycleRuleShapes(t *testing.T) {
	for _, kind := range []string{"required_status_checks", "pull_request", "merge_queue", "required_deployments"} {
		for _, raw := range []string{`null`, `{}`, `{"bogus":true}`, `[]`} {
			if lifecycleRuleValid(kind, json.RawMessage(raw)) {
				t.Fatal("malformed accepted", kind, raw)
			}
		}
	}
	if !lifecycleRuleValid("required_linear_history", nil) || lifecycleRuleValid("unknown", nil) {
		t.Fatal("rule classification")
	}
}

func TestLifecycleSupportedRichPolicyAndMalformedRules(t *testing.T) {
	queue := map[string]any{"merge_method": "SQUASH", "grouping_strategy": "ALLGREEN", "check_response_timeout_minutes": 60, "max_entries_to_build": 5, "max_entries_to_merge": 5, "min_entries_to_merge": 1, "min_entries_to_merge_wait_minutes": 0}
	for _, kind := range []string{"required_linear_history", "required_signatures", "merge_queue"} {
		t.Run(kind, func(t *testing.T) {
			f := lifecycleTransportFixture()
			rule := map[string]any{"type": kind}
			if kind == "merge_queue" {
				rule["parameters"] = queue
				f.capability["mergeQueue"] = map[string]any{"id": "QUEUE_1"}
			}
			f.responses["repos/o/r/rules/branches/main?per_page=100&page=1"] = []any{rule}
			g := &GH{Runner: f, Executable: "synthetic", Limits: Defaults()}
			s, e := g.ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1})
			if e != nil || !s.PolicyKnown || !s.Verified {
				t.Fatal(s, e)
			}
			action := LifecycleAction{Kind: "merge", Method: "SQUASH", Expected: s}
			if kind == "merge_queue" {
				action.Kind = "enqueue"
			}
			if e := s.ValidateAction(action); e != nil {
				t.Fatal("valid rich policy disabled", e, s.Readiness)
			}
			if kind == "required_linear_history" {
				action.Method = "MERGE"
				if s.ValidateAction(action) == nil {
					t.Fatal("linear policy offered merge commit")
				}
			}
			rule["parameters"] = map[string]any{"invalid": true}
			s, e = g.ReadLifecycle(context.Background(), s.Identity)
			if e != nil || s.PolicyKnown {
				t.Fatal("malformed policy treated known", s, e)
			}
		})
	}
}
func TestLifecycleMalformedClassicPolicyAndUnknownRules(t *testing.T) {
	for _, rules := range []any{[]any{map[string]any{"type": "new_unknown_policy"}}, []any{map[string]any{"type": "pull_request", "parameters": map[string]any{"required_approving_review_count": -1}}}} {
		f := lifecycleTransportFixture()
		f.responses["repos/o/r/rules/branches/main?per_page=100&page=1"] = rules
		s, e := (&GH{Runner: f, Executable: "synthetic", Limits: Defaults()}).ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1})
		if e != nil || s.PolicyKnown {
			t.Fatal("unknown malformed rule accepted", s, e)
		}
	}
	f := lifecycleTransportFixture()
	f.responses["repos/o/r/branches/main"] = map[string]any{"protected": true}
	f.responses["repos/o/r/branches/main/protection"] = map[string]any{"required_status_checks": map[string]any{"contexts": []string{}, "strict": "bogus"}, "required_pull_request_reviews": nil, "required_signatures": map[string]any{"enabled": false}, "required_linear_history": map[string]any{"enabled": false}, "required_conversation_resolution": map[string]any{"enabled": false}, "lock_branch": map[string]any{"enabled": false}, "restrictions": nil}
	s, e := (&GH{Runner: f, Executable: "synthetic", Limits: Defaults()}).ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1})
	if e != nil || s.PolicyKnown {
		t.Fatal("malformed classic policy accepted", s, e)
	}
}
func TestLifecycleMissingQueueAndAutoEvidence(t *testing.T) {
	for _, field := range []string{"mergeQueue", "autoMergeRequest", "mergeQueueEntry"} {
		f := lifecycleTransportFixture()
		if field == "mergeQueue" {
			delete(f.capability, field)
		} else {
			delete(f.capability["pullRequest"].(map[string]any), field)
		}
		if _, e := (&GH{Runner: f, Executable: "synthetic", Limits: Defaults()}).ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1}); e == nil {
			t.Fatal("omitted nullable evidence accepted", field)
		}
	}
}

func TestLifecycleMergedCanonicalEvidence(t *testing.T) {
	f := lifecycleTransportFixture()
	f.state = "closed"
	f.responses["graphql"].(map[string]any)["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["state"] = "MERGED"
	f.capability["pullRequest"].(map[string]any)["state"] = "MERGED"
	s, e := (&GH{Runner: f, Executable: "synthetic", Limits: Defaults()}).ReadLifecycle(context.Background(), Identity{Repository: "o/r", Number: 1})
	if e != nil || !s.Verified || s.State != "MERGED" {
		t.Fatal("merged state failed canonical refresh", s, e)
	}
	if !s.ActionObserved(LifecycleAction{Kind: "merge", Expected: s}) {
		t.Fatal("merged action not reconciled")
	}
}
