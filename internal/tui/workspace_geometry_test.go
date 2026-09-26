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
	if g.Rail != image.Rect(0, 3, 33, 22) || g.Divider != image.Rect(33, 3, 36, 22) || g.Detail != image.Rect(36, 3, 100, 22) {
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
