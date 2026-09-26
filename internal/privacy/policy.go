package privacy

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
)

// Policy is user-owned and must be constructed outside the reviewed checkout.
type Policy struct {
	Excluded []string
}

var credentialName = regexp.MustCompile(`(?i)(^|[._-])(env|secret|credential|token|password|passwd|private|id_rsa)([._-]|$)`)

// These heuristics are defense in depth, not exhaustive secret detection. Scan
// source text rather than parsing a format: patches can contain partial JSON,
// YAML, or assignments and must be checked on both added and removed lines.
var credentialContent = regexp.MustCompile(`(?i)(api[_-]?key|(?:access|refresh|auth)[_-]?token|secret|password|passwd|private[_-]?key)["']?\s*(?::=|:|=)\s*[^\s]+`)
var credentialMaterial = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )?PRIVATE KEY-----|\bgh[pousr]_[A-Za-z0-9]{36}\b|\bgithub_pat_[A-Za-z0-9_]{82}\b|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)

func (p Policy) ExcludedPath(path []byte) (bool, string) {
	s := filepath.ToSlash(string(path))
	base := filepath.Base(s)
	if credentialName.MatchString(base) || strings.HasSuffix(strings.ToLower(base), ".pem") {
		return true, "credential-like filename"
	}
	for _, pattern := range p.Excluded {
		if ok, _ := filepath.Match(pattern, s); ok {
			return true, "user exclusion: " + pattern
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true, "user exclusion: " + pattern
		}
	}
	return false, ""
}

func (p Policy) ExcludedContent(b []byte) (bool, string) {
	if bytes.IndexByte(b, 0) >= 0 {
		return true, "binary content"
	}
	if credentialContent.Match(b) || credentialMaterial.Match(b) {
		return true, "credential-like content"
	}
	return false, ""
}

func (p Policy) Allows(path, content []byte) (bool, string) {
	if ok, reason := p.ExcludedPath(path); ok {
		return false, reason
	}
	if ok, reason := p.ExcludedContent(content); ok {
		return false, reason
	}
	return true, ""
}
