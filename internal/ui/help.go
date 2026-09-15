package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var helpRows = [][2]string{
	{"j/k ↑/↓", "move"},
	{"g/G", "top / bottom"},
	{"d/u", "half page down / up (diff)"},
	{"tab / shift+tab", "cycle panes"},
	{"h/l ←/→", "previous / next pane"},
	{"enter", "open selection in next pane"},
	{"space / z", "collapse project"},
	{"*", "pin / unpin project to the top"},
	{"1 / 2 / 3 / 4", "Changes / vs Base / Log / Stashes tab"},
	{"space", "stage or unstage file (Changes); pop stash (Stashes)"},
	{"a / A", "stage all / unstage all; apply stash (Stashes)"},
	{"x / X", "discard file / discard all; drop stash (Stashes)"},
	{"C", "commit staged changes"},
	{"t / T", "stash / stash including untracked"},
	{"w", "toggle ignore whitespace"},
	{"b", "set base branch for project"},
	{": / ctrl+p", "command palette"},
	{"f / F", "fetch project / fetch all"},
	{"p / P", "pull (ff-only) / push"},
	{"c / n", "checkout branch / new branch"},
	{"N / D", "new worktree / remove worktree"},
	{"e / s / y", "open editor / shell / copy path"},
	{"o", "toggle output pane"},
	{"ctrl+c", "cancel running operation"},
	{"/", "filter projects (esc clears)"},
	{"r / R", "refresh selected / all"},
	{"?", "toggle this help"},
	{"q", "quit"},
}

func helpView(t Theme, width, height int) string {
	var b strings.Builder
	b.WriteString(t.TitleFocused.Render("pyragit keys") + "\n\n")
	for _, r := range helpRows {
		b.WriteString(t.Key.Render(padRight(r[0], 18)) + r[1] + "\n")
	}
	b.WriteString("\n" + t.Dim.Render("press ? or esc to close"))
	box := t.BorderFocus.Padding(0, 2).Render(b.String())
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
