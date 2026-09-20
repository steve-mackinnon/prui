package tui

import "strings"

type bindingGroup uint8

const (
	groupNav bindingGroup = 1 << iota
	groupFooter
	groupHelp
)

type binding struct {
	keys    string
	desc    string
	groups  bindingGroup
	section string
}

var bindings = []binding{
	{keys: "ctrl+p", desc: "open the pull-request switcher", groups: groupFooter | groupHelp, section: "App"},
	{keys: "j/k, up/down", desc: "move selection or scroll diff", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "J/K", desc: "scroll diff by 5", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "h/l, ctrl+h/ctrl+l", desc: "focus list/diff", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "enter", desc: "focus the diff; open a line editor or selected-comment action menu", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "tab", desc: "expand/collapse the selected guide or section", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "esc", desc: "back", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "n", desc: "next guide row, or unit without guides", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "[/]", desc: "next/previous guide, or file slice without guides", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "d/u, pgup/pgdown", desc: "page through diff", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "left/right", desc: "horizontal scroll", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "home", desc: "reset selected guide scroll, or selected unit's without guides", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "i", desc: "toggle full inventory; every raw unit, unfiltered by guides", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "G", desc: "guides or deterministic file plan", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "e", desc: "show evidence scope", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "g", desc: "generate an OpenAI guide after confirmation", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "U", desc: "show GitHub URL", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "m", desc: "mark/unmark the whole file slice, including its units under other guides", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "enter", desc: "submit inline comment editor", groups: groupHelp, section: "Review"},
	{keys: "shift+enter", desc: "newline in inline comment editor", groups: groupHelp, section: "Review"},
	{keys: "backspace/delete", desc: "delete previous/following rune in comment editor", groups: groupHelp, section: "Review"},
	{keys: "esc", desc: "discard inline comment editor", groups: groupHelp, section: "Review"},
	{keys: "c", desc: "refresh ephemeral inline review comments", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "r/a/d", desc: "reply, react, or delete a selected comment (delete requires ownership and confirmation)", groups: groupHelp, section: "Review"},
	{keys: "r", desc: "refresh GitHub metadata", groups: groupFooter | groupHelp, section: "Diagnostics"},
	{keys: "N", desc: "new comparison, empty progress", groups: groupFooter | groupHelp, section: "Diagnostics"},
	{keys: "s", desc: "open saved sessions", groups: groupFooter | groupHelp, section: "App"},
	{keys: "?", desc: "show keyboard help", groups: groupFooter | groupHelp, section: "App"},
	{keys: "q/ctrl+c", desc: "quit", groups: groupFooter | groupHelp, section: "App"},
}

func renderBindings(group bindingGroup) string {
	lines := make([]string, 0, len(bindings))
	for _, b := range bindings {
		if b.groups&group != 0 {
			lines = append(lines, b.keys+": "+b.desc)
		}
	}
	return strings.Join(lines, "\n")
}

func renderHealth() string {
	sections := []string{"Navigate", "Review", "Views", "Diagnostics", "App"}
	lines := make([]string, 0, len(bindings)+len(sections)*2)
	for _, section := range sections {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, section)
		for _, b := range bindings {
			if b.groups&groupHelp != 0 && b.section == section {
				lines = append(lines, b.keys+": "+b.desc)
			}
		}
	}
	return strings.Join(lines, "\n")
}
