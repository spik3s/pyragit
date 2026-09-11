package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/spik3s/pyragit/internal/git"
	"github.com/spik3s/pyragit/internal/state"
)

type filesTab int

const (
	tabChanges filesTab = iota
	tabBase
	tabLog
)

func (ft filesTab) String() string {
	switch ft {
	case tabChanges:
		return "Changes"
	case tabBase:
		return "vs Base"
	case tabLog:
		return "Log"
	}
	return "?"
}

// fileEntry is one selectable line in the files pane.
type fileEntry struct {
	header bool // section header, not selectable
	label  string
	path   string
	mode   git.DiffMode // for tabChanges
	status byte
	hash   string    // for tabLog
	when   time.Time // file mtime or commit time; rendered as a right column
	author string    // for tabLog
}

// filesPane lists changed files or commits for the selected worktree.
type filesPane struct {
	tab      filesTab
	entries  []fileEntry
	cursor   int
	offset   int
	width    int
	height   int
	worktree string
	title    string
}

func (f *filesPane) setChanges(wt *state.Worktree) {
	f.worktree = wt.Path
	f.entries = f.entries[:0]
	f.title = ""
	if !wt.Loaded {
		f.entries = append(f.entries, fileEntry{header: true, label: "loading…"})
		f.cursor = 0
		return
	}
	if wt.Snap.Err != nil {
		f.entries = append(f.entries, fileEntry{header: true, label: "error: " + wt.Snap.Err.Error()})
		f.cursor = 0
		return
	}
	st := wt.Snap.Status
	var conflicts, staged, unstaged, untracked []fileEntry
	for _, fs := range st.Files {
		when := wt.Snap.FileTimes[fs.Path]
		switch {
		case fs.Conflict:
			conflicts = append(conflicts, fileEntry{label: fs.Path, path: fs.Path, mode: git.DiffUnstaged, status: 'U', when: when})
		case fs.Untracked:
			untracked = append(untracked, fileEntry{label: fs.Path, path: fs.Path, mode: git.DiffUntracked, status: '?', when: when})
		default:
			if fs.Staged != '.' {
				label := fs.Path
				if fs.OrigPath != "" {
					label = fs.OrigPath + " → " + fs.Path
				}
				staged = append(staged, fileEntry{label: label, path: fs.Path, mode: git.DiffStaged, status: fs.Staged, when: when})
			}
			if fs.Unstaged != '.' {
				unstaged = append(unstaged, fileEntry{label: fs.Path, path: fs.Path, mode: git.DiffUnstaged, status: fs.Unstaged, when: when})
			}
		}
	}
	add := func(name string, es []fileEntry) {
		if len(es) == 0 {
			return
		}
		f.entries = append(f.entries, fileEntry{header: true, label: fmt.Sprintf("%s (%d)", name, len(es))})
		f.entries = append(f.entries, es...)
	}
	add("Conflicts", conflicts)
	add("Staged", staged)
	add("Unstaged", unstaged)
	add("Untracked", untracked)
	if len(f.entries) == 0 {
		f.entries = append(f.entries, fileEntry{header: true, label: "working tree clean"})
	}
	f.cursor = 0
	f.offset = 0
	f.selectFirst()
}

func (f *filesPane) setMessage(wt *state.Worktree, msg string) {
	f.worktree = wt.Path
	f.entries = []fileEntry{{header: true, label: msg}}
	f.cursor, f.offset = 0, 0
}

func (f *filesPane) setDiffFiles(wt *state.Worktree, base string, files []git.DiffFile) {
	f.worktree = wt.Path
	f.title = "vs " + base
	f.entries = f.entries[:0]
	if len(files) == 0 {
		f.entries = append(f.entries, fileEntry{header: true, label: "no changes vs " + base})
	}
	for _, df := range files {
		label := df.Path
		if df.OrigPath != "" {
			label = df.OrigPath + " → " + df.Path
		}
		f.entries = append(f.entries, fileEntry{label: label, path: df.Path, status: df.Status})
	}
	f.cursor, f.offset = 0, 0
	f.selectFirst()
}

