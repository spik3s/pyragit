package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// frame draws a bordered pane with a title in the top border.
func frame(t Theme, title, content string, width, height int, focused bool) string {
	style := t.Border
	titleStyle := t.Title
	if focused {
		style = t.BorderFocus
		titleStyle = t.TitleFocused
	}
	innerW := max(width-2, 1)
	body := style.Width(width).Height(height).MaxHeight(height).Render(content)
	// Overlay the title on the top border line.
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) == 2 {
		label := " " + ansi.Truncate(title, max(innerW-2, 1), "…") + " "
		top := lines[0]
		// Replace border characters starting at column 2 with the label.
		prefix := ansi.Truncate(top, 2, "")
		rest := ansi.TruncateLeft(top, 2+ansi.StringWidth(label), "")
		lines[0] = prefix + titleStyle.Render(label) + rest
		body = lines[0] + "\n" + lines[1]
	}
	return body
}

func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

func short(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// smartTime renders a time compactly: clock time if today, month/day and
// clock time if this year, otherwise the date. Zero times render empty.
func smartTime(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("Jan 2 15:04")
	}
	return t.Format("2006-01-02")
}

// absTime renders a full timestamp for the status bar.
func absTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// ago renders a relative age as a phrase: "just now", "3h ago".
func ago(t time.Time) string {
	r := strings.TrimSpace(relTime(t))
	if r == "now" {
		return "just now"
	}
	return r + " ago"
}

// relTime renders a compact relative age like "3h", "2d", "5w".
func relTime(t time.Time) string {
	if t.IsZero() {
		return "   ?"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return " now"
	case d < time.Hour:
		return fmt.Sprintf("%3dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%3dh", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%3dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%3dw", int(d.Hours()/24/7))
	}
	return fmt.Sprintf("%3dy", int(d.Hours()/24/365))
}

var _ = lipgloss.Left
