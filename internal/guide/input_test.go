package guide

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
)

// builder assembles a frozen-looking inventory without Git, so input policy and
// budget contracts are tested on the shape they actually receive.
type builder struct{ inv inventory.Inventory }

func (b *builder) unit(path, patch string, kind inventory.Kind) string {
	fileID := "file:" + path
	known := false
	for _, f := range b.inv.Files {
		known = known || f.ID == fileID
	}
	if !known {
		b.inv.Files = append(b.inv.Files, inventory.FileChange{ID: fileID, OldPath: []byte(path), NewPath: []byte(path), Status: "M"})
	}
	reference := ""
	if patch != "" {
		reference = fmt.Sprintf("%x", sha256.Sum256([]byte(patch)))
		if b.inv.Patches == nil {
			b.inv.Patches = map[string][]byte{}
		}
		b.inv.Patches[reference] = []byte(patch)
	}
	id := fmt.Sprintf("u%d", len(b.inv.Units)+1)
	b.inv.Units = append(b.inv.Units, inventory.ReviewUnit{ID: id, FileChangeID: fileID, Kind: kind, PatchReference: reference})
	return id
}

func (b *builder) build() inventory.Inventory {
	b.inv.Complete = true
	b.inv.Comparison.InventoryID = "inventory"
	return b.inv
}

func sent(in Input, id string) bool {
	for _, u := range in.Units {
		if u.ID == id {
			return true
		}
	}
	return false
}

func withheldFor(in Input, path string) string {
	for _, w := range in.Withheld {
		if string(w.Path) == path {
			return w.Reason
		}
	}
	return ""
}

func material(in Input) string {
	var b strings.Builder
	for _, u := range in.Units {
		b.Write(u.Path)
		b.Write(u.Patch)
	}
	for _, e := range in.Evidence {
		b.Write(e.Path)
		b.Write(e.Excerpt)
	}
	return b.String()
}

func TestInputPolicyFiltersUploads(t *testing.T) {
	b := &builder{}
	code := b.unit("internal/http/login.go", "@@ -1 +1 @@\n+func Login()\n", inventory.TextHunk)
	meta := b.unit("internal/http/login.go", "", inventory.FileMetadata)
	env := b.unit("config/.env", "@@ -0,0 +1 @@\n+TOKEN=abcdef\n", inventory.TextHunk)
	leaked := b.unit("internal/http/client.go", "@@ -0,0 +1 @@\n+api_key = \"sk-live-secret-value\"\n", inventory.TextHunk)
	excluded := b.unit("vendor/lib.go", "@@ -0,0 +1 @@\n+package lib\n", inventory.TextHunk)
	inv := b.build()
	c := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{
		{EvidenceID: "e1", Path: []byte("README.md"), Excerpt: []byte("project readme\n")},
		{EvidenceID: "e2", Path: []byte("deploy/secret.yaml"), Excerpt: []byte("nothing\n")},
	}}
	in := InputFrom(inv, c, privacy.Policy{Excluded: []string{"vendor/*"}}, Defaults)

	if !sent(in, code) || !sent(in, meta) {
		t.Fatal("eligible units missing from the request")
	}
	for _, id := range []string{env, leaked, excluded} {
		if sent(in, id) {
			t.Fatal("policy-excluded unit reached the request", id)
		}
	}
	if r := withheldFor(in, "config/.env"); r != "credential-like filename" {
		t.Fatal("credential-like filename not stated", r)
	}
	if r := withheldFor(in, "internal/http/client.go"); r != "credential-like content" {
		t.Fatal("credential-like content not stated", r)
	}
	if r := withheldFor(in, "vendor/lib.go"); !strings.Contains(r, "user exclusion") {
		t.Fatal("user exclusion not stated", r)
	}
	if strings.Contains(material(in), "sk-live-secret-value") || strings.Contains(material(in), "TOKEN=abcdef") {
		t.Fatal("secret content assembled into the request")
	}
	if len(in.Evidence) != 1 || in.Evidence[0].EvidenceID != "e1" {
		t.Fatal("evidence not re-checked at the upload boundary")
	}
	if len(in.Omissions) != 1 || string(in.Omissions[0].Path) != "deploy/secret.yaml" {
		t.Fatal("withheld evidence not recorded", in.Omissions)
	}
	if in.ComparisonID != "inventory" || in.Limits != Defaults {
		t.Fatal("request does not state its identity and bounds")
	}
}

