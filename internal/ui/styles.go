package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds the styles used by every pane. It adapts to the terminal's
// background once a tea.BackgroundColorMsg arrives.
type Theme struct {
	Accent, Muted, Good, Warn, Bad, Info color.Color

	Title        lipgloss.Style
	TitleFocused lipgloss.Style
	Border       lipgloss.Style
	BorderFocus  lipgloss.Style
	Selected     lipgloss.Style
	SelectedDim  lipgloss.Style
	Dim          lipgloss.Style
	StatusBar    lipgloss.Style
	Key          lipgloss.Style
	Tab          lipgloss.Style
	TabActive    lipgloss.Style
	DiffAdd      lipgloss.Style
	DiffDel      lipgloss.Style
	DiffHunk     lipgloss.Style
	DiffMeta     lipgloss.Style
	BadgeDirty   lipgloss.Style
	BadgeAhead   lipgloss.Style
	BadgeBehind  lipgloss.Style
	BadgeWarn    lipgloss.Style
	BadgeStale   lipgloss.Style
	KeyBarKey    lipgloss.Style
	KeyBarLabel  lipgloss.Style
}

// NewTheme builds a theme for a dark or light background.
func NewTheme(dark bool) Theme {
	ld := lipgloss.LightDark(dark)
	c := func(light, darkc string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(darkc)) }
	t := Theme{
		Accent: c("#5f5fd7", "#8787ff"),
		Muted:  c("#8a8a8a", "#6c6c6c"),
		Good:   c("#008700", "#5fd75f"),
		Warn:   c("#af8700", "#ffd75f"),
		Bad:    c("#d70000", "#ff5f5f"),
		Info:   c("#0087af", "#5fd7ff"),
	}
	t.Title = lipgloss.NewStyle().Bold(true).Foreground(t.Muted)
	t.TitleFocused = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	t.Border = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Muted)
	t.BorderFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Accent)
	t.Selected = lipgloss.NewStyle().Reverse(true).Bold(true)
	t.SelectedDim = lipgloss.NewStyle().Reverse(true).Faint(true)
	t.Dim = lipgloss.NewStyle().Foreground(t.Muted)
	t.StatusBar = lipgloss.NewStyle().Foreground(t.Muted)
	t.Key = lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
	t.Tab = lipgloss.NewStyle().Foreground(t.Muted)
	t.TabActive = lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Underline(true)
	t.DiffAdd = lipgloss.NewStyle().Foreground(t.Good)
	t.DiffDel = lipgloss.NewStyle().Foreground(t.Bad)
	t.DiffHunk = lipgloss.NewStyle().Foreground(t.Info)
	t.DiffMeta = lipgloss.NewStyle().Bold(true).Foreground(t.Muted)
	t.BadgeDirty = lipgloss.NewStyle().Foreground(t.Warn)
	t.BadgeAhead = lipgloss.NewStyle().Foreground(t.Good)
	t.BadgeBehind = lipgloss.NewStyle().Foreground(t.Info)
	t.BadgeWarn = lipgloss.NewStyle().Foreground(t.Bad).Bold(true)
	t.BadgeStale = lipgloss.NewStyle().Foreground(t.Muted)
	// Norton Commander style: bare key, then the label on a coloured block.
	t.KeyBarKey = lipgloss.NewStyle().Bold(true)
	t.KeyBarLabel = lipgloss.NewStyle().
		Background(c("#5f5fd7", "#5f5faf")).
		Foreground(c("#ffffff", "#eeeeee"))
	return t
}
