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
	{"1 / 2 / 3", "Changes / vs Base / Log tab"},
	{"w", "toggle ignore whitespace"},
	{"b", "set base branch for project"},
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
