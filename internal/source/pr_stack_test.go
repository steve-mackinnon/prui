package source

import (
	"context"
	"strings"
	"testing"
)

func TestPullRequestStackPositions(t *testing.T) {
	pr := func(n int, base, head string) PullRequest {
		return PullRequest{Identity: Identity{Repository: "owner/repo", Number: n}, TargetBranch: base, HeadBranch: head, HeadRepository: "owner/repo"}
	}
	for _, tc := range []struct {
		name      string
		prs       []PullRequest
		positions []int
		size      int
	}{
		{"reverse list", []PullRequest{pr(3, "two", "three"), pr(1, "main", "one"), pr(2, "one", "two"), pr(4, "main", "solo")}, []int{3, 1, 2, 0}, 3},
		{"branching", []PullRequest{pr(1, "main", "one"), pr(2, "one", "two"), pr(3, "one", "three")}, []int{0, 0, 0}, 0},
		{"cycle", []PullRequest{pr(1, "two", "one"), pr(2, "one", "two")}, []int{0, 0}, 0},
		{"ambiguous head", []PullRequest{pr(1, "main", "one"), pr(2, "main", "one"), pr(3, "one", "two")}, []int{0, 0, 0}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			annotatePullRequestStacks(tc.prs)
			for i, p := range tc.prs {
				if p.StackPosition != tc.positions[i] || (p.StackPosition > 0 && p.StackSize != tc.size) {
					t.Fatalf("PR %d: %d/%d", p.Identity.Number, p.StackPosition, p.StackSize)
				}
			}
		})
	}
	fork := []PullRequest{pr(1, "main", "one"), pr(2, "one", "two")}
	fork[0].HeadRepository = "fork/repo"
	annotatePullRequestStacks(fork)
	if fork[1].StackSize != 0 {
		t.Fatal("fork head matched base repository branch")
	}
}

func TestGitHubListStackBranches(t *testing.T) {
	g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(_ context.Context, r Request) ([]byte, error) {
		for _, field := range []string{"headRefName", "headRepository{nameWithOwner}"} {
			if !strings.Contains(r.Args[5], field) {
				t.Fatalf("missing %s", field)
			}
		}
		return []byte(`{"data":{"repository":{"pullRequests":{"nodes":[{"number":2,"title":"Second","createdAt":"2026-09-23T12:00:00Z","baseRefName":"one","headRefName":"two","headRepository":{"nameWithOwner":"owner/repo"}},{"number":1,"title":"First","createdAt":"2026-09-23T12:00:00Z","baseRefName":"main","headRefName":"one","headRepository":{"nameWithOwner":"owner/repo"}}]}}}}`), nil
	})}
	prs, err := g.ListPullRequests(context.Background(), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if prs[0].StackPosition != 2 || prs[1].StackPosition != 1 || prs[0].StackSize != 2 {
		t.Fatal(prs)
	}
}

func TestGitHubListStackMetadataValidation(t *testing.T) {
	for _, tc := range []struct {
		name, branch, repository string
		valid                    bool
	}{
		{"missing head", `""`, `null`, true},
		{"fork", `"feature"`, `{"nameWithOwner":"fork/repo"}`, true},
		{"invalid branch", `"bad\nbranch"`, `null`, false},
		{"oversized branch", `"` + strings.Repeat("x", 513) + `"`, `null`, false},
		{"invalid repository", `"feature"`, `{"nameWithOwner":"bad/repo/extra"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := GH{Executable: "trusted-gh", Limits: Defaults(), Runner: listRunner(func(context.Context, Request) ([]byte, error) {
				return []byte(`{"data":{"repository":{"pullRequests":{"nodes":[{"number":1,"title":"Change","createdAt":"2026-09-23T12:00:00Z","headRefName":` + tc.branch + `,"headRepository":` + tc.repository + `}]}}}}`), nil
			})}
			prs, err := g.ListPullRequests(context.Background(), "owner/repo")
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && (len(prs) != 1 || prs[0].StackSize != 0) {
				t.Fatal("standalone or unknown head received a stack")
			}
		})
	}
}