func (f *filesPane) setCommits(wt *state.Worktree, base string, commits []git.Commit) {
	f.worktree = wt.Path
	f.title = "ahead of " + base
	f.entries = f.entries[:0]
	if len(commits) == 0 {
		f.entries = append(f.entries, fileEntry{header: true, label: "no commits ahead of " + base})
	}
	for _, c := range commits {
		f.entries = append(f.entries, fileEntry{
			label: fmt.Sprintf("%s %s", c.Short, c.Subject),
			hash:  c.Hash, status: 'c', when: c.When, author: c.Author,
		})
	}
	f.cursor, f.offset = 0, 0
	f.selectFirst()
}

func (f *filesPane) selectFirst() {
	for i, e := range f.entries {
		if !e.header {
			f.cursor = i
			return
		}
	}
}

func (f *filesPane) selected() *fileEntry {
	if f.cursor >= 0 && f.cursor < len(f.entries) && !f.entries[f.cursor].header {
		return &f.entries[f.cursor]
	}
	return nil
}

// move steps the cursor over selectable entries.
func (f *filesPane) move(delta int) {
	if len(f.entries) == 0 {
		return
	}
	i := f.cursor
	for {
		i += delta
		if i < 0 || i >= len(f.entries) {
			break
		}
		if !f.entries[i].header {
			f.cursor = i
			break
		}
	}
	f.clamp()
}

func (f *filesPane) clamp() {
	inner := f.innerHeight()
	if f.cursor < f.offset {
		f.offset = f.cursor
	}
	if f.cursor >= f.offset+inner {
		f.offset = f.cursor - inner + 1
	}
	if f.offset < 0 {
		f.offset = 0
	}
}

func (f *filesPane) innerHeight() int {
	h := f.height - 3 // border + tab line
	if h < 1 {
		h = 1
	}
	return h
}

func (f *filesPane) view(t Theme, focused bool) string {
	w := f.width - 2
	if w < 4 {
		w = 4
	}
	var tabs []string
	for i, name := range []string{"1 Changes", "2 vs Base", "3 Log"} {
		if filesTab(i) == f.tab {
			tabs = append(tabs, t.TabActive.Render(name))
		} else {
			tabs = append(tabs, t.Tab.Render(name))
		}
	}
	lines := []string{padRight(ansi.Truncate(strings.Join(tabs, "  "), w, ""), w)}
	inner := f.innerHeight()
	now := time.Now()
	// Width of the time column: widest rendered time among visible entries.
	timeW := 0
	for i := f.offset; i < len(f.entries) && i < f.offset+inner; i++ {
		if tw := ansi.StringWidth(smartTime(f.entries[i].when, now)); tw > timeW {
			timeW = tw
		}
	}
	for i := f.offset; i < len(f.entries) && len(lines) < inner+1; i++ {
		e := f.entries[i]
		var line string
		if e.header {
			line = t.Title.Render(padRight(ansi.Truncate(e.label, w, "…"), w))
		} else {
			labelW := w - 2
			when := smartTime(e.when, now)
			if timeW > 0 && labelW-timeW-1 >= 8 {
				labelW -= timeW + 1
				when = " " + padLeft(when, timeW)
			} else {
				when = ""
			}
			text := padRight(ansi.Truncate(e.label, labelW, "…"), labelW)
			if i == f.cursor {
				plain := string(statusRune(e.status)) + " " + text + when
				if focused {
					line = t.Selected.Render(plain)
				} else {
					line = t.SelectedDim.Render(plain)
				}
			} else {
				line = statusMark(t, e.status) + " " + text + t.Dim.Render(when)
			}
		}
		lines = append(lines, line)
	}
	for len(lines) < inner+1 {
		lines = append(lines, strings.Repeat(" ", w))
	}
	title := f.tab.String()
	if f.title != "" {
		title = f.title
	}
	return frame(t, title, strings.Join(lines, "\n"), f.width, f.height, focused)
}

func statusRune(s byte) rune {
	switch s {
	case 0:
		return ' '
	case 'c':
		return '•'
	}
	return rune(s)
}

func statusMark(t Theme, s byte) string {
	r := string(statusRune(s))
	switch s {
	case 'A', '?':
		return t.DiffAdd.Render(r)
	case 'D':
		return t.DiffDel.Render(r)
	case 'U':
		return t.BadgeWarn.Render(r)
	case 'M', 'R', 'C', 'T':
		return t.BadgeDirty.Render(r)
	}
	return t.Dim.Render(r)
}
