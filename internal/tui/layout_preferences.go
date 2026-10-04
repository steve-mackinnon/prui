package tui

import "prui/internal/layoutprefs"

// SetLayoutPreferences is wired only for interactive invocations. Tests and
// plain output do not consult or write the user's configuration.
func (m *Model) SetLayoutPreferences(p layoutprefs.Preferences, save func(layoutprefs.Preferences) error) {
	m.layoutPreferences = p
	m.saveLayoutPreferences = save
	m.applyLayoutPreferences(m.reviewTabState)
}
func (m *Model) applyLayoutPreferences(s *reviewTabState) {
	if s == nil {
		return
	}
	p := m.layoutPreferences
	s.listWidthPreference = p.RailWidth
	s.commit.width = p.CommitWidth
	s.groupFiles = p.GroupFiles
	s.collapseGenerated = p.CollapseGenerated
	if p.Split {
		s.layout = diffLayoutSideBySide
	} else {
		s.layout = diffLayoutUnified
	}
}
func (m *Model) saveLayout() {
	p := layoutprefs.Preferences{Version: 1, Split: m.layout == diffLayoutSideBySide, RailWidth: m.listWidthPreference, CommitWidth: m.commit.width, GroupFiles: m.groupFiles, CollapseGenerated: m.collapseGenerated}
	m.layoutPreferences = p
	if m.saveLayoutPreferences != nil {
		if err := m.saveLayoutPreferences(p); err != nil {
			m.ActionError = err
		}
	}
}
