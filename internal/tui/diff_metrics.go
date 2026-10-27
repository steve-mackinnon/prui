package tui

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/theme"
)

type diffStat struct {
	added, removed int
	partial        bool
}

func (s diffStat) String() string {
	text := fmt.Sprintf("+%d/-%d", s.added, s.removed)
	if s.partial {
		text += " (partial)"
	}
	return text
}

func (s *diffStat) add(other diffStat) {
	s.added += other.added
	s.removed += other.removed
	s.partial = s.partial || other.partial
}

// Count only hunk bodies. Each unit is a distinct occurrence, even when two
// units reference identical patch bytes. Headers and no-newline markers do not count.
func unitDiffStat(u inventory.ReviewUnit, patches map[string][]byte) diffStat {
	s := diffStat{partial: u.Kind == inventory.Unavailable}
	if u.Kind != inventory.TextHunk {
		return s
	}
	patch, ok := patches[u.PatchReference]
	if !ok {
		s.partial = true
		return s
	}
	inHunk := false
	for len(patch) > 0 {
		line, rest, found := bytes.Cut(patch, []byte{'\n'})
		patch = rest
		switch {
		case bytes.HasPrefix(line, []byte("@@ -")):
			inHunk = true
		case bytes.HasPrefix(line, []byte("diff --git ")):
			inHunk = false
		case inHunk && len(line) > 0:
			switch line[0] {
			case '+':
				s.added++
			case '-':
				s.removed++
			}
		}
		if !found {
			break
		}
	}
	return s
}

func diffMetrics(units []inventory.ReviewUnit, patches map[string][]byte) map[string]diffStat {
	stats := make(map[string]diffStat)
	for _, u := range units {
		s := stats[u.FileChangeID]
		s.add(unitDiffStat(u, patches))
		stats[u.FileChangeID] = s
	}
	return stats
}

func portionDiffStat(s *review.Session, units []int) diffStat {
	var total diffStat
	seen := make(map[int]bool, len(units))
	for _, i := range units {
		if !seen[i] {
			total.add(unitDiffStat(s.Inventory.Units[i], s.Inventory.Patches))
			seen[i] = true
		}
	}
	return total
}

func sessionFileDivider(s *review.Session, file int) string {
	return fileDivider(s.Inventory.Files[file]) + " " + portionDiffStat(s, s.Slices[file].Units).String()
}

var diffStatPattern = regexp.MustCompile(`\+[0-9]+/-[0-9]+`)

// Metrics remain logical, unstyled text until the viewport has clipped them.
// Source lines never enter this UI-only styling path.
func (m *Model) styleDiffMetrics(s string) string {
	if len(m.styles) == 0 || m.colorProfile <= colorprofile.Ascii || !strings.Contains(s, "/-") {
		return s
	}
	return diffStatPattern.ReplaceAllStringFunc(s, func(stat string) string {
		added, removed, _ := strings.Cut(stat, "/")
		addColor, _ := m.theme.Color(theme.Added)
		removeColor, _ := m.theme.Color(theme.Removed)
		return semantic(addColor, false).Render(added) + "/" + semantic(removeColor, false).Render(removed)
	})
}
