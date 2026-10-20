package commits

import "slices"

// Keep unusual/large scopes on the existing full-tree path. Limits charge the
// metadata actually read; filtering and frozen-source validation still follow.
func compositionPathspecs(paths map[string]bool) []string {
	if len(paths) == 0 || len(paths) > 256 {
		return nil
	}
	remaining := 64 << 10
	selected := make([]string, 0, len(paths))
	for path := range paths {
		if !safeCompositionPath(path) || len(path)+1 > remaining {
			return nil
		}
		remaining -= len(path) + 1
		selected = append(selected, path)
	}
	slices.Sort(selected)
	return selected
}

func compositionTreeArgs(tree string, selected []string) []string {
	args := []string{"ls-tree", "-r", "-z", "--full-tree", tree, "--"}
	if len(selected) > 0 {
		args = append([]string{"--literal-pathspecs"}, args...)
		args = append(args, selected...)
	}
	return args
}
