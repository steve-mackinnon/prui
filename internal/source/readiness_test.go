package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const readinessSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const readinessBase = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type readinessFixture struct {
	responses map[string]any
	denied    map[string]bool
	calls     []string
	pulls     int
	changed   bool
}

func newReadinessFixture() *readinessFixture {
	return &readinessFixture{responses: map[string]any{
		"repos/o/r/branches/main":                                                             map[string]any{"protected": false},
		"repos/o/r/rules/branches/main?per_page=100&page=1":                                   []any{},
		"repos/o/r/commits/" + readinessSHA + "/statuses?per_page=100&page=1":                 []any{},
		"repos/o/r/commits/" + readinessSHA + "/check-runs?filter=latest&per_page=100&page=1": map[string]any{"check_runs": []any{}},
		"repos/o/r/pulls/1/reviews?per_page=100&page=1":                                       []any{},
		"graphql": map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"headRefOid": readinessSHA, "baseRefOid": readinessBase, "reviewDecision": "APPROVED", "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN", "state": "OPEN", "isDraft": false}}}},
	}, denied: map[string]bool{}}
}
func (f *readinessFixture) Run(_ context.Context, q Request) ([]byte, error) {
	if len(q.Args) < 4 || q.Args[0] != "api" || q.Args[1] != "--hostname" || q.Args[2] != "github.com" {
		return nil, fmt.Errorf("unexpected request %v", q.Args)
	}
	for _, a := range q.Args {
		if a == "--method" || a == "POST" || strings.Contains(a, "mutation") {
			return nil, errors.New("mutation in readiness")
		}
	}
	p := q.Args[3]
	f.calls = append(f.calls, p)
	if f.denied[p] {
		return nil, errors.New("synthetic permission denied")
	}
	if p == "repos/o/r/pulls/1" {
		f.pulls++
		head := readinessSHA
		if f.changed && f.pulls > 1 {
			head = strings.Repeat("c", 40)
		}
		return json.Marshal(map[string]any{"head": map[string]string{"sha": head}, "base": map[string]string{"sha": readinessBase, "ref": "main"}, "state": "open", "draft": false})
	}
	v, ok := f.responses[p]
	if !ok {
		return nil, fmt.Errorf("unconfigured request %s", p)
	}
	if strings.Contains(p, "/check-runs?") {
		obj := v.(map[string]any)
		clone := map[string]any{}
		for k, value := range obj {
			clone[k] = value
		}
		if _, ok := clone["total_count"]; !ok {
			total := 0
			prefix := p[:strings.LastIndex(p, "=")+1]
			for path, value := range f.responses {
				if strings.HasPrefix(path, prefix) {
					total += len(value.(map[string]any)["check_runs"].([]any))
				}
			}
			clone["total_count"] = total
		}
		v = clone
	}
	return json.Marshal(v)
}
func fixtureReadiness(t *testing.T, f *readinessFixture) Readiness {
	t.Helper()
	g := &GH{Runner: f, Executable: "synthetic", Limits: Defaults()}
	r, e := g.ReadReadiness(context.Background(), Identity{"o/r", 1})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func checkRow(id int, name, status, conclusion string, app int) any {
	return map[string]any{"id": id, "name": name, "head_sha": readinessSHA, "status": status, "conclusion": conclusion, "details_url": "https://example.com/details", "app": map[string]int{"id": app}}
}
func requireChecks(f *readinessFixture, rows ...any) {
	f.responses["repos/o/r/rules/branches/main?per_page=100&page=1"] = []any{map[string]any{"type": "required_status_checks", "parameters": map[string]any{"required_status_checks": rows, "strict_required_status_checks_policy": false}}}
}
func TestReadinessRequiredOptionalAndUnknownConclusions(t *testing.T) {
	for _, conclusion := range []string{"success", "failure", "cancelled", "skipped", "neutral", "timed_out", ""} {
		t.Run(conclusion, func(t *testing.T) {
			f := newReadinessFixture()
			requireChecks(f, map[string]any{"context": "ci", "integration_id": 7})
			f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"check_runs": []any{checkRow(1, "ci", "completed", conclusion, 7), checkRow(2, "optional", "completed", "failure", 8)}}
			r := fixtureReadiness(t, f)
			if r.Ready(readinessSHA) != (conclusion == "success") {
				t.Fatalf("wrong readiness %+v", r)
			}
			if r.Checks[0].Required != "required" || r.Checks[1].Required != "optional" || r.Checks[0].URL == "" {
				t.Fatalf("classification/links %+v", r.Checks)
			}
			if r.Ready(readinessBase) {
				t.Fatal("old expected head accepted")
			}
		})
	}
}
func TestReadinessMissingPermissionsAndChangingHeadNeverPass(t *testing.T) {
	for _, path := range []string{"repos/o/r/branches/main", "repos/o/r/rules/branches/main?per_page=100&page=1", "repos/o/r/commits/" + readinessSHA + "/statuses?per_page=100&page=1", "repos/o/r/pulls/1/reviews?per_page=100&page=1", "graphql"} {
		t.Run(path, func(t *testing.T) {
			f := newReadinessFixture()
			f.denied[path] = true
			r := fixtureReadiness(t, f)
			if r.Ready(readinessSHA) || len(r.Problems) == 0 {
				t.Fatalf("partial treated as ready %+v", r)
			}
		})
	}
	f := newReadinessFixture()
	f.changed = true
	r := fixtureReadiness(t, f)
	if r.HeadVerified || r.Ready(readinessSHA) || r.HeadSHA != readinessSHA {
		t.Fatal("changed head lost provenance", r)
	}
}
func TestReadinessContextAppIdentityAndClassicStatusDeduplication(t *testing.T) {
	f := newReadinessFixture()
	requireChecks(f, map[string]any{"context": "ci", "integration_id": 7}, map[string]any{"context": "legacy"})
	f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"check_runs": []any{checkRow(1, "ci", "completed", "success", 8)}}
	f.responses["repos/o/r/commits/"+readinessSHA+"/statuses?per_page=100&page=1"] = []any{map[string]any{"id": 2, "context": "legacy", "state": "failure"}, map[string]any{"id": 1, "context": "legacy", "state": "success"}}
	r := fixtureReadiness(t, f)
	if r.Ready(readinessSHA) || len(r.Checks) != 3 {
		t.Fatalf("wrong contexts %+v", r.Checks)
	}
	for _, c := range r.Checks {
		if c.Name == "legacy" && (c.State != "failure" || c.Required != "required") {
			t.Fatal("older success replaced latest failure", c)
		}
		if c.Name == "ci" && c.AppID == 8 && c.Required != "optional" {
			t.Fatal("wrong app fulfilled requirement")
		}
	}
}
func TestReadinessBoundedPaginationBothCheckAPIs(t *testing.T) {
	for _, kind := range []string{"statuses", "check-runs"} {
		t.Run(kind, func(t *testing.T) {
			f := newReadinessFixture()
			path := "repos/o/r/commits/" + readinessSHA + "/" + kind + "?"
			if kind == "check-runs" {
				path += "filter=latest&"
			}
			path += "per_page=100&page="
			rows := []any{}
			for i := 1; i <= 100; i++ {
				if kind == "statuses" {
					rows = append(rows, map[string]any{"id": i, "context": fmt.Sprint(i), "state": "success"})
				} else {
					rows = append(rows, checkRow(i, fmt.Sprint(i), "completed", "success", 7))
				}
			}
			set := func(page int, rows []any) {
				var v any = rows
				if kind == "check-runs" {
					v = map[string]any{"check_runs": rows}
				}
				f.responses[path+fmt.Sprint(page)] = v
			}
			set(1, rows)
			last := []any{checkRow(101, "tail", "in_progress", "", 7)}
			if kind == "statuses" {
				last = []any{map[string]any{"id": 101, "context": "tail", "state": "pending"}}
			}
			set(2, last)
			r := fixtureReadiness(t, f)
			if !r.ChecksComplete || len(r.Checks) != 101 {
				t.Fatal("pagination truncated", len(r.Checks), r.Problems)
			}
			for page := 2; page <= 5; page++ {
				set(page, rows)
			}
			r = fixtureReadiness(t, f)
			if r.ChecksComplete || r.Ready(readinessSHA) {
				t.Fatal("limit treated as complete")
			}
		})
	}
}
func reviewRow(id int, author, state, sha string) any {
	return map[string]any{"id": id, "user": map[string]string{"login": author}, "state": state, "commit_id": sha, "submitted_at": time.Date(2026, 1, 1, 0, id, 0, 0, time.UTC), "html_url": "https://github.com/o/r/pull/1#pullrequestreview-1"}
}
func TestReadinessDismissedStaleAndCommentOnlyReviews(t *testing.T) {
	f := newReadinessFixture()
	f.responses["repos/o/r/pulls/1/reviews?per_page=100&page=1"] = []any{reviewRow(1, "alice", "APPROVED", readinessBase), reviewRow(2, "alice", "COMMENTED", readinessSHA), reviewRow(3, "bob", "APPROVED", readinessSHA), reviewRow(4, "bob", "DISMISSED", readinessSHA)}
	r := fixtureReadiness(t, f)
	if len(r.Reviews) != 2 || r.Reviews[0].SHA != readinessBase || r.Reviews[0].Decision != "APPROVED" || r.Reviews[1].Decision != "DISMISSED" {
		t.Fatal(r.Reviews)
	}
}
func TestReadinessRequiredReviewAndMergeConflict(t *testing.T) {
	f := newReadinessFixture()
	f.responses["repos/o/r/rules/branches/main?per_page=100&page=1"] = []any{map[string]any{"type": "pull_request", "parameters": map[string]any{"required_approving_review_count": 2, "require_code_owner_review": false, "require_last_push_approval": false, "required_review_thread_resolution": false, "dismiss_stale_reviews_on_push": true}}}
	f.responses["graphql"] = map[string]any{"data": map[string]any{"repository": map[string]any{"pullRequest": map[string]any{"headRefOid": readinessSHA, "baseRefOid": readinessBase, "reviewDecision": "CHANGES_REQUESTED", "mergeable": "CONFLICTING", "mergeStateStatus": "DIRTY", "state": "OPEN", "isDraft": false}}}}
	r := fixtureReadiness(t, f)
	if r.RequiredReviews != 2 || r.ReviewDecision != "CHANGES_REQUESTED" || r.Ready(readinessSHA) || len(r.Blockers) < 2 {
		t.Fatalf("missing blockers %+v", r)
	}
}

