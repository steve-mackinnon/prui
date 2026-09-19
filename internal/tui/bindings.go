package tui

import "strings"

type bindingGroup uint8

const (
	groupNav bindingGroup = 1 << iota
	groupFooter
	groupHelp
)

type binding struct {
	keys   string
	desc   string
	groups bindingGroup
}

var bindings = []binding{
	{keys: "ctrl+p / p", desc: "open the pull-request switcher", groups: groupFooter | groupHelp},
	{keys: "j/k, up/down", desc: "move selection or scroll diff", groups: groupNav | groupHelp},
	{keys: "J/K", desc: "scroll diff by 5", groups: groupNav | groupHelp},
	{keys: "ctrl+h/ctrl+l", desc: "focus list/diff", groups: groupNav | groupFooter | groupHelp},
	{keys: "enter", desc: "focus the diff; on a file, jump to its place in the guide diff", groups: groupNav | groupHelp},
	{keys: "tab", desc: "expand/collapse the selected guide or section", groups: groupNav | groupHelp},
	{keys: "esc", desc: "back", groups: groupNav | groupFooter | groupHelp},
	{keys: "n", desc: "next guide row, or unit without guides", groups: groupNav | groupFooter | groupHelp},
	{keys: "[/]", desc: "next/previous guide, or file slice without guides", groups: groupNav | groupHelp},
	{keys: "pgup/pgdown", desc: "page through diff", groups: groupNav | groupHelp},
	{keys: "h/l, left/right", desc: "horizontal scroll", groups: groupNav | groupHelp},
	{keys: "home", desc: "reset selected guide scroll, or selected unit's without guides", groups: groupNav | groupHelp},
	{keys: "i", desc: "toggle full inventory; every raw unit, unfiltered by guides", groups: groupFooter | groupHelp},
	{keys: "G", desc: "guides or deterministic file plan", groups: groupFooter | groupHelp},
	{keys: "e", desc: "show evidence scope", groups: groupFooter | groupHelp},
	{keys: "a", desc: "show accepted plan", groups: groupFooter | groupHelp},
	{keys: "v", desc: "move selected unit", groups: groupFooter | groupHelp},
	{keys: "o", desc: "reorder slices", groups: groupFooter | groupHelp},
	{keys: "g", desc: "generate an OpenAI guide after confirmation", groups: groupFooter | groupHelp},
	{keys: "u", desc: "show GitHub URL", groups: groupFooter | groupHelp},
	{keys: "m", desc: "mark/unmark the whole file slice, including its units under other guides", groups: groupFooter | groupHelp},
	{keys: "r", desc: "refresh GitHub metadata", groups: groupFooter | groupHelp},
	{keys: "N", desc: "new comparison, empty progress", groups: groupFooter | groupHelp},
	{keys: "s", desc: "open saved sessions", groups: groupFooter | groupHelp},
	{keys: "?", desc: "show keyboard help", groups: groupFooter | groupHelp},
	{keys: "q/ctrl+c", desc: "quit", groups: groupFooter | groupHelp},
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
