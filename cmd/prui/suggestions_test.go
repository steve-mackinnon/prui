package main

import (
	"context"
	"prui/internal/source"
	"strings"
	"testing"
)

func TestSuggestionOfflineBoundaryBeforeClientAccess(t *testing.T) {
	a := &application{offline: true}
	if _, err := a.prepareSuggestion(context.Background(), source.Metadata{}, source.ReviewComment{}, ""); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
	if _, err := a.applySuggestion(context.Background(), source.SuggestionApplication{}); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal(err)
	}
}
