package tui

import (
	"context"
	"image"
	"testing"
)

func TestWorkspaceGeometryBounds(t *testing.T) {
	m := New(context.Background(), nil)
	defer m.Close()
	m.Session = largeSession(2, 2)
	m.Width, m.Height = 100, 24
	g := m.workspaceGeometry()
	if g.Rail != image.Rect(1, 3, 34, 21) || g.Divider != image.Rect(34, 3, 35, 21) || g.Detail != image.Rect(35, 3, 99, 21) {
		t.Fatalf("geometry: %+v", g)
	}
	m.listWidthPreference = 1000
	if m.listWidth() != 57 || m.detailWidth() != 40 {
		t.Fatalf("clamp: %d %d", m.listWidth(), m.detailWidth())
	}
	m.Width = 99
	if !m.workspaceGeometry().Divider.Empty() {
		t.Fatal("narrow divider")
	}
	m.Height = 2
	if !m.workspaceGeometry().Rail.Empty() || !m.workspaceGeometry().Detail.Empty() {
		t.Fatal("hidden body hit regions")
	}
}
