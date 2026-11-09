package agentchat

import (
	"strings"
	"testing"

	reviewcontext "prui/internal/context"
	"prui/internal/inventory"
	"prui/internal/privacy"
	"prui/internal/source"
)

func fixture() (inventory.Inventory, source.ReviewCommentTarget) {
	id := source.Identity{Repository: "owner/repo", Number: 1}
	head := strings.Repeat("a", 40)
	base := strings.Repeat("b", 40)
	f := inventory.FileChange{ID: "file", NewPath: []byte("pkg/work.go"), OldPath: []byte("pkg/work.go"), NewOID: strings.Repeat("c", 40), OldOID: strings.Repeat("d", 40), Status: "M"}
	u := inventory.ReviewUnit{ID: "unit", FileChangeID: f.ID, Kind: inventory.TextHunk, PatchReference: "patch"}
	inv := inventory.Inventory{
		Comparison: source.PinnedComparison{InventoryID: "comparison", Metadata: source.Metadata{Identity: id, HeadSHA: head}, MergeBaseSHA: base},
		Files:      []inventory.FileChange{f}, Units: []inventory.ReviewUnit{u},
		Patches: map[string][]byte{"patch": []byte("@@ -10,3 +10,3 @@\n context()\n-old()\n+newThing()\n tail()\n")},
	}
	target := source.ReviewCommentTarget{Identity: id, CommitID: head, Path: "pkg/work.go", Side: "RIGHT", Line: 11}
	return inv, target
}

func TestBuildRanksCapturedEvidenceAndResolvesExactCitations(t *testing.T) {
	inv, target := fixture()
	captured := reviewcontext.ContextBundle{ComparisonID: "comparison", Evidence: []reviewcontext.Evidence{
		{EvidenceID: "z", CommitSHA: inv.Comparison.Metadata.HeadSHA, Path: []byte("pkg/work.go"), LineStart: 11, LineEnd: 12, Excerpt: []byte("newThing()\n")},
		{EvidenceID: "a", CommitSHA: inv.Comparison.MergeBaseSHA, Path: []byte("pkg/test.go"), LineStart: 1, LineEnd: 2, Excerpt: []byte("newThing() test\n")},
		{EvidenceID: "unrelated", Path: []byte("pkg/other.go"), LineStart: 1, LineEnd: 2, Excerpt: []byte("elsewhere\n")},
	}}
	m, err := Build(inv, captured, privacy.Policy{}, target, "What does newThing do?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Evidence) != 2 || m.Evidence[0].ID != "z" || m.Evidence[1].ID != "a" {
		t.Fatalf("ranking = %+v", m.Evidence)
	}
	if m.Hunk.Revision != inv.Comparison.Metadata.HeadSHA || len(m.Hunk.Lines) != 3 || !m.Hunk.Lines[1].Selected {
		t.Fatalf("hunk = %+v", m.Hunk)
	}
	resolved, err := ValidateAnswer(m, Answer{Text: "The function is changed.", References: []Reference{{EvidenceID: "z", StartLine: 11, EndLine: 11}, {EvidenceID: "z", StartLine: 99, EndLine: 99}, {EvidenceID: "invented", StartLine: 1, EndLine: 1}}})
	if err != nil || !resolved.References[0].Available || resolved.References[0].Excerpt.Text != "newThing()\n" || resolved.References[1].Available || resolved.References[2].Available {
		t.Fatalf("resolution = %+v, %v", resolved, err)
	}
	if b, err := m.JSON(); err != nil || len(b) > MaxRequestBytes {
		t.Fatalf("JSON: %d, %v", len(b), err)
	}
}

