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
	{keys: "C", desc: "filter Files/Guide by captured commits (one read-only net diff)", groups: groupHelp, section: "Views"},
	{keys: "P", desc: "open the pull-request switcher", groups: groupFooter | groupHelp, section: "App"},
	{keys: "j/k", desc: "move the focused list or diff cursor", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "up/down", desc: "move list selection or scroll continuous diff", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "J/K", desc: "scroll diff by 5", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "zz", desc: "center the focused diff on its line cursor", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "h/l, ctrl+h/ctrl+l", desc: "focus list/diff", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "enter", desc: "focus the diff; open a line editor or selected-comment action menu", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "tab", desc: "expand/collapse the selected guide or section", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "esc", desc: "back", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "n/p", desc: "move comment target in File diff; move files, guide rows, or inventory units in list", groups: groupNav | groupFooter | groupHelp, section: "Navigate"},
	{keys: "{/}", desc: "previous/next guide, or file", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "[/]", desc: "narrow/widen the file and guide pane by 2 columns", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "d/u, pgup/pgdown", desc: "page through diff", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "left/right", desc: "horizontal scroll", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "home", desc: "reset active diff scroll", groups: groupNav | groupHelp, section: "Navigate"},
	{keys: "i", desc: "toggle full inventory; every raw unit, unfiltered by guides", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "F/G", desc: "select Files or Guide", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "S", desc: "toggle side-by-side detail (unified default; falls back below 160 columns)", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "e", desc: "show evidence scope", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "v/V", desc: "next/previous PR context view", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "1/2/3/4", desc: "select Description, Files, Guide, or Commits", groups: groupFooter | groupHelp, section: "Views"},
	{keys: "g", desc: "choose provider and model, then generate a guide", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "U", desc: "show GitHub URL", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "m", desc: "mark/unmark the whole file slice, including its units under other guides", groups: groupFooter | groupHelp, section: "Review"},
	{keys: "R", desc: "submit a PR review with pending line comments", groups: groupHelp, section: "Review"},
	{keys: "enter", desc: "submit inline comment editor", groups: groupHelp, section: "Review"},
	{keys: "ctrl+o", desc: "choose old/new side of split or context rows", groups: groupHelp, section: "Review"},
	{keys: "ctrl+v", desc: "start/cancel range; j/k chooses end, enter composes", groups: groupHelp, section: "Review"},
	{keys: "ctrl+f", desc: "compose a file comment (enter posts; queuing unsupported)", groups: groupHelp, section: "Review"},
	{keys: "ctrl+p", desc: "save inline comment as a local pending draft", groups: groupHelp, section: "Review"},
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
