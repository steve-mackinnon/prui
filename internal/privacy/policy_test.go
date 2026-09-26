package privacy

import "testing"

func TestPolicyFiltersPathsAndCredentialContent(t *testing.T) {
	p := Policy{Excluded: []string{"docs/private/*"}}
	for _, path := range []string{".env", "keys/server.pem", "docs/private/design.md"} {
		if ok, _ := p.ExcludedPath([]byte(path)); !ok {
			t.Fatalf("%s was not excluded", path)
		}
	}
	if ok, _ := p.Allows([]byte("config.txt"), []byte("api_key = super-secret")); ok {
		t.Fatal("credential-like content retained")
	}
	if ok, _ := p.Allows([]byte("README.md"), []byte("ordinary documentation")); !ok {
		t.Fatal("ordinary content excluded")
	}
}

func TestPolicyRecognizesCredentialSyntaxAndMaterial(t *testing.T) {
	for name, content := range map[string]string{
		"JSON":           `{"api_key": "synthetic-value"}`,
		"YAML":           `'password': 'synthetic-value'`,
		"camel case":     `const apiKey = "synthetic-value";`,
		"shell":          `export ACCESS_TOKEN=synthetic-value`,
		"Go assignment":  `clientSecret := "synthetic-value"`,
		"private key":    "-----BEGIN RSA PRIVATE KEY-----\nsynthetic-key-data",
		"GitHub token":   `value = "ghp_012345678901234567890123456789012345"`,
		"AWS access key": `value = "AKIA0123456789ABCDEF"`,
	} {
		t.Run(name, func(t *testing.T) {
			if excluded, reason := (Policy{}).ExcludedContent([]byte(content)); !excluded || reason != "credential-like content" {
				t.Fatal("credential-like content was not withheld")
			}
		})
	}
}

func TestPolicyRetainsOrdinaryCredentialDiscussion(t *testing.T) {
	for _, content := range []string{
		"Configure your API key before generating guides.",
		`{"password_length": 12, "api_key_count": 2}`,
		"func passwordLength(value string) int { return len(value) }",
		"-----BEGIN PUBLIC KEY-----",
		`value = "ghp_short-example"`,
	} {
		if excluded, reason := (Policy{}).ExcludedContent([]byte(content)); excluded {
			t.Errorf("ordinary content withheld: %q (%s)", content, reason)
		}
	}
}
