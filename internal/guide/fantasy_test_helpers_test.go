package guide

import (
	"context"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
)

const testKey = "sk-test-secret-value"

func twoUnits(t *testing.T) inventory.Inventory {
	t.Helper()
	b := &builder{}
	b.unit("internal/http/login.go", "@@ -1,2 +1,3 @@\n func Login() {\n+\treturn nil\n }\n", inventory.TextHunk)
	b.unit("README.md", "@@ -1 +1,2 @@\n title\n+line\n", inventory.TextHunk)
	return b.build()
}

func run(t *testing.T, a Analyzer, inv inventory.Inventory, limits Limits) Bundle {
	t.Helper()
	in := InputFrom(inv, reviewcontext.ContextBundle{}, privacy.Policy{}, limits)
	return Analyze(context.Background(), a, inv, in)
}
