package ui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
)

// diffView shows a unified diff with line colouring.
type diffView struct {
	vp       viewport.Model
	width    int
	height   int
	title    string
	raw      string
	ignoreWS bool
	key      diffKey
	loading  bool
}

func newDiffView() diffView {
	return diffView{vp: viewport.New()}
}

func (d *diffView) resize(w, h int) {
	d.width, d.height = w, h
	d.vp.SetWidth(max(w-2, 1))
	d.vp.SetHeight(max(h-2, 1))
}

func (d *diffView) setContent(t Theme, raw string) {
	d.raw = raw
	d.loading = false
	d.vp.SetContent(colourDiff(t, raw, max(d.width-2, 1)))
	d.vp.GotoTop()
}

func (d *diffView) view(t Theme, focused bool) string {
	title := d.title
	if title == "" {
		title = "Diff"
	}
	if d.ignoreWS {
		title += " [-w]"
	}
	if d.loading {
		title += " …"
	}
	return frame(t, title, d.vp.View(), d.width, d.height, focused)
}

// colourDiff applies theme styles per diff line and truncates long lines so
// the viewport never scrolls horizontally.
func colourDiff(t Theme, raw string, width int) string {
	if strings.TrimSpace(raw) == "" {
		return t.Dim.Render("(no diff)")
	}
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		l = strings.ReplaceAll(l, "\t", "    ")
		l = ansi.Truncate(l, width, "…")
		switch {
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"):
			out[i] = t.DiffMeta.Render(l)
		case strings.HasPrefix(l, "+"):
			out[i] = t.DiffAdd.Render(l)
		case strings.HasPrefix(l, "-"):
			out[i] = t.DiffDel.Render(l)
		case strings.HasPrefix(l, "@@"):
			out[i] = t.DiffHunk.Render(l)
		case strings.HasPrefix(l, "diff --git"), strings.HasPrefix(l, "index "),
			strings.HasPrefix(l, "commit "), strings.HasPrefix(l, "Author:"), strings.HasPrefix(l, "Date:"),
			strings.HasPrefix(l, "new file"), strings.HasPrefix(l, "deleted file"), strings.HasPrefix(l, "rename "), strings.HasPrefix(l, "similarity "):
			out[i] = t.DiffMeta.Render(l)
		default:
			out[i] = l
		}
	}
	return strings.Join(out, "\n")
}
