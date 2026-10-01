package commits

import (
	"bytes"
	"regexp"
	"strconv"
	"unicode/utf8"

	"prui/internal/inventory"
	"prui/internal/source"
)

var targetHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)

// ContainsTarget verifies membership against raw captured hunk bytes. It does
// not establish whether GitHub accepts that commit's first-parent coordinates.
func ContainsTarget(bundle *Bundle, target source.ReviewCommentTarget) bool {
	if bundle == nil || bundle.Status != Captured {
		return false
	}
	for _, entry := range bundle.Entries {
		if entry.SHA != target.CommitID || entry.Status != Captured || entry.Diff == nil {
			continue
		}
		return InventoryContainsTarget(entry.Diff.Files, entry.Diff.Units, entry.Diff.Patches, target)
	}
	return false
}

// InventoryContainsTarget is shared by delivery validation for the frozen PR
// inventory and the selected commit. Presentation never determines an anchor.
func InventoryContainsTarget(files []inventory.FileChange, units []inventory.ReviewUnit, patches map[string][]byte, target source.ReviewCommentTarget) bool {
	if target.Line <= 0 || (target.Side != "LEFT" && target.Side != "RIGHT") || target.Path == "" || !utf8.ValidString(target.Path) {
		return false
	}
	byID := make(map[string]inventory.FileChange, len(files))
	for _, f := range files {
		byID[f.ID] = f
	}
	for _, u := range units {
		if u.Kind != inventory.TextHunk {
			continue
		}
		f, ok := byID[u.FileChangeID]
		if !ok {
			continue
		}
		oldLine, newLine := 0, 0
		anchored := false
		for raw := range bytes.SplitSeq(patches[u.PatchReference], []byte("\n")) {
			if header := targetHunkHeader.FindSubmatch(raw); header != nil {
				var e1, e2 error
				oldLine, e1 = strconv.Atoi(string(header[1]))
				newLine, e2 = strconv.Atoi(string(header[2]))
				anchored = e1 == nil && e2 == nil
				continue
			}
			if !anchored || len(raw) == 0 {
				continue
			}
			var path, side string
			var line int
			switch raw[0] {
			case '+':
				path, side, line = string(f.NewPath), "RIGHT", newLine
				newLine++
			case '-':
				path, side, line = string(f.OldPath), "LEFT", oldLine
				oldLine++
			case ' ':
				path, side, line = string(f.NewPath), "RIGHT", newLine
				oldLine++
				newLine++
			default:
				continue
			}
			if target.Path == path && target.Side == side && target.Line == line {
				return true
			}
		}
	}
	return false
}

// HistoricalCommentTarget accepts the first-parent coordinates verified against
// GitHub: ordinary single-parent additions and modifications on the right side.
// Deleted lines, renamed files, and root or merge commits remain unsupported.
func HistoricalCommentTarget(bundle *Bundle, target source.ReviewCommentTarget) bool {
	if bundle == nil || bundle.Status != Captured || target.Side != "RIGHT" {
		return false
	}
	for _, entry := range bundle.Entries {
		if entry.SHA != target.CommitID || entry.Status != Captured || len(entry.Parents) != 1 || entry.Diff == nil || !entry.Diff.Complete {
			continue
		}
		for _, file := range entry.Diff.Files {
			if string(file.NewPath) != target.Path || (file.Status != "A" && file.Status != "M") || (file.Status == "M" && !bytes.Equal(file.OldPath, file.NewPath)) {
				continue
			}
			if InventoryContainsTarget([]inventory.FileChange{file}, entry.Diff.Units, entry.Diff.Patches, target) {
				return true
			}
		}
	}
	return false
}
