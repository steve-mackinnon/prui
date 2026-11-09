// Package agentchat builds bounded, immutable material for a single explicit
// question about a frozen pull-request hunk. It never reads the repository.
package agentchat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"prui/internal/commits"
	reviewcontext "prui/internal/context"
	"prui/internal/inventory"
	"prui/internal/privacy"
	"prui/internal/source"
)

const (
	MaxSourceBytes   = 32 << 10
	MaxQuestionBytes = 8 << 10
	MaxHistoryBytes  = 16 << 10
	MaxRequestBytes  = 128 << 10
	MaxAnswerBytes   = 32 << 10
)

type SourceLine struct {
	Line     int    `json:"line"`
	Text     string `json:"text"`
	Selected bool   `json:"selected"`
}

type Excerpt struct {
	ID       string       `json:"id"`
	Revision string       `json:"revision"`
	BlobID   string       `json:"blob_id,omitempty"`
	Path     string       `json:"path"`
	Side     string       `json:"side,omitempty"`
	Start    int          `json:"start"`
	End      int          `json:"end"`
	Text     string       `json:"text,omitempty"`
	Lines    []SourceLine `json:"lines,omitempty"`
}

type Turn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type Omission struct {
	Reason    string `json:"reason"`
	Count     int    `json:"count"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type Manifest struct {
	ComparisonID string                     `json:"comparison_id"`
	Anchor       source.ReviewCommentTarget `json:"anchor"`
	Question     string                     `json:"question"`
	Hunk         Excerpt                    `json:"hunk"`
	Evidence     []Excerpt                  `json:"evidence,omitempty"`
	History      []Turn                     `json:"history,omitempty"`
	Omissions    []Omission                 `json:"omissions,omitempty"`
}

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,[0-9]+)? \+([0-9]+)(?:,[0-9]+)? @@`)
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z_0-9]{2,}`)

// Build checks raw target membership and reapplies upload privacy to every
// captured excerpt. Returned slices own their data and are safe to retain for a
// request after the caller changes tab or view.
func Build(inv inventory.Inventory, captured reviewcontext.ContextBundle, policy privacy.Policy, target source.ReviewCommentTarget, question string, history []Turn) (Manifest, error) {
	if inv.Comparison.InventoryID == "" || target.CommitID != inv.Comparison.Metadata.HeadSHA ||
		target.Identity != inv.Comparison.Metadata.Identity || target.SubjectType != "" ||
		!commits.InventoryContainsTarget(inv.Files, inv.Units, inv.Patches, target) {
		return Manifest{}, errors.New("selected range is not in the frozen PR comparison")
	}
	if strings.TrimSpace(question) == "" || !utf8.ValidString(question) || len(question) > MaxQuestionBytes {
		return Manifest{}, errors.New("question must be nonblank UTF-8 within 8 KiB")
	}
	if excluded, _ := policy.ExcludedContent([]byte(question)); excluded {
		return Manifest{}, errors.New("question is excluded by the upload privacy policy")
	}
	if captured.ComparisonID != "" && captured.ComparisonID != inv.Comparison.InventoryID {
		return Manifest{}, errors.New("captured evidence belongs to another comparison")
	}
	hunk, fullPath, err := selectedHunk(inv, target)
	if err != nil {
		return Manifest{}, err
	}
	blocked := map[string]bool{}
	for _, o := range captured.OmittedPaths {
		if privacyOmission(o.Reason) {
			blocked[string(o.Path)] = true
		}
	}
	for _, e := range captured.Evidence {
		if allowed, _ := policy.Allows(e.Path, e.Excerpt); !allowed {
			blocked[string(e.Path)] = true
		}
	}
	privacyWithheldEvidence := 0
	for _, e := range captured.Evidence {
		if blocked[string(e.Path)] {
			privacyWithheldEvidence++
		}
	}
	if blocked[fullPath] {
		return Manifest{}, errors.New("selected source is excluded by the upload privacy policy")
	}
	for _, f := range inv.Files {
		if string(f.NewPath) != fullPath && string(f.OldPath) != fullPath {
			continue
		}
		if blocked[string(f.OldPath)] || blocked[string(f.NewPath)] {
			return Manifest{}, errors.New("selected source is excluded by the upload privacy policy")
		}
		for _, u := range inv.Units {
			if u.FileChangeID != f.ID {
				continue
			}
			if ok, _ := policy.Allows([]byte(fullPath), inv.Patches[u.PatchReference]); !ok {
				return Manifest{}, errors.New("selected source is excluded by the upload privacy policy")
			}
		}
	}
	m := Manifest{ComparisonID: inv.Comparison.InventoryID, Anchor: target, Question: question, Hunk: hunk}
	// The selected rows are mandatory. Add nearest captured rows while room
	// remains; record omitted rows without disclosing excluded paths.
	selectedBytes := 0
	selected := make([]int, 0, len(hunk.Lines))
	for i, line := range hunk.Lines {
		if line.Selected {
			selectedBytes += len(line.Text)
			selected = append(selected, i)
		}
	}
	if len(selected) == 0 || selectedBytes > MaxSourceBytes {
		return Manifest{}, errors.New("selected source exceeds the 32 KiB request limit")
	}
	remaining := MaxSourceBytes - selectedBytes
	keep := make([]bool, len(hunk.Lines))
	for _, i := range selected {
		keep[i] = true
	}
	order := make([]int, 0, len(hunk.Lines)-len(selected))
	for i := range hunk.Lines {
		if !keep[i] {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		da, db := distance(order[a], selected), distance(order[b], selected)
		if da != db {
			return da < db
		}
		return order[a] < order[b]
	})
	for _, i := range order {
		if len(hunk.Lines[i].Text) <= remaining {
			keep[i] = true
			remaining -= len(hunk.Lines[i].Text)
		}
	}
	var retained []SourceLine
	var omittedStart, omittedEnd int
	flushOmittedRange := func() {
		if omittedStart > 0 {
			m.Omissions = append(m.Omissions, Omission{Reason: "nearby hunk rows omitted", Count: omittedEnd - omittedStart + 1, StartLine: omittedStart, EndLine: omittedEnd})
			omittedStart, omittedEnd = 0, 0
		}
	}
	for i, line := range hunk.Lines {
		if keep[i] {
			flushOmittedRange()
			retained = append(retained, line)
		} else {
			if omittedStart != 0 && line.Line != omittedEnd+1 {
				flushOmittedRange()
			}
			if omittedStart == 0 {
				omittedStart = line.Line
			}
			omittedEnd = line.Line
		}
	}
	flushOmittedRange()
	m.Hunk.Lines = retained
	usedSource := MaxSourceBytes - remaining
	// The context capture's privacy ledger is authoritative even if an excerpt
	// itself looks harmless. Do not upload omitted path names.
	ids := map[string]bool{m.Hunk.ID: true}
	candidates := rankEvidence(captured.Evidence, target, question, m.Hunk, blocked, policy)
	for _, e := range candidates {
		if ids[e.ID] {
			addOmission(&m, "duplicate captured evidence omitted", 1)
			continue
		}
		if len(m.Evidence) >= 3 {
			addOmission(&m, "captured evidence count limit", 1)
			continue
		}
		if usedSource+len(e.Text) > MaxSourceBytes {
			addOmission(&m, "captured evidence source budget", 1)
			continue
		}
		ids[e.ID] = true
		m.Evidence = append(m.Evidence, e)
		usedSource += len(e.Text)
	}
	if len(m.Evidence) == 0 {
		addOmission(&m, "no relevant captured evidence supplied", 1)
	}
	if privacyWithheldEvidence > 0 {
		addOmission(&m, "captured excerpts withheld by privacy policy", privacyWithheldEvidence)
	}
	// History is complete turns only, newest first for budget selection then
	// restored to chronological order. Charge all history bytes against the
	// source budget too: quoted source can otherwise evade the 32 KiB bound.
	historyBytes := 0
	var reverse []Turn
	for i := len(history) - 1; i >= 0; i-- {
		t := history[i]
		n := len(t.Question) + len(t.Answer)
		if questionBlocked, _ := policy.ExcludedContent([]byte(t.Question)); questionBlocked {
			addOmission(&m, "prior conversation turns withheld by privacy policy", 1)
			continue
		}
		if answerBlocked, _ := policy.ExcludedContent([]byte(t.Answer)); answerBlocked {
			addOmission(&m, "prior conversation turns withheld by privacy policy", 1)
			continue
		}
		if t.Question == "" || t.Answer == "" || !utf8.ValidString(t.Question) || !utf8.ValidString(t.Answer) || n > MaxHistoryBytes-historyBytes || n > MaxSourceBytes-usedSource {
			addOmission(&m, "earlier conversation turns omitted", i+1)
			break
		}
		reverse = append(reverse, t)
		historyBytes += n
		usedSource += n
	}
	slices.Reverse(reverse)
	m.History = reverse
	for _, o := range captured.OmittedPaths {
		if privacyOmission(o.Reason) {
			addOmission(&m, "captured source withheld by privacy policy", 1)
		}
	}
	for _, reason := range captured.ExhaustedBudgets {
		if reason != "" {
			addOmission(&m, "captured evidence budget exhausted", 1)
		}
	}
	encoded, err := json.Marshal(m)
	if err != nil || len(encoded) > MaxRequestBytes {
		return Manifest{}, errors.New("request exceeds the 128 KiB serialized limit")
	}
	return m, nil
}

func addOmission(m *Manifest, reason string, count int) {
	for i := range m.Omissions {
		if m.Omissions[i].Reason == reason {
			m.Omissions[i].Count += count
			return
		}
	}
	m.Omissions = append(m.Omissions, Omission{Reason: reason, Count: count})
}
func privacyOmission(reason string) bool {
	return reason == "credential-like content" || reason == "credential-like filename" || reason == "binary content" || strings.HasPrefix(reason, "user exclusion: ")
}
func distance(i int, selected []int) int {
	d := len(selected) + i + 1
	for _, s := range selected {
		n := i - s
		if n < 0 {
			n = -n
		}
		if n < d {
			d = n
		}
	}
	return d
}

func selectedHunk(inv inventory.Inventory, target source.ReviewCommentTarget) (Excerpt, string, error) {
	for _, u := range inv.Units {
		if u.Kind != inventory.TextHunk {
			continue
		}
		var file inventory.FileChange
		found := false
		for _, f := range inv.Files {
			if f.ID == u.FileChangeID {
				file = f
				found = true
				break
			}
		}
		if !found {
			continue
		}
		path := file.NewPath
		blob := file.NewOID
		revision := inv.Comparison.Metadata.HeadSHA
		if target.Side == "LEFT" {
			path = file.OldPath
			blob = file.OldOID
			revision = inv.Comparison.MergeBaseSHA
		}
		if string(path) != target.Path {
			continue
		}
		patch := inv.Patches[u.PatchReference]
		if ok, _ := (privacy.Policy{}).ExcludedContent(patch); ok {
			continue
		}
		var lines []SourceLine
		oldLine, newLine := 0, 0
		inHunk := false
		for raw := range bytes.SplitSeq(patch, []byte("\n")) {
			if h := hunkHeader.FindSubmatch(raw); h != nil {
				oldLine, _ = strconv.Atoi(string(h[1]))
				newLine, _ = strconv.Atoi(string(h[2]))
				inHunk = true
				continue
			}
			if !inHunk {
				continue
			}
			if len(raw) == 0 {
				continue
			}
			var n int
			switch raw[0] {
			case '+':
				if target.Side != "RIGHT" {
					newLine++
					continue
				}
				n = newLine
				newLine++
			case '-':
				if target.Side != "LEFT" {
					oldLine++
					continue
				}
				n = oldLine
				oldLine++
			case ' ':
				if target.Side == "LEFT" {
					n = oldLine
				} else {
					n = newLine
				}
				oldLine++
				newLine++
			default:
				continue
			}
			if !utf8.Valid(raw[1:]) {
				return Excerpt{}, "", errors.New("selected hunk contains invalid UTF-8")
			}
			start := target.StartLine
			if start == 0 {
				start = target.Line
			}
			lines = append(lines, SourceLine{Line: n, Text: string(raw[1:]), Selected: n >= start && n <= target.Line})
		}
		selected := false
		for _, line := range lines {
			if line.Selected {
				selected = true
				break
			}
		}
		if selected {
			return Excerpt{ID: "hunk:" + u.ID, Revision: revision, BlobID: blob, Path: target.Path, Side: target.Side, Start: lines[0].Line, End: lines[len(lines)-1].Line, Lines: lines}, target.Path, nil
		}
	}
	return Excerpt{}, "", errors.New("selected hunk is unavailable")
}

type rankedExcerpt struct {
	excerpt Excerpt
	score   int
}

func rankEvidence(evidence []reviewcontext.Evidence, target source.ReviewCommentTarget, question string, hunk Excerpt, blocked map[string]bool, policy privacy.Policy) []Excerpt {
	queryIDs := map[string]bool{}
	for _, s := range identifier.FindAllString(question+" "+strings.Join(sourceTexts(hunk), " "), -1) {
		queryIDs[s] = true
	}
	var ranked []rankedExcerpt
	for _, e := range evidence {
		if e.EvidenceID == "" || blocked[string(e.Path)] || !utf8.Valid(e.Path) || !utf8.Valid(e.Excerpt) || e.LineStart <= 0 || e.LineEnd < e.LineStart {
			continue
		}
		if ok, _ := policy.Allows(e.Path, e.Excerpt); !ok {
			continue
		}
		score := 0
		if string(e.Path) == target.Path {
			start := target.StartLine
			if start == 0 {
				start = target.Line
			}
			if e.LineStart <= target.Line && e.LineEnd >= start {
				score = 2
			}
		}
		if score == 0 {
			for _, id := range identifier.FindAllString(string(e.Excerpt), -1) {
				if queryIDs[id] {
					score = 1
					break
				}
			}
		}
		if score == 0 {
			continue
		}
		ranked = append(ranked, rankedExcerpt{Excerpt{ID: e.EvidenceID, Revision: e.CommitSHA, BlobID: e.BlobID, Path: string(e.Path), Start: e.LineStart, End: e.LineEnd, Text: string(e.Excerpt)}, score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].excerpt.ID < ranked[j].excerpt.ID
	})
	out := make([]Excerpt, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.excerpt)
	}
	return out
}
func sourceTexts(e Excerpt) []string {
	out := make([]string, 0, len(e.Lines))
	for _, line := range e.Lines {
		if line.Selected {
			out = append(out, line.Text)
		}
	}
	return out
}

// JSON returns the exact bounded package for a provider call.
func (m Manifest) JSON() ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil || len(b) > MaxRequestBytes {
		return nil, fmt.Errorf("request exceeds the 128 KiB serialized limit")
	}
	return b, nil
}