func TestBuildPrivacyAndQuestionLimits(t *testing.T) {
	inv, target := fixture()
	if _, err := Build(inv, reviewcontext.ContextBundle{OmittedPaths: []reviewcontext.Omitted{{Path: []byte(target.Path), Reason: "credential-like content"}}}, privacy.Policy{}, target, "why?", nil); err == nil {
		t.Fatal("privacy ledger did not block selected file")
	}
	if _, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{Excluded: []string{"*.go"}}, target, "why?", nil); err == nil {
		t.Fatal("policy did not block selected file")
	}
	if _, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, strings.Repeat("q", MaxQuestionBytes+1), nil); err == nil {
		t.Fatal("oversized question accepted")
	}
	if _, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, " ", nil); err == nil {
		t.Fatal("blank question accepted")
	}
}

func TestBuildBoundedHistoryKeepsLatestCompleteTurns(t *testing.T) {
	inv, target := fixture()
	history := []Turn{{Question: "old", Answer: strings.Repeat("x", MaxHistoryBytes)}, {Question: "recent", Answer: "answer"}}
	m, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "follow up", history)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.History) != 1 || m.History[0].Question != "recent" {
		t.Fatalf("history = %+v", m.History)
	}
	found := false
	for _, o := range m.Omissions {
		if o.Reason == "earlier conversation turns omitted" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing history omission")
	}
}

func TestBuildOldSideKeepsOldRevisionAndSelectedDeletion(t *testing.T) {
	inv, target := fixture()
	target.Side = "LEFT"
	target.Line = 11
	m, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "Why remove old?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Hunk.Revision != inv.Comparison.MergeBaseSHA || m.Hunk.Lines[1].Text != "old()" || !m.Hunk.Lines[1].Selected {
		t.Fatalf("old hunk = %+v", m.Hunk)
	}
}

func TestBuildOldSideLine857IsSuppliedAlongsideDifferentFileContext(t *testing.T) {
	inv, target := fixture()
	inv.Files[0].OldPath = []byte("crates/dropout_core/src/engine/mod.rs")
	inv.Files[0].NewPath = []byte("crates/dropout_core/src/engine/mod.rs")
	inv.Patches["patch"] = []byte("diff --git a/crates/dropout_core/src/engine/mod.rs b/crates/dropout_core/src/engine/mod.rs\n" +
		"--- a/crates/dropout_core/src/engine/mod.rs\n+++ b/crates/dropout_core/src/engine/mod.rs\n" +
		"@@ -856,3 +856,2 @@\n fn build_engine() {\n-    old_engine_mode();\n }\n")
	target.Path = "crates/dropout_core/src/engine/mod.rs"
	target.Side = "LEFT"
	target.Line = 857
	captured := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{{
		EvidenceID: "params-evidence", CommitSHA: inv.Comparison.Metadata.HeadSHA,
		Path: []byte("crates/dropout_core/src/engine/params.rs"), LineStart: 20, LineEnd: 21,
		Excerpt: []byte("pub fn old_engine_mode_params() {}\nold_engine_mode\n"),
	}}}
	m, err := Build(inv, captured, privacy.Policy{}, target, "Why remove old_engine_mode from engine?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Hunk.ID != "hunk:unit" || m.Hunk.Revision != inv.Comparison.MergeBaseSHA || m.Hunk.Side != "LEFT" || m.Hunk.Path != target.Path {
		t.Fatalf("old-side hunk identity = %+v", m.Hunk)
	}
	selected := 0
	for _, line := range m.Hunk.Lines {
		if line.Selected {
			selected++
			if line.Line != 857 || line.Text != "    old_engine_mode();" {
				t.Fatalf("wrong selected source: %+v", line)
			}
		}
	}
	if selected != 1 || len(m.Evidence) != 1 || m.Evidence[0].Path != "crates/dropout_core/src/engine/params.rs" {
		t.Fatalf("selected/evidence = %d, %+v", selected, m.Evidence)
	}
	b, err := m.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id":"hunk:unit"`, `"line":857`, `"text":"    old_engine_mode();"`, `"selected":true`, `"side":"LEFT"`, `"path":"crates/dropout_core/src/engine/mod.rs"`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("serialized request lacks %s: %s", want, b)
		}
	}
}

