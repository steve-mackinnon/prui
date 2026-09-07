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
