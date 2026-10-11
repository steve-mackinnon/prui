package tui

import (
	"fmt"
	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/syntax"
)

func syntaxDiagnosticRows(status syntax.FileStatus) []diffLine {
	if status.Old == syntax.Ready && status.New == syntax.Ready {
		return nil
	}
	if status.Warning() {
		return body(classWarning, "! Syntax highlighting unavailable: "+status.Description()+". Source text is available.")
	}
	return body(classMetadata, "Syntax highlighting: "+status.Description()+".")
}

func fileSyntaxDiagnostic(inv inventory.Inventory, f inventory.FileChange) []diffLine {
	if status, ok := inv.SyntaxStatus[f.ID]; ok {
		return syntaxDiagnosticRows(status)
	}
	if len(inv.Syntax) == 0 {
		return nil
	}
	if status, ok := inventory.SyntaxDiagnostics([]inventory.FileChange{f}, inv.Units, inv.Syntax, nil)[f.ID]; ok {
		return syntaxDiagnosticRows(status)
	}
	return nil
}

// Syntax diagnostics are derived once from each immutable snapshot. Repaints
// inspect cached summaries rather than traversing every PR and commit unit.
type syntaxDiagnosticCache struct {
	session *review.Session
	main    map[string]syntax.FileStatus
	commits []commitSyntaxDiagnostic
	health  []healthItem
}

type commitSyntaxDiagnostic struct {
	sha      string
	files    []inventory.FileChange
	statuses map[string]syntax.FileStatus
}

func (m *Model) syntaxDiagnosticsFor(s *review.Session) *syntaxDiagnosticCache {
	if c := m.syntaxHealthCache; c != nil && c.session == s {
		return c
	}
	c := &syntaxDiagnosticCache{session: s, main: inventory.SyntaxDiagnostics(s.Inventory.Files, s.Inventory.Units, s.Inventory.Syntax, s.Inventory.SyntaxStatus)}
	diagnostics := c.main
	failed := 0
	for _, status := range diagnostics {
		if status.Warning() {
			failed++
		}
	}
	var items []healthItem
	if failed > 0 {
		items = append(items, healthItem{text: syntaxFailureSummary("Syntax unavailable", failed), severity: 2})
	}
	commitFailed := 0
	if s.Commits != nil {
		for _, entry := range s.Commits.Entries {
			if d := entry.Diff; d != nil {
				statuses := inventory.SyntaxDiagnostics(d.Files, d.Units, d.Syntax, d.SyntaxStatus)
				if len(statuses) > 0 {
					c.commits = append(c.commits, commitSyntaxDiagnostic{sha: entry.SHA, files: d.Files, statuses: statuses})
				}
				for _, status := range statuses {
					if status.Warning() {
						commitFailed++
					}
				}
			}
		}
	}
	if commitFailed > 0 {
		items = append(items, healthItem{text: syntaxFailureSummary("Commit syntax unavailable", commitFailed), severity: 2})
	}
	c.health = items
	m.syntaxHealthCache = c
	return c
}

func (m *Model) syntaxHealthDetails(s *review.Session) string {
	cache := m.syntaxDiagnosticsFor(s)
	text := ""
	add := func(label string, files []inventory.FileChange, diagnostics map[string]syntax.FileStatus) {
		for _, f := range files {
			if status, ok := diagnostics[f.ID]; ok {
				prefix := "Syntax"
				if status.Warning() {
					prefix = "! Syntax"
				}
				text += "\n" + prefix + " " + label + pathLabel(f) + ": " + status.Description()
			}
		}
	}
	add("", s.Inventory.Files, cache.main)
	for _, commit := range cache.commits {
		add("commit "+shortCommitSHA(commit.sha)+" · ", commit.files, commit.statuses)
	}
	if text != "" {
		text += "\nSyntax capture failures leave source text readable. To retry, open a new comparison; reopening a saved review preserves its metadata."
	}
	return text
}

func syntaxFailureSummary(label string, count int) string {
	unit := "file"
	if count != 1 {
		unit = "files"
	}
	return fmt.Sprintf("! %s (%d %s)", label, count, unit)
}
