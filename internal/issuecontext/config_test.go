package issuecontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinearConfigExplicitGlobalAuthorization(t *testing.T) {
	if ConfigPath("config.json") != "" {
		t.Fatal("repository-relative config permitted")
	}
	path := filepath.Join(t.TempDir(), "issue-context.json")
	if c, err := LoadConfig(path); err != nil || c.Enabled {
		t.Fatal("absent config enabled", err)
	}
	for _, body := range []string{`{"enabled":true}`, `{"enabled":true,"token":"secret"}`, `{"enabled":true,"repositories":["../repo"],"workspace":"work","credential_env":"TEST","auth":"api_key"}`, `{} {}`, strings.Repeat("x", (64<<10)+1)} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(path); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid config accepted or exposed", err)
		}
	}
	valid := `{"enabled":true,"repositories":["o/r"],"workspace":"work","credential_env":"TEST","auth":"oauth"}`
	if err := os.WriteFile(path, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := LoadConfig(path); err != nil || !c.Enabled || !c.authorized("O/R") {
		t.Fatal(c, err)
	}
}
