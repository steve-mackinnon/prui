package tui

import (
	"context"
	"strings"
	"testing"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
	"pr-review/internal/review"
)

func TestGuidePreparationWaitsForConsentAndInspectorCannotSend(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.openReviewTab(saved)
	m.SetLifecycle(store, nil, nil)
	prepared := &guide.Preparation{UnavailableReason: "Pinned search unavailable; saved evidence only"}
	sent := false
	m.SetGuidePreparation(func(context.Context, *review.Session) (*guide.Preparation, error) { return prepared, nil })
	m.SetPreparedGuideLifecycle(func(_ context.Context, _ *review.Session, p *guide.Preparation) (*review.Session, error) {
		sent = true
		if p != prepared {
			t.Error("different approved preparation")
		}
		return nil, nil
	})
	cmd, _ := m.lifecycleKey("g")
	if cmd == nil || !m.Busy || sent {
		t.Fatal("preparation must run locally before consent")
	}
	m.Update(cmd())
	if m.top() != pageGuideConsent || sent {
		t.Fatal("did not stop for consent")
	}
	if !strings.Contains(m.guideConsentView(), "saved evidence only") {
		t.Fatal("fallback reason missing")
	}
	m.guideConsentKey("d")
	if m.guideConsentKey("enter") != nil || sent {
		t.Fatal("inspector enter sent source")
	}
	m.guideConsentKey("esc")
	cmd = m.guideConsentKey("enter")
	if cmd == nil {
		t.Fatal("confirmation did not start")
	}
	m.Update(cmd())
	if !sent || m.guidePreparation != nil {
		t.Fatal("approved preparation not transferred")
	}
}

func TestGuidePreparationCancellationDiscardsLateResult(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.openReviewTab(saved)
	m.SetLifecycle(store, nil, nil)
	m.SetGuidePreparation(func(context.Context, *review.Session) (*guide.Preparation, error) { return &guide.Preparation{}, nil })
	cmd, _ := m.lifecycleKey("g")
	m.cancelCurrentAction()
	m.Update(cmd())
	if m.top() == pageGuideConsent || m.guidePreparation != nil {
		t.Fatal("cancelled preparation opened consent")
	}
}

func TestGuideInspectionEscapesSourceAndExclusionRemovesBothRevisions(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	corpus := &reviewcontext.SearchCorpus{Files: []reviewcontext.SearchFile{
		{Revision: "head", Evidence: reviewcontext.Evidence{Path: []byte("sample.go"), CommitSHA: strings.Repeat("a", 40), Excerpt: []byte("safe\n\x1b]52;c;injected\a\n" + strings.Repeat("more\n", 50))}},
		{Revision: "merge_base", Evidence: reviewcontext.Evidence{Path: []byte("sample.go"), Excerpt: []byte("old")}},
	}}
	p := guide.NewPreparation(inventory.Inventory{}, reviewcontext.ContextBundle{}, corpus, privacy.Policy{})
	m.guidePreparation = p
	m.push(pageGuideConsent)
	m.guideConsentKey("v")
	view := m.guideConsentView()
	if strings.Contains(view, "\x1b") || !strings.Contains(view, strings.Repeat("a", 40)) {
		t.Fatal("inspector did not escape source/show commit")
	}
	m.guideConsentKey("end")
	if !strings.Contains(m.guideConsentView(), "esc: back") {
		t.Fatal("inspector lost back instruction")
	}
	m.guideConsentKey("enter")
	if m.guidePreparation != p || m.Busy {
		t.Fatal("inspector Enter submitted")
	}
	m.guideConsentKey("esc")
	m.guideConsentKey("x")
	if len(p.Files()) != 0 {
		t.Fatal("exclusion left a revision eligible")
	}
	m.guideConsentKey("esc")
	if m.guidePreparation != nil {
		t.Fatal("cancel retained source")
	}
}

