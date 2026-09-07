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
var credentialContent = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|secret|password|private[_-]?key)\s*[:=]\s*[^\s]+`)

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
	if credentialContent.Match(b) {
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