func TestBuildNeverTruncatesOversizedSelectedLines(t *testing.T) {
	inv, target := fixture()
	inv.Patches["patch"] = []byte("@@ -1 +1 @@\n-a\n+" + strings.Repeat("x", MaxSourceBytes+1) + "\n")
	target.Line = 1
	if _, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "why?", nil); err == nil {
		t.Fatal("oversized selection accepted")
	}
}

func TestBuildRejectsPrivateQuestionAndOmitsPrivatePriorTurn(t *testing.T) {
	inv, target := fixture()
	if _, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "api_key=very-secret", nil); err == nil || strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("private current question was not safely blocked: %v", err)
	}
	m, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "Why change this?", []Turn{
		{Question: "previous", Answer: "access_token=hidden-value"},
		{Question: "recent", Answer: "ordinary answer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.History) != 1 || m.History[0].Question != "recent" {
		t.Fatalf("private turn survived: %+v", m.History)
	}
	b, err := m.JSON()
	if err != nil || strings.Contains(string(b), "hidden-value") {
		t.Fatalf("private history leaked: %v", err)
	}
	if omissionCount(m, "prior conversation turns withheld by privacy policy") != 1 {
		t.Fatalf("missing sanitized history omission: %+v", m.Omissions)
	}
}

func TestBuildSkipsDiffFileHeadersBeforeFirstHunk(t *testing.T) {
	inv, target := fixture()
	inv.Patches["patch"] = []byte("diff --git a/pkg/work.go b/pkg/work.go\nindex abc..def 100644\n--- a/pkg/work.go\n+++ b/pkg/work.go\n@@ -10,3 +10,3 @@\n context()\n-old()\n+newThing()\n tail()\n")
	m, err := Build(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, target, "Why?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Hunk.Lines) != 3 {
		t.Fatalf("header became source: %+v", m.Hunk.Lines)
	}
	for _, line := range m.Hunk.Lines {
		if line.Line <= 0 || strings.Contains(line.Text, "pkg/work.go") {
			t.Fatalf("header became source: %+v", line)
		}
	}
}

func TestBuildReportsOmittedSourceRangesAndPrivateExcerptCount(t *testing.T) {
	inv, target := fixture()
	target.Line = 2
	inv.Patches["patch"] = []byte("@@ -1,3 +1,3 @@\n " + strings.Repeat("x", MaxSourceBytes) + "\n-old()\n+tiny()\n " + strings.Repeat("y", MaxSourceBytes) + "\n")
	captured := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{
		{EvidenceID: "private", Path: []byte("other/secret.go"), LineStart: 1, LineEnd: 1, Excerpt: []byte("api_key=hidden-value")},
	}}
	m, err := Build(inv, captured, privacy.Policy{}, target, "Why tiny?", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Hunk.Lines) != 1 || m.Hunk.Lines[0].Line != 2 {
		t.Fatalf("oversized nearby rows survived: %+v", m.Hunk.Lines)
	}
	if omissionCount(m, "captured excerpts withheld by privacy policy") != 1 {
		t.Fatalf("private excerpt omission missing: %+v", m.Omissions)
	}
	var ranges []int
	for _, omission := range m.Omissions {
		if omission.Reason == "nearby hunk rows omitted" {
			ranges = append(ranges, omission.StartLine, omission.EndLine)
		}
	}
	if len(ranges) != 4 || ranges[0] != 1 || ranges[1] != 1 || ranges[2] != 3 || ranges[3] != 3 {
		t.Fatalf("omitted source ranges = %v", ranges)
	}
	b, err := m.JSON()
	if err != nil || strings.Contains(string(b), "hidden-value") || strings.Contains(string(b), "other/secret.go") {
		t.Fatalf("private excerpt leaked: %v", err)
	}
}

func omissionCount(m Manifest, reason string) int {
	for _, omission := range m.Omissions {
		if omission.Reason == reason {
			return omission.Count
		}
	}
	return 0
}
