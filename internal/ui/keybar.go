package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// keyHint is one "key Label" pair in the bottom key bar.
type keyHint struct{ key, label string }

var (
	sidebarHints = []keyHint{
		{"f", "Fetch"}, {"p", "Pull"}, {"P", "Push"}, {"c", "Checkout"}, {"n", "Branch"},
		{"N", "New wt"}, {"D", "Remove"}, {"b", "Base"}, {"s", "Shell"}, {"e", "Edit"},
		{"/", "Filter"}, {":", "More"},
	}
	filesHints = []keyHint{
		{"↵", "Diff"}, {"1", "Changes"}, {"2", "vs Base"}, {"3", "Log"}, {"w", "Whitespace"},
		{"r", "Refresh"}, {"e", "Edit"}, {"y", "Copy path"}, {":", "More"},
	}
	diffHints = []keyHint{
		{"j/k", "Scroll"}, {"d/u", "Page"}, {"g/G", "Top/End"}, {"w", "Whitespace"},
		{"↵", "Back"}, {":", "More"},
	}
	promptHints  = []keyHint{{"↵", "Confirm"}, {"esc", "Cancel"}, {"↑/↓", "Choose"}}
	confirmHints = []keyHint{{"y", "Yes"}, {"n", "No"}}
	helpHints    = []keyHint{{"?", "Close"}}
	filterHints  = []keyHint{{"↵", "Keep"}, {"esc", "Clear"}, {"↑/↓", "Move"}}
	opHints      = []keyHint{{"ctrl+c", "Cancel"}, {"o", "Output"}}
	alwaysHints  = []keyHint{{"?", "Help"}, {"q", "Quit"}}
)

// hints picks the key hints for the current UI state.
func (a App) hints() []keyHint {
	switch {
	case a.showHelp:
		return helpHints
	case a.prompt.active && a.prompt.confirm:
		return confirmHints
	case a.prompt.active:
		return promptHints
	case a.sidebar.filtering:
		return filterHints
	}
	var hs []keyHint
	if a.op != nil && a.op.running {
		hs = append(hs, opHints...)
	}
	switch a.focus {
	case paneSidebar:
		hs = append(hs, sidebarHints...)
	case paneFiles:
		hs = append(hs, filesHints...)
	case paneDiff:
		hs = append(hs, diffHints...)
	}
	return hs
}

// keyBar renders the hints for the current state, dropping items from the
// right when the terminal is too narrow. Help and quit are always kept at
// the far right.
func (a App) keyBar() string {
	t := a.theme
	render := func(h keyHint) string { return t.Key.Render(h.key) + " " + t.Dim.Render(h.label) }
	right := make([]string, 0, len(alwaysHints))
	for _, h := range alwaysHints {
		right = append(right, render(h))
	}
	rightStr := strings.Join(right, "  ")
	avail := a.width - lipgloss.Width(rightStr) - 2
	var parts []string
	used := 0
	for _, h := range a.hints() {
		s := render(h)
		w := lipgloss.Width(s)
		if len(parts) > 0 {
			w += 2
		}
		if used+w > avail {
			break
		}
		parts = append(parts, s)
		used += w
	}
	left := strings.Join(parts, "  ")
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(rightStr)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + rightStr
}