func TestGuidePreparationTabSwitchDiscardsPendingAndApprovedSource(t *testing.T) {
	store, saved := programStore(t)
	m := newModel(context.Background())
	defer m.Close()
	m.openReviewTab(saved)
	m.SetLifecycle(store, nil, nil)
	other := *saved
	other.ID = "other"
	other.Inventory.Comparison.Metadata.Identity.Number++
	m.openReviewTab(&other)
	m.activateTab(0)
	m.SetGuidePreparation(func(context.Context, *review.Session) (*guide.Preparation, error) { return &guide.Preparation{}, nil })
	cmd, _ := m.lifecycleKey("g")
	m.activateTab(1)
	m.Update(cmd())
	if m.Session.ID != "other" || m.guidePreparation != nil || m.Busy {
		t.Fatal("late preparation retargeted active review")
	}
	m.activateTab(0)
	if m.Busy {
		t.Fatal("origin remained busy after cancelled preparation")
	}
	m.guidePreparation = &guide.Preparation{}
	m.push(pageGuideConsent)
	m.activateTab(1)
	m.activateTab(0)
	if m.guidePreparation != nil || m.top() == pageGuideConsent {
		t.Fatal("tab switch retained stale consent")
	}
}

func TestGuideOmissionsInspectableWithoutSending(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	p := guide.NewPreparation(inventory.Inventory{}, reviewcontext.ContextBundle{OmittedPaths: []reviewcontext.Omitted{{Path: []byte("private\x1b.go"), Reason: "credential-like content"}}}, nil, privacy.Policy{})
	m.guidePreparation = p
	m.push(pageGuideConsent)
	if !strings.Contains(m.guideConsentView(), "Omitted:") {
		t.Fatal("missing omitted scope count")
	}
	m.guideConsentKey("o")
	view := m.guideConsentView()
	if !strings.Contains(view, "credential-like content") || strings.Contains(view, "\x1b") {
		t.Fatal("omission details missing or unescaped")
	}
	m.guideConsentKey("enter")
	if m.guidePreparation != p || m.Busy {
		t.Fatal("omission inspection sent source")
	}
}

func TestEvidenceInspectorShowsEscapedBodiesAndScrolls(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Session = &review.Session{}
	m.Session.Context.Evidence = []reviewcontext.Evidence{{Path: []byte("sample.go"), Excerpt: []byte("first excerpt\n\x1b]52;c;secret\a\n" + strings.Repeat("middle\n", 30) + "last excerpt")}}
	m.Stack = []page{pageReview, pageEvidence}
	view := m.evidenceView()
	if !strings.Contains(view, "first excerpt") || strings.Contains(view, "\x1b]52") {
		t.Fatal("evidence body missing or unsafe")
	}
	m.evidenceKey("end")
	if !strings.Contains(m.evidenceView(), "last excerpt") {
		t.Fatal("cannot reach end of evidence")
	}
	m.evidenceKey("esc")
	if m.top() != pageReview {
		t.Fatal("cannot leave evidence")
	}
}

func TestSearchEvidenceUsesSubmittedLedgerInsteadOfSavedSelection(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Session = &review.Session{}
	m.Session.Context.Evidence = []reviewcontext.Evidence{{Excerpt: []byte("old fixed evidence")}}
	m.Session.Guides = &guide.Bundle{RetrievalVersion: guide.SearchVersion, RetrievedEvidence: []reviewcontext.Evidence{{Excerpt: []byte("uploaded tool result")}}}
	view := m.evidenceView()
	if !strings.Contains(view, "Search evidence sent") || !strings.Contains(view, "uploaded tool result") || strings.Contains(view, "old fixed evidence") {
		t.Fatal("search evidence inspector does not use submitted ledger")
	}
}

func TestSearchEvidencePreservesIncompleteScope(t *testing.T) {
	m := newModel(context.Background())
	defer m.Close()
	m.Height = 40
	m.Session = &review.Session{}
	m.Session.Guides = &guide.Bundle{RetrievalVersion: guide.SearchVersion, RetrievalIncomplete: true, RetrievalOmissions: []reviewcontext.Omitted{{Path: []byte("unchanged.go"), Reason: "blob limit"}}}
	view := m.evidenceView()
	if !strings.Contains(view, "no match does not prove absence") || !strings.Contains(view, "unchanged.go: blob limit") {
		t.Fatal("search scope warning lost after generation")
	}
}
