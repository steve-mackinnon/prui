package guide

import (
	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"testing"
)

func TestPreparationExclusionBindsConsentAndRenameAliases(t *testing.T) {
	b := &builder{}
	id := b.unit("new.go", "change\n", inventory.TextHunk)
	inv := b.build()
	inv.Files[0].OldPath = []byte("old.go")
	c := &reviewcontext.SearchCorpus{Files: []reviewcontext.SearchFile{
		{Revision: "merge_base", Evidence: reviewcontext.Evidence{Path: []byte("old.go"), Excerpt: []byte("old\n")}},
		{Revision: "head", Evidence: reviewcontext.Evidence{Path: []byte("new.go"), Excerpt: []byte("new\n")}},
	}}
	p := NewPreparation(inv, reviewcontext.ContextBundle{}, c, privacy.Policy{})
	if !sent(p.Preview(), id) {
		t.Fatal("eligible rename withheld")
	}
	p.Confirm()
	if err := p.ValidateApproval(); err != nil {
		t.Fatal(err)
	}
	if err := p.Exclude("old.go"); err != nil {
		t.Fatal(err)
	}
	if p.ValidateApproval() == nil {
		t.Fatal("changed scope retained consent")
	}
	if sent(p.Preview(), id) || len(p.Files()) != 0 {
		t.Fatal("rename exclusion did not close aliases")
	}
	if len(c.Files) != 2 {
		t.Fatal("preparation mutated caller corpus")
	}
}

func TestPreparationWithholdsUnscannedChangedFiles(t *testing.T) {
	b := &builder{}
	id := b.unit("large.go", "innocent hunk", inventory.TextHunk)
	p := NewPreparation(b.build(), reviewcontext.ContextBundle{}, &reviewcontext.SearchCorpus{Incomplete: true}, privacy.Policy{})
	if sent(p.Preview(), id) {
		t.Fatal("unscanned changed file uploaded")
	}
}

func TestPreparationFallbackExclusionsAndRecipientBinding(t *testing.T) {
	b := &builder{}
	id := b.unit("a.go", "change", inventory.TextHunk)
	p := NewPreparation(b.build(), reviewcontext.ContextBundle{Evidence: []reviewcontext.Evidence{{Path: []byte("a.go"), Excerpt: []byte("source")}}}, nil, privacy.Policy{})
	if p.Preview().Search != nil || len(p.Files()) != 1 {
		t.Fatal("fallback not inspectable")
	}
	p.Confirm()
	p.Recipient = "https://different.invalid"
	if p.ValidateApproval() == nil {
		t.Fatal("changed recipient kept approval")
	}
	if err := p.Exclude("a.go"); err != nil {
		t.Fatal(err)
	}
	if sent(p.Preview(), id) || len(p.Preview().Evidence) != 0 {
		t.Fatal("fallback exclusion ignored")
	}
}

func TestPreparationDeniedAdditionDoesNotExcludeOtherAddedFiles(t *testing.T) {
	b := &builder{}
	secret := b.unit("bad.go", "api_key = secret", inventory.TextHunk)
	safe := b.unit("good.go", "ordinary", inventory.TextHunk)
	inv := b.build()
	for i := range inv.Files {
		inv.Files[i].OldPath = nil
	}
	corpus := &reviewcontext.SearchCorpus{Files: []reviewcontext.SearchFile{
		{Revision: "head", Evidence: reviewcontext.Evidence{Path: []byte("bad.go"), Excerpt: []byte("ordinary")}},
		{Revision: "head", Evidence: reviewcontext.Evidence{Path: []byte("good.go"), Excerpt: []byte("ordinary")}},
	}}
	p := NewPreparation(inv, reviewcontext.ContextBundle{}, corpus, privacy.Policy{})
	if sent(p.Preview(), secret) || !sent(p.Preview(), safe) {
		t.Fatal("empty old name contaminated unrelated addition")
	}
}
