// Package issuecontext provides explicitly authorized, read-only issue context.
package issuecontext

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"prui/internal/source"
	"regexp"
	"strings"
)

// Config contains authorization and credential names, never credential values.
// This separate global file cannot be selected by reviewed repository content.
type Config struct {
	Enabled       bool     `json:"enabled"`
	Repositories  []string `json:"repositories"`
	Workspace     string   `json:"workspace"`
	CredentialEnv string   `json:"credential_env"`
	Auth          string   `json:"auth"` // api_key or oauth (existing access token)
}

var workspacePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`)
var envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func ConfigPath(guidePath string) string {
	if !filepath.IsAbs(guidePath) {
		return ""
	}
	return filepath.Join(filepath.Dir(guidePath), "issue-context.json")
}
func LoadConfig(path string) (Config, error) {
	if !filepath.IsAbs(path) {
		return Config{}, errors.New("Linear configuration unavailable")
	}
	f, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, errors.New("Linear configuration unavailable")
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		return Config{}, errors.New("invalid Linear configuration")
	}
	var c Config
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || !c.valid() {
		return Config{}, errors.New("invalid Linear configuration")
	}
	return c, nil
}
func (c Config) valid() bool {
	if !c.Enabled {
		return !c.Enabled && len(c.Repositories) == 0 && c.Workspace == "" && c.CredentialEnv == "" && c.Auth == ""
	}
	if len(c.Repositories) == 0 || len(c.Repositories) > 100 || !workspacePattern.MatchString(c.Workspace) || !envPattern.MatchString(c.CredentialEnv) || (c.Auth != "api_key" && c.Auth != "oauth") {
		return false
	}
	for _, r := range c.Repositories {
		if _, err := source.ParseIdentity("1", r); err != nil {
			return false
		}
	}
	return true
}
func (c Config) authorized(repository string) bool {
	for _, r := range c.Repositories {
		if strings.EqualFold(r, repository) {
			return true
		}
	}
	return false
}
