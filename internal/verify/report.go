// Package verify defines the machine-readable boundary for manual, agent-run
// acceptance checks. It does not open pull requests or read credentials.
package verify

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

type Status string

const (
	Passed  Status = "passed"
	Failed  Status = "failed"
	Skipped Status = "skipped"
	NotRun  Status = "not_run"
)

type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Artifacts struct {
	Report     string   `json:"report,omitempty"`
	Transcript string   `json:"transcript,omitempty"`
	Screens    []string `json:"screens,omitempty"`
}

// Failure contains source-safe diagnostic context for a failed verifier stage.
// Stage is a fixed verifier step name; elapsed_ms is measured locally.
type Failure struct {
	Stage     string `json:"stage"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

type Report struct {
	SchemaVersion int       `json:"schema_version"`
	Status        Status    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	PR            string    `json:"pr,omitempty"`
	HeadSHA       string    `json:"head_sha,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`
	Checks        []Check   `json:"checks"`
	Artifacts     Artifacts `json:"artifacts"`
	Timing        *Timing   `json:"timing,omitempty"`
	Failure       *Failure  `json:"failure,omitempty"`
}

func NewReport() Report { return Report{SchemaVersion: 1, Status: NotRun} }

// MarshalJSON keeps schema versioning explicit even when a caller constructs a
// report literal in a test or failure path.
func (r Report) MarshalJSON() ([]byte, error) {
	type report Report
	if r.SchemaVersion == 0 {
		r.SchemaVersion = 1
	}
	return json.Marshal(report(r))
}

func validateArtifactPath(path string) error {
	if path == "" || filepath.IsAbs(path) {
		return errUnsafeArtifactPath
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errUnsafeArtifactPath
	}
	return nil
}