func protectionFixture(f *readinessFixture) map[string]any {
	f.responses["repos/o/r/branches/main"] = map[string]any{"protected": true}
	p := map[string]any{
		"lock_branch": map[string]any{"enabled": false}, "restrictions": nil,
		"required_status_checks":        map[string]any{"strict": false, "contexts": []any{"ci"}, "checks": []any{map[string]any{"context": "ci", "app_id": 7}}},
		"required_pull_request_reviews": map[string]any{"required_approving_review_count": 1, "dismiss_stale_reviews": true, "require_code_owner_reviews": false, "require_last_push_approval": false},
		"required_signatures":           map[string]any{"enabled": false}, "required_conversation_resolution": map[string]any{"enabled": false}, "required_linear_history": map[string]any{"enabled": false},
	}
	f.responses["repos/o/r/branches/main/protection"] = p
	f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"check_runs": []any{checkRow(1, "ci", "completed", "success", 7)}}
	return p
}
func TestReadinessClassicProtectionMalformedAndOmittedPolicies(t *testing.T) {
	f := newReadinessFixture()
	protectionFixture(f)
	r := fixtureReadiness(t, f)
	if !r.Ready(readinessSHA) || r.RequiredReviews != 1 || r.Checks[0].Required != "required" {
		t.Fatalf("classic protection not applied %+v", r)
	}
	for _, field := range []string{"required_signatures", "required_conversation_resolution", "required_linear_history", "lock_branch", "required_status_checks", "required_pull_request_reviews"} {
		for _, bad := range []any{nil, map[string]any{}, map[string]any{"enabled": "false"}, "bad", 42} {
			t.Run(fmt.Sprintf("%s-%v", field, bad), func(t *testing.T) {
				f := newReadinessFixture()
				p := protectionFixture(f)
				p[field] = bad
				r := fixtureReadiness(t, f)
				if (field == "required_status_checks" || field == "required_pull_request_reviews") && bad == nil {
					return
				}
				if r.RequirementsKnown || r.Ready(readinessSHA) {
					t.Fatal("malformed policy looked known", r)
				}
			})
		}
		t.Run(field+"-omitted", func(t *testing.T) {
			f := newReadinessFixture()
			p := protectionFixture(f)
			delete(p, field)
			r := fixtureReadiness(t, f)
			if r.RequirementsKnown || r.Ready(readinessSHA) {
				t.Fatal("missing policy looked known", r)
			}
		})
	}
}
func TestReadinessInheritedAndClassicRequirementsCombine(t *testing.T) {
	f := newReadinessFixture()
	protectionFixture(f)
	requireChecks(f, map[string]any{"context": "org-ci", "integration_id": 9})
	r := fixtureReadiness(t, f)
	if !r.RequirementsKnown || len(r.Checks) != 2 || r.Ready(readinessSHA) {
		t.Fatal("inherited requirement lost", r)
	}
	for _, c := range r.Checks {
		if c.Required != "required" {
			t.Fatal("required context became optional", c)
		}
	}
}
func TestReadinessPartialCheckCountWrongHeadAndGraphRevisions(t *testing.T) {
	for _, bad := range []any{nil, 3, -1} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			f := newReadinessFixture()
			f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"total_count": bad, "check_runs": []any{checkRow(1, "ci", "completed", "success", 7)}}
			r := fixtureReadiness(t, f)
			if r.ChecksComplete || r.Ready(readinessSHA) {
				t.Fatal("count unavailable or short page passed")
			}
		})
	}
	f := newReadinessFixture()
	row := checkRow(1, "ci", "completed", "success", 7).(map[string]any)
	row["head_sha"] = readinessBase
	f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"check_runs": []any{row}}
	r := fixtureReadiness(t, f)
	if r.ChecksComplete || len(r.Checks) > 0 || r.Ready(readinessSHA) {
		t.Fatal("wrong head accepted")
	}
	f = newReadinessFixture()
	f.responses["graphql"].(map[string]any)["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["headRefOid"] = readinessBase
	r = fixtureReadiness(t, f)
	if r.HeadVerified || r.Ready(readinessSHA) {
		t.Fatal("graphql mismatching head accepted")
	}
}
func TestReadinessUnknownRuleKeepsUnmatchedChecksUnknown(t *testing.T) {
	f := newReadinessFixture()
	f.responses["repos/o/r/rules/branches/main?per_page=100&page=1"] = []any{map[string]any{"type": "future_policy", "parameters": map[string]any{}}}
	f.responses["repos/o/r/commits/"+readinessSHA+"/check-runs?filter=latest&per_page=100&page=1"] = map[string]any{"check_runs": []any{checkRow(1, "ci", "completed", "success", 7)}}
	r := fixtureReadiness(t, f)
	if r.RequirementsKnown || r.Checks[0].Required != "unknown" || r.Ready(readinessSHA) {
		t.Fatal("unknown policy made optional checks", r)
	}
}
func TestReadinessURLSafety(t *testing.T) {
	for _, raw := range []string{"https://ci.example.com/failure", "https://github.com/o/r/pull/1/checks"} {
		if readinessURL(raw) != raw {
			t.Fatal("valid detail link rejected")
		}
	}
	for _, raw := range []string{"http://ci.example.com", "javascript:alert(1)", "https://secret@ci.example.com", "https://ci.example.com/\n", "https://ci.example.com/\x1b[31m"} {
		if readinessURL(raw) != "" {
			t.Fatal("unsafe detail link", raw)
		}
	}
}
