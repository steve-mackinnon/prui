package tui

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"time"
)

const mouseDoubleClickInterval = 500 * time.Millisecond

type mouseActivation struct {
	target string
	at     time.Time
}

// Semantic identity, rather than screen coordinates, prevents a refreshed or
// scrolled row from inheriting the previous row's activation.
func (m *Model) mouseActivate(target string) bool { return m.mouseActivateAt(target, time.Now()) }
func (m *Model) mouseActivateAt(target string, now time.Time) bool {
	m.mouseClickSerial++
	target = fmt.Sprintf("%d/%d/%d/%s", m.top(), m.activeTab, m.selectedReviewView(), target)
	previous := m.mouseActivation
	m.mouseActivation = mouseActivation{target, now}
	if previous.target == target && !now.Before(previous.at) && now.Sub(previous.at) <= mouseDoubleClickInterval {
		m.mouseActivation = mouseActivation{}
		return true
	}
	return false
}
func (m *Model) resetMouseActivation() { m.mouseActivation = mouseActivation{} }

// Route through Update so mouse activation has exactly the keyboard guards.
func (m *Model) mouseKey(key string) tea.Cmd {
	msg := mouseKey(key)
	_, cmd := m.Update(msg)
	return cmd
}
