package source

import "encoding/json"

func lifecyclePolicyAdvisory(s string) bool {
	switch s {
	case "Branch policy: required_signatures (verify on GitHub)", "Branch policy: required_conversation_resolution (verify on GitHub)", "Branch policy: required_linear_history (verify on GitHub)", "Branch push restrictions (verify merge authorization on GitHub)", "Strict status checks require an up-to-date branch; verify on GitHub", "Rules require an up-to-date branch; verify on GitHub", "Additional owner/last-push approval requirements; verify on GitHub", "Rules require owner/last-push approval or resolved threads; verify on GitHub", "Effective rule: required_linear_history (verify on GitHub)", "Effective rule: required_signatures (verify on GitHub)", "Effective rule: non_fast_forward (verify on GitHub)", "Effective rule: required_deployments (verify on GitHub)":
		return true
	}
	return false
}
func lifecycleRuleValid(kind string, b json.RawMessage) bool {
	switch kind {
	case "required_linear_history", "required_signatures", "non_fast_forward":
		return len(b) == 0 || string(b) == "null" || string(b) == "{}"
	case "required_status_checks":
		var v struct {
			Checks []struct {
				Context string
				App     *int64 `json:"integration_id"`
			} `json:"required_status_checks"`
		}
		if json.Unmarshal(b, &v) != nil || v.Checks == nil || !readinessBooleans(b, "strict_required_status_checks_policy") {
			return false
		}
		for _, c := range v.Checks {
			if c.Context == "" || !validDiscussionField(c.Context, 1024, false) || (c.App != nil && *c.App <= 0) {
				return false
			}
		}
		return true
	case "pull_request":
		var v struct {
			Count *int `json:"required_approving_review_count"`
		}
		return json.Unmarshal(b, &v) == nil && v.Count != nil && *v.Count >= 0 && *v.Count <= 6 && readinessBooleans(b, "require_code_owner_review", "require_last_push_approval", "required_review_thread_resolution", "dismiss_stale_reviews_on_push")
	case "required_deployments":
		var v struct {
			Environments []string `json:"required_deployment_environments"`
		}
		if json.Unmarshal(b, &v) != nil || len(v.Environments) == 0 {
			return false
		}
		for _, e := range v.Environments {
			if !validDiscussionField(e, 1024, false) || e == "" {
				return false
			}
		}
		return true
	case "merge_queue":
		var v struct {
			Method   string `json:"merge_method"`
			Grouping string `json:"grouping_strategy"`
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(b, &v) != nil || json.Unmarshal(b, &fields) != nil || (v.Method != "MERGE" && v.Method != "SQUASH" && v.Method != "REBASE") || (v.Grouping != "ALLGREEN" && v.Grouping != "HEADGREEN") {
			return false
		}
		for _, name := range []string{"check_response_timeout_minutes", "max_entries_to_build", "max_entries_to_merge", "min_entries_to_merge", "min_entries_to_merge_wait_minutes"} {
			var n int
			if json.Unmarshal(fields[name], &n) != nil || n < 0 || (name != "min_entries_to_merge_wait_minutes" && n == 0) {
				return false
			}
		}
		return true
	}
	return false
}
