package main

import (
	"context"
	"strings"
	"testing"

	"prui/internal/session"
	"prui/internal/source"
)

func TestDraftReconciliationOfflineRefusesBeforeGitHubAccess(t *testing.T) {
	app := application{offline: true}
	if _, err := app.reconcileDraft(context.Background(), source.Metadata{}, session.Draft{}); err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatal("offline outcome read allowed", err)
	}
}