func TestInputBudgetsWithholdRatherThanTruncate(t *testing.T) {
	b := &builder{}
	first := b.unit("a.go", strings.Repeat("a", 100), inventory.TextHunk)
	second := b.unit("b.go", strings.Repeat("b", 100), inventory.TextHunk)
	third := b.unit("c.go", strings.Repeat("c", 100), inventory.TextHunk)
	inv := b.build()

	units := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Limits{Units: 1})
	if len(units.Units) != 1 || !sent(units, first) {
		t.Fatal("unit limit not applied in inventory order")
	}
	if withheldFor(units, "b.go") != "request unit limit reached" || withheldFor(units, "c.go") == "" {
		t.Fatal("units past the limit are not reported")
	}
	perUnit := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Limits{UnitBytes: 50})
	if len(perUnit.Units) != 0 || withheldFor(perUnit, "a.go") != "unit exceeds request limit" {
		t.Fatal("oversize unit silently sent or unexplained")
	}
	aggregate := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Limits{Bytes: 150})
	if len(aggregate.Units) != 1 || !sent(aggregate, first) || sent(aggregate, second) || sent(aggregate, third) {
		t.Fatal("aggregate byte budget not enforced")
	}
	if withheldFor(aggregate, "b.go") != "request byte budget exhausted" {
		t.Fatal("exhausted budget not stated as scope")
	}
	for _, u := range aggregate.Units {
		if len(u.Patch) != 100 {
			t.Fatal("budget truncated a unit instead of withholding it")
		}
	}
	evidence := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{{EvidenceID: "e1", Path: []byte("README.md"), Excerpt: bytes.Repeat([]byte("r"), 100)}}}
	tight := InputFrom(inv, evidence, privacy.Policy{}, Limits{Bytes: 300})
	if len(tight.Evidence) != 0 || len(tight.Omissions) != 1 || tight.Omissions[0].Reason != "guide request byte budget exhausted" {
		t.Fatal("evidence ignored the shared request budget")
	}
}

func TestInputDigestTracksMaterial(t *testing.T) {
	b := &builder{}
	b.unit("a.go", "@@ -1 +1 @@\n+one\n", inventory.TextHunk)
	inv := b.build()
	base := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults)
	if base.Digest == "" || base.Digest != InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults).Digest {
		t.Fatal("digest is not a stable identity of the same material")
	}
	other := &builder{}
	other.unit("a.go", "@@ -1 +1 @@\n+two\n", inventory.TextHunk)
	if InputFrom(other.build(), reviewcontext.ContextBundle{}, privacy.Policy{}, Defaults).Digest == base.Digest {
		t.Fatal("changed patch content kept the same digest")
	}
	c := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{{EvidenceID: "e1", Path: []byte("README.md"), Excerpt: []byte("readme\n")}}}
	if InputFrom(inv, c, privacy.Policy{}, Defaults).Digest == base.Digest {
		t.Fatal("added evidence kept the same digest")
	}
	narrow := Defaults
	narrow.Bytes = Defaults.Bytes / 2
	if InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, narrow).Digest == base.Digest {
		t.Fatal("different bounds kept the same digest")
	}
}

func TestInputWithholdsCredentialPatchesAndEvidence(t *testing.T) {
	for name, content := range map[string]string{
		"JSON":           `{"api_key": "synthetic-value"}`,
		"YAML":           `'access_token': 'synthetic-value'`,
		"assignment":     `clientSecret := "synthetic-value"`,
		"private key":    "-----BEGIN OPENSSH PRIVATE KEY-----",
		"token material": `value = "ghp_012345678901234567890123456789012345"`,
	} {
		t.Run(name, func(t *testing.T) {
			b := &builder{}
			blocked := b.unit("config.json", "@@ -0,0 +1 @@\n+"+content+"\n", inventory.TextHunk)
			allowed := b.unit("main.go", "@@ -0,0 +1 @@\n+func main() {}\n", inventory.TextHunk)
			c := reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{
				{EvidenceID: "blocked", Path: []byte("settings.yml"), Excerpt: []byte(content)},
				{EvidenceID: "allowed", Path: []byte("README.md"), Excerpt: []byte("Configure your API key.")},
			}}
			in := InputFrom(b.build(), c, privacy.Policy{}, Defaults)
			if sent(in, blocked) || !sent(in, allowed) || strings.Contains(material(in), content) {
				t.Fatal("request did not withhold only credential material")
			}
			if len(in.Evidence) != 1 || in.Evidence[0].EvidenceID != "allowed" {
				t.Fatal("evidence not filtered at upload boundary")
			}
			if withheldFor(in, "config.json") != "credential-like content" || len(in.Omissions) != 1 || in.Omissions[0].Reason != "credential-like content" {
				t.Fatal("withheld content not recorded")
			}
		})
	}
}
