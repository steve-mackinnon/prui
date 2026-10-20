package source

import "strings"

// Only linear chains of loaded open PRs receive an ordinal. Fork branches,
// ambiguous heads, branching dependencies and cycles must not invent an order.
func annotatePullRequestStacks(prs []PullRequest) {
	parents := make([][]int, len(prs))
	children := make([][]int, len(prs))
	for i := range prs {
		prs[i].StackPosition, prs[i].StackSize = 0, 0
		for j := range prs {
			if prs[j].HeadBranch != "" && prs[i].TargetBranch == prs[j].HeadBranch && strings.EqualFold(prs[i].Identity.Repository, prs[j].HeadRepository) {
				parents[i] = append(parents[i], j)
				children[j] = append(children[j], i)
			}
		}
	}
	seen := make([]bool, len(prs))
	for start := range prs {
		if seen[start] {
			continue
		}
		component := []int{start}
		seen[start] = true
		valid, root := true, -1
		for n := 0; n < len(component); n++ {
			i := component[n]
			if len(parents[i]) > 1 || len(children[i]) > 1 {
				valid = false
			}
			if len(parents[i]) == 0 {
				root = i
			}
			for _, neighbors := range [][]int{parents[i], children[i]} {
				for _, j := range neighbors {
					if !seen[j] {
						seen[j] = true
						component = append(component, j)
					}
				}
			}
		}
		if !valid || root < 0 || len(component) < 2 {
			continue
		}
		i := root
		for position := 1; position <= len(component); position++ {
			prs[i].StackPosition, prs[i].StackSize = position, len(component)
			if len(children[i]) == 0 {
				break
			}
			i = children[i][0]
		}
	}
}
