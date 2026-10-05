package source

import (
	"errors"
	"strconv"
	"unicode/utf8"
)

// ValidateReviewCommentTarget validates target shape. Captured-patch membership
// and remote freshness must additionally be checked at the write boundary.
func ValidateReviewCommentTarget(t ReviewCommentTarget) error {
	_, err := ParseIdentity(strconv.Itoa(t.Identity.Number), t.Identity.Repository)
	if err != nil || !shaPattern.MatchString(t.CommitID) || t.Path == "" || !utf8.ValidString(t.Path) {
		return errors.New("invalid review comment target")
	}
	return ValidateCommentCoordinates(t)
}

// ValidateCommentCoordinates checks the line/file shape independently of identity.
func ValidateCommentCoordinates(t ReviewCommentTarget) error {
	if t.SubjectType == "file" {
		if t.Line != 0 || t.Side != "" || t.StartLine != 0 || t.StartSide != "" {
			return errors.New("file comment cannot contain line coordinates")
		}
		return nil
	}
	if t.SubjectType != "" || t.Line <= 0 || (t.Side != "LEFT" && t.Side != "RIGHT") {
		return errors.New("invalid line comment target")
	}
	if t.StartLine == 0 && t.StartSide == "" {
		return nil
	}
	if t.StartLine <= 0 || t.StartLine >= t.Line || t.StartSide != t.Side {
		return errors.New("range must increase on a single diff side")
	}
	return nil
}

type commentPayload struct {
	Path        string `json:"path"`
	Body        string `json:"body"`
	Line        int    `json:"line,omitempty"`
	Side        string `json:"side,omitempty"`
	StartLine   int    `json:"start_line,omitempty"`
	StartSide   string `json:"start_side,omitempty"`
	SubjectType string `json:"subject_type,omitempty"`
}

func commentPayloadFor(c ReviewComment) commentPayload {
	t := c.Target
	return commentPayload{Path: t.Path, Body: c.Body, Line: t.Line, Side: t.Side, StartLine: t.StartLine, StartSide: t.StartSide, SubjectType: t.SubjectType}
}
func remoteCommentAnchor(id Identity, sha, path, side string, line, start *int, startSide, subject string) *ReviewCommentTarget {
	t := ReviewCommentTarget{Identity: id, CommitID: sha, Path: path}
	if subject == "FILE" {
		if line != nil || start != nil {
			return nil
		}
		t.SubjectType = "file"
	} else {
		t.Side = side
		if line != nil {
			t.Line = *line
		}
		if start != nil {
			t.StartLine = *start
			t.StartSide = startSide
		}
	}
	if ValidateReviewCommentTarget(t) != nil {
		return nil
	}
	return &t
}

// Malformed history cannot prove absence of an attempted write. Cross-side
// remote ranges are valid GitHub data but are not supported outbound targets.
func validateRemoteRange(line, start *int, side, endSide string) error {
	if start != nil && (*start <= 0 || line != nil && side == endSide && *start >= *line || side != "LEFT" && side != "RIGHT") {
		return errors.New("invalid review comment range")
	}
	return nil
}
