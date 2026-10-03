// Package commits captures immutable first-parent commit diffs for offline browsing.
package commits

import (
	"prui/internal/inventory"
	"prui/internal/syntax"
)

type Status string

const (
	Captured    Status = "captured"
	Unavailable Status = "unavailable"
)

type Bundle struct {
	BaseSHA, HeadSHA string
	Status           Status
	Reason           string
	Complete         bool
	Entries          []Entry
}

type Entry struct {
	SHA, Subject, Author string
	Parents              []string
	Status               Status
	Reason               string
	Diff                 *Diff
}

type Diff struct {
	Files    []inventory.FileChange
	Units    []inventory.ReviewUnit
	Patches  map[string][]byte
	Syntax   map[string]syntax.Patch `json:"syntax,omitempty"`
	Complete bool
	Problems []string
}
