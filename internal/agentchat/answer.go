package agentchat

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// Answer is the provider's structured result. References are claims supplied
// by the model, not navigation coordinates until resolved against this request.
type Answer struct {
	Text       string      `json:"text"`
	References []Reference `json:"references"`
}

type Reference struct {
	EvidenceID string `json:"evidence_id"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
}

type ResolvedReference struct {
	Reference
	Available bool    `json:"available"`
	Excerpt   Excerpt `json:"excerpt,omitempty"`
}

type ResolvedAnswer struct {
	Text       string              `json:"text"`
	References []ResolvedReference `json:"references"`
}

// ValidateAnswer keeps malformed prose out of the UI. Bad model citations stay
// visible as unavailable references, but cannot navigate to invented source.
func ValidateAnswer(m Manifest, answer Answer) (ResolvedAnswer, error) {
	if strings.TrimSpace(answer.Text) == "" || !utf8.ValidString(answer.Text) || len(answer.Text) > MaxAnswerBytes || len(answer.References) > 64 {
		return ResolvedAnswer{}, errors.New("agent answer is empty, invalid, or exceeds its limit")
	}
	byID := map[string]Excerpt{m.Hunk.ID: m.Hunk}
	for _, e := range m.Evidence {
		byID[e.ID] = e
	}
	resolved := ResolvedAnswer{Text: answer.Text, References: make([]ResolvedReference, 0, len(answer.References))}
	for _, ref := range answer.References {
		r := ResolvedReference{Reference: ref}
		e, ok := byID[ref.EvidenceID]
		if ok && ref.StartLine >= e.Start && ref.EndLine <= e.End && ref.StartLine > 0 && ref.StartLine <= ref.EndLine {
			if e.ID == m.Hunk.ID {
				// The hunk may omit nearby rows under the source budget. A
				// citation into an omitted row is unavailable too.
				covered := true
				for n := ref.StartLine; n <= ref.EndLine; n++ {
					found := false
					for _, line := range e.Lines {
						if line.Line == n {
							found = true
							break
						}
					}
					if !found {
						covered = false
						break
					}
				}
				ok = covered
			}
			if ok {
				r.Available = true
				r.Excerpt = e
			}
		}
		resolved.References = append(resolved.References, r)
	}
	return resolved, nil
}
