package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/spik3s/pyragit/internal/state"
)

// sidebarRow is one visible line in the tree: a project header or a worktree.
type sidebarRow struct {
	project  *state.Project
	worktree *state.Worktree // nil for project rows
	last     bool            // last worktree row of its project
}

// sidebar renders the project/worktree tree.
type sidebar struct {
	rows      []sidebarRow
	cursor    int
	offset    int
	width     int
	height    int
	filter    string
	filtering bool
	staleDays int
}

func newSidebar(staleDays int) sidebar { return sidebar{staleDays: staleDays} }

// rebuild flattens the store into rows, honouring collapsed projects and the
// filter, and keeps the cursor on the same worktree when possible.
func (s *sidebar) rebuild(store *state.Store) {
	var keep string
	if w := s.selectedWorktree(); w != nil {
		keep = w.Path
	}
	s.rows = s.rows[:0]
	q := strings.ToLower(s.filter)
	// Pinned projects first, keeping the store's alphabetical order within each group.
	ordered := make([]*state.Project, 0, len(store.Projects))
	for _, p := range store.Projects {
		if p.Pinned {
			ordered = append(ordered, p)
		}
	}
	for _, p := range store.Projects {
		if !p.Pinned {
			ordered = append(ordered, p)
		}
	}
	for _, p := range ordered {
		var wts []*state.Worktree
		for _, w := range p.Worktrees {
			if q == "" || strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(strings.ToLower(w.Branch), q) {
				wts = append(wts, w)
			}
		}
		if len(wts) == 0 {
			continue
		}
		s.rows = append(s.rows, sidebarRow{project: p})
		if p.Collapsed && q == "" {
			continue
		}
		for i, w := range wts {
			s.rows = append(s.rows, sidebarRow{project: p, worktree: w, last: i == len(wts)-1})
		}
	}
	s.cursor = 0
	for i, r := range s.rows {
		if r.worktree != nil && r.worktree.Path == keep {
			s.cursor = i
			break
		}
	}
	if keep == "" {
		s.selectFirstWorktree()
	}
	s.clamp()
}

func (s *sidebar) selectFirstWorktree() {
	for i, r := range s.rows {
		if r.worktree != nil {
			s.cursor = i
			return
		}
	}
}

func (s *sidebar) selectedWorktree() *state.Worktree {
	if s.cursor >= 0 && s.cursor < len(s.rows) {
		return s.rows[s.cursor].worktree
	}
	return nil
}

func (s *sidebar) selectedProject() *state.Project {
	if s.cursor >= 0 && s.cursor < len(s.rows) {
		return s.rows[s.cursor].project
	}
	return nil
}

func (s *sidebar) move(delta int) {
	s.cursor += delta
	s.clamp()
}

func (s *sidebar) clamp() {
	if len(s.rows) == 0 {
		s.cursor, s.offset = 0, 0
		return
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.rows) {
		s.cursor = len(s.rows) - 1
	}
	inner := s.innerHeight()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+inner {
		s.offset = s.cursor - inner + 1
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// innerHeight is the number of rows visible inside the border and title.
func (s *sidebar) innerHeight() int {
	h := s.height - 2 // border
	if s.filtering || s.filter != "" {
		h--
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (s *sidebar) view(t Theme, focused bool, now time.Time) string {
	inner := s.innerHeight()
	w := s.width - 2
	if w < 4 {
		w = 4
	}
	lines := make([]string, 0, inner)
	for i := s.offset; i < len(s.rows) && len(lines) < inner; i++ {
		lines = append(lines, s.renderRow(t, s.rows[i], i == s.cursor, focused, w, now))
	}
	for len(lines) < inner {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if s.filtering || s.filter != "" {
		f := "/" + s.filter
		if s.filtering {
			f += "▏"
		}
		lines = append([]string{padRight(t.Key.Render(f), w)}, lines...)
	}
	return frame(t, "Projects", strings.Join(lines, "\n"), s.width, s.height, focused)
}

func (s *sidebar) renderRow(t Theme, r sidebarRow, selected, focused bool, w int, now time.Time) string {
	var text string
	if r.worktree == nil {
		arrow := "▾"
		if r.project.Collapsed {
			arrow = "▸"
		}
		text = fmt.Sprintf("%s %s", arrow, r.project.Name)
		if n := len(r.project.Worktrees); n > 1 {
			text += t.Dim.Render(fmt.Sprintf(" (%d)", n))
		}
		if r.project.Pinned {
			text += " " + t.BadgeDirty.Render("★")
		}
		text = padRight(ansi.Truncate(text, w, "…"), w)
		if selected {
			if focused {
				return t.Selected.Render(text)
			}
			return t.SelectedDim.Render(text)
		}
		return t.Title.Render(text)
	}
	wt := r.worktree
	name := wt.Branch
	if name == "" {
		if wt.Detached {
			name = "detached " + short(wt.Head)
		} else {
			name = "(unknown)"
		}
	}
	badges := s.badges(t, wt, now)
	if wt.Loaded && wt.Snap.Err == nil {
		if age := relTime(wt.Snap.LastActivity()); strings.TrimSpace(age) != "?" {
			if badges != "" {
				badges += " "
			}
			badges += t.Dim.Render(strings.TrimSpace(age))
		}
	}
	// ◆ marks the main worktree; linked worktrees hang off it as a tree.
	prefix := "  ├ "
	switch {
	case wt.IsMain():
		prefix = "  ◆ "
	case r.last:
		prefix = "  └ "
	}
	bw := ansi.StringWidth(badges)
	nameW := w - len([]rune(prefix)) - bw
	if bw > 0 {
		nameW--
	}
	if nameW < 3 {
		nameW = 3
	}
	label := prefix + padRight(ansi.Truncate(name, nameW, "…"), nameW)
	if bw > 0 {
		label += " " + badges
	}
	label = padRight(label, w)
	if selected {
		if focused {
			return t.Selected.Render(ansi.Strip(label))
		}
		return t.SelectedDim.Render(ansi.Strip(label))
	}
	return label
}

// badges builds the compact status indicators for a worktree row.
func (s *sidebar) badges(t Theme, wt *state.Worktree, now time.Time) string {
	if wt.Loading && !wt.Loaded {
		return t.Dim.Render("…")
	}
	if !wt.Loaded {
		return ""
	}
	snap := wt.Snap
	if snap.Err != nil {
		return t.BadgeWarn.Render("✗")
	}
	var parts []string
	st := snap.Status
	if n := len(st.Files); n > 0 {
		parts = append(parts, t.BadgeDirty.Render(fmt.Sprintf("●%d", n)))
	}
	if st.ConflictCount() > 0 {
		parts = append(parts, t.BadgeWarn.Render("⚠"))
	}
	if st.Ahead > 0 {
		parts = append(parts, t.BadgeAhead.Render(fmt.Sprintf("↑%d", st.Ahead)))
	}
	if st.Behind > 0 {
		parts = append(parts, t.BadgeBehind.Render(fmt.Sprintf("↓%d", st.Behind)))
	}
	if !st.HasUpstream() && !st.Detached {
		parts = append(parts, t.BadgeWarn.Render("!"))
	}
	if s.staleDays > 0 && !snap.LastCommit.IsZero() && now.Sub(snap.LastCommit) > time.Duration(s.staleDays)*24*time.Hour && !st.Dirty() && !wt.IsMain() {
		parts = append(parts, t.BadgeStale.Render("zz"))
	}
	return strings.Join(parts, " ")
}
