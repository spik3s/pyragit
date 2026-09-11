// Package ui is the Bubble Tea front end.
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/state"
)

type pane int

const (
	paneSidebar pane = iota
	paneFiles
	paneDiff
	paneCount
)

// App is the root model.
type App struct {
	cfg     config.Config
	cfgPath string
	created bool
	theme   Theme
	width   int
	height  int

	store   *state.Store
	loading bool
	loadErr []error
	status  string // transient status-bar message

	focus    pane
	showHelp bool
	sidebar  sidebar
	files    filesPane
	diff     diffView
	prompt   prompt

	mergeBase map[string]string // worktree path -> merge-base with its base branch
}

// New builds the root model.
func New(cfg config.Config, cfgPath string, created bool) App {
	return App{
		cfg:     cfg,
		cfgPath: cfgPath,
		created: created,
		theme:   NewTheme(true),
		loading: true,
		sidebar: newSidebar(cfg.StaleAfterDays),
		diff:    newDiffView(),
		prompt:  newPrompt(),

		mergeBase: map[string]string{},
	}
}

func (a App) Init() tea.Cmd {
	return tea.Batch(loadCmd(a.cfg), tea.RequestBackgroundColor)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		a.theme = NewTheme(msg.IsDark())
		return a, nil
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		a.layout()
		return a, nil
	case loadedMsg:
		return a.onLoaded(msg)
	case snapshotMsg:
		return a.onSnapshot(state.Snapshot(msg))
	case diffMsg:
		if msg.key == a.diff.key {
			if msg.err != nil {
				a.diff.setContent(a.theme, "error: "+msg.err.Error())
			} else {
				a.diff.setContent(a.theme, msg.content)
			}
		}
		return a, nil
	case baseFilesMsg:
		return a.onBaseFiles(msg)
	case logMsg:
		return a.onLog(msg)
	case branchesMsg:
		return a.onBranches(msg)
	case promptResultMsg:
		return a.onPromptResult(msg)
	case savedConfigMsg:
		if msg.err != nil {
			a.status = "config save failed: " + msg.err.Error()
		}
		return a, nil
	case tea.KeyPressMsg:
		return a.onKey(msg)
	}
	return a, nil
}

func (a App) onBaseFiles(msg baseFilesMsg) (tea.Model, tea.Cmd) {
	w := a.sidebar.selectedWorktree()
	if w == nil || w.Path != msg.worktree || a.files.tab != tabBase || w.Project.BaseBranch != msg.base {
		return a, nil
	}
	if msg.err != nil {
		a.files.setMessage(w, msg.err.Error())
		return a, a.loadDiffForSelection()
	}
	a.mergeBase[w.Path] = msg.mergeBase
	a.files.setDiffFiles(w, msg.base, msg.files)
	return a, a.loadDiffForSelection()
}

func (a App) onLog(msg logMsg) (tea.Model, tea.Cmd) {
	w := a.sidebar.selectedWorktree()
	if w == nil || w.Path != msg.worktree || a.files.tab != tabLog || w.Project.BaseBranch != msg.base {
		return a, nil
	}
	if msg.err != nil {
		a.files.setMessage(w, msg.err.Error())
		return a, a.loadDiffForSelection()
	}
	a.mergeBase[w.Path] = msg.mergeBase
	a.files.setCommits(w, msg.base, msg.commits)
	return a, a.loadDiffForSelection()
}

func (a App) onBranches(msg branchesMsg) (tea.Model, tea.Cmd) {
	w := a.sidebar.selectedWorktree()
	if w == nil || w.Path != msg.worktree {
		return a, nil
	}
	if msg.err != nil {
		a.status = msg.err.Error()
		return a, nil
	}
	names := make([]string, 0, len(msg.branches))
	for _, b := range msg.branches {
		names = append(names, b.Name)
	}
	cmd := a.prompt.open(promptSetBase, "Base branch for "+w.Project.Name, names, "")
	// Preselect the current base.
	for i, n := range a.prompt.filtered {
		if n == w.Project.BaseBranch {
			a.prompt.cursor = i
		}
	}
	return a, cmd
}

func (a App) onPromptResult(msg promptResultMsg) (tea.Model, tea.Cmd) {
	switch msg.kind {
	case promptSetBase:
		w := a.sidebar.selectedWorktree()
		if w == nil {
			return a, nil
		}
		p := w.Project
		p.BaseBranch = msg.value
		a.cfg.SetBaseBranch(p.Path, msg.value)
		for _, wt := range p.Worktrees {
			wt.Loading = true
			delete(a.mergeBase, wt.Path)
		}
		a.status = "base branch for " + p.Name + " set to " + msg.value
		return a, tea.Batch(saveConfigCmd(a.cfgPath, a.cfg), refreshAllCmd(p.Worktrees), a.onSelectionChanged())
	}
	return a, nil
}

func (a *App) layout() {
	if a.width == 0 || a.height == 0 {
		return
	}
	bodyH := a.height - 1 // status bar
	sw := clamp(a.width/4, 24, 40)
	fw := clamp(a.width/3, 30, 60)
	if a.width < 100 {
		fw = clamp((a.width-sw)/2, 20, 60)
	}
	dw := a.width - sw - fw
	if dw < 20 {
		dw = 20
	}
	a.sidebar.width, a.sidebar.height = sw, bodyH
	a.files.width, a.files.height = fw, bodyH
	a.diff.resize(dw, bodyH)
	a.sidebar.clamp()
	a.files.clamp()
	if a.diff.raw != "" {
		a.diff.vp.SetContent(colourDiff(a.theme, a.diff.raw, max(dw-2, 1)))
	}
}

func (a App) onLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	a.store = msg.store
	a.loading = false
	a.loadErr = msg.errs
	wts := a.store.All()
	for _, w := range wts {
		w.Loading = true
	}
	a.sidebar.rebuild(a.store)
	a.status = fmt.Sprintf("%d projects, %d worktrees discovered in %s", len(a.store.Projects), len(wts), msg.took.Round(time.Millisecond))
	if a.created {
		a.status = "wrote default config to " + a.cfgPath + " · " + a.status
	}
	if len(msg.errs) > 0 {
		a.status += fmt.Sprintf(" · %d errors", len(msg.errs))
	}
	cmd := a.onSelectionChanged()
	return a, tea.Batch(refreshAllCmd(wts), cmd)
}

func (a App) onSnapshot(snap state.Snapshot) (tea.Model, tea.Cmd) {
	if a.store == nil {
		return a, nil
	}
	a.store.Apply(snap)
	a.sidebar.rebuild(a.store)
	if w := a.sidebar.selectedWorktree(); w != nil && w.Path == snap.Path {
		return a, a.onSelectionChanged()
	}
	return a, nil
}

// onSelectionChanged refreshes the files pane for the selected worktree and
// kicks off loading the diff for its first entry.
func (a *App) onSelectionChanged() tea.Cmd {
	w := a.sidebar.selectedWorktree()
	if w == nil {
		a.files.entries = nil
		a.files.worktree = ""
		a.diff.setContent(a.theme, "")
		a.diff.title = ""
		return nil
	}
	switch a.files.tab {
	case tabChanges:
		a.files.setChanges(w)
		return a.loadDiffForSelection()
	case tabBase:
		a.files.setMessage(w, "loading…")
		a.files.title = "vs " + w.Project.BaseBranch
		a.diff.key = diffKey{}
		a.diff.title = ""
		a.diff.setContent(a.theme, "")
		return baseFilesCmd(w.Path, w.Project.BaseBranch)
	case tabLog:
		a.files.setMessage(w, "loading…")
		a.files.title = "ahead of " + w.Project.BaseBranch
		a.diff.key = diffKey{}
		a.diff.title = ""
		a.diff.setContent(a.theme, "")
		return logCmd(w.Path, w.Project.BaseBranch, 200)
	}
	return nil
}

func (a *App) loadDiffForSelection() tea.Cmd {
	e := a.files.selected()
	if e == nil {
		a.diff.key = diffKey{}
		a.diff.title = ""
		a.diff.setContent(a.theme, "")
		return nil
	}
	switch a.files.tab {
	case tabChanges:
		key := diffKey{worktree: a.files.worktree, tab: a.files.tab, path: e.path, mode: e.mode, ignoreWS: a.diff.ignoreWS}
		if key == a.diff.key && !a.diff.loading {
			return nil
		}
		a.diff.key = key
		a.diff.title = e.path
		a.diff.loading = true
		return workingDiffCmd(key)
	case tabBase:
		mb := a.mergeBase[a.files.worktree]
		if mb == "" {
			return nil
		}
		key := diffKey{worktree: a.files.worktree, tab: a.files.tab, path: e.path, rev: mb, ignoreWS: a.diff.ignoreWS}
		if key == a.diff.key && !a.diff.loading {
			return nil
		}
		a.diff.key = key
		a.diff.title = e.path
		a.diff.loading = true
		return rangeDiffCmd(key)
	case tabLog:
		key := diffKey{worktree: a.files.worktree, tab: a.files.tab, rev: e.hash, ignoreWS: a.diff.ignoreWS}
		if key == a.diff.key && !a.diff.loading {
			return nil
		}
		a.diff.key = key
		a.diff.title = short(e.hash)
		a.diff.loading = true
		return showCmd(key)
	}
	return nil
}

func (a App) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if a.showHelp {
		if k == keyHelp || k == keyEsc || k == keyQuit {
			a.showHelp = false
		}
		return a, nil
	}
	if a.prompt.active {
		cmd, res := a.prompt.update(msg)
		if res != nil {
			return a.Update(*res)
		}
		return a, cmd
	}
	if a.sidebar.filtering {
		return a.onFilterKey(msg)
	}
	switch k {
	case keyBase:
		if w := a.sidebar.selectedWorktree(); w != nil {
			return a, branchesCmd(w.Path)
		}
		return a, nil
	case keyCtrlC, keyQuit:
		return a, tea.Quit
	case keyHelp:
		a.showHelp = true
		return a, nil
	case keyTab, keyL, keyRight:
		if a.focus < paneCount-1 {
			a.focus++
		}
		return a, nil
	case keyShiftTab, keyH, keyLeft:
		if a.focus > 0 {
			a.focus--
		}
		return a, nil
	case keyRefresh:
		if w := a.sidebar.selectedWorktree(); w != nil {
			w.Loading = true
			a.status = "refreshing " + w.Branch
			return a, refreshCmd(w.Path, w.Project.BaseBranch)
		}
		return a, nil
	case keyRefreshAl:
		if a.store != nil {
			wts := a.store.All()
			for _, w := range wts {
				w.Loading = true
			}
			a.status = "refreshing all"
			return a, refreshAllCmd(wts)
		}
		return a, nil
	case keyTab1, keyTab2, keyTab3:
		a.files.tab = filesTab(k[0] - '1')
		return a, a.onSelectionChanged()
	case keyWS:
		a.diff.ignoreWS = !a.diff.ignoreWS
		a.diff.key = diffKey{}
		return a, a.loadDiffForSelection()
	case keyFilter:
		a.sidebar.filtering = true
		a.focus = paneSidebar
		return a, nil
	}
	switch a.focus {
	case paneSidebar:
		return a.onSidebarKey(k)
	case paneFiles:
		return a.onFilesKey(k)
	case paneDiff:
		return a.onDiffKey(k, msg)
	}
	return a, nil
}

func (a App) onFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEsc:
		a.sidebar.filter = ""
		a.sidebar.filtering = false
	case keyEnter:
		a.sidebar.filtering = false
	case "backspace":
		if n := len(a.sidebar.filter); n > 0 {
			a.sidebar.filter = a.sidebar.filter[:n-1]
		}
	case keyDown, "ctrl+n":
		a.sidebar.move(1)
	case keyUp, "ctrl+p":
		a.sidebar.move(-1)
	default:
		if msg.Text != "" {
			a.sidebar.filter += msg.Text
		}
	}
	a.sidebar.rebuild(a.store)
	return a, a.onSelectionChanged()
}

func (a App) onSidebarKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case keyJ, keyDown:
		a.sidebar.move(1)
	case keyK, keyUp:
		a.sidebar.move(-1)
	case keyTop:
		a.sidebar.cursor = 0
		a.sidebar.clamp()
	case keyBottom:
		a.sidebar.cursor = len(a.sidebar.rows) - 1
		a.sidebar.clamp()
	case keySpace, keyCollapse:
		if p := a.sidebar.selectedProject(); p != nil {
			p.Collapsed = !p.Collapsed
			a.sidebar.rebuild(a.store)
		}
	case keyEnter:
		if a.sidebar.selectedWorktree() == nil {
			if p := a.sidebar.selectedProject(); p != nil {
				p.Collapsed = !p.Collapsed
				a.sidebar.rebuild(a.store)
			}
		} else {
			a.focus = paneFiles
		}
	default:
		return a, nil
	}
	return a, a.onSelectionChanged()
}

func (a App) onFilesKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case keyJ, keyDown:
		a.files.move(1)
	case keyK, keyUp:
		a.files.move(-1)
	case keyTop:
		a.files.cursor = 0
		a.files.move(1)
		a.files.move(-1)
	case keyBottom:
		a.files.cursor = len(a.files.entries)
		a.files.move(-1)
	case keyEnter:
		a.focus = paneDiff
	default:
		return a, nil
	}
	return a, a.loadDiffForSelection()
}

func (a App) onDiffKey(k string, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch k {
	case keyJ, keyDown:
		a.diff.vp.ScrollDown(1)
	case keyK, keyUp:
		a.diff.vp.ScrollUp(1)
	case keyHalfDown:
		a.diff.vp.HalfPageDown()
	case keyHalfUp:
		a.diff.vp.HalfPageUp()
	case keyTop:
		a.diff.vp.GotoTop()
	case keyBottom:
		a.diff.vp.GotoBottom()
	case keySpace, "pgdown":
		a.diff.vp.ScrollDown(a.diff.vp.Height())
	case "pgup":
		a.diff.vp.ScrollUp(a.diff.vp.Height())
	}
	return a, nil
}

func (a App) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.WindowTitle = "pyragit"
	if a.width == 0 {
		v.SetContent("loading…")
		return v
	}
	if a.showHelp {
		v.SetContent(helpView(a.theme, a.width, a.height))
		return v
	}
	if a.prompt.active {
		v.SetContent(a.prompt.view(a.theme, a.width, a.height))
		return v
	}
	now := time.Now()
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		a.sidebar.view(a.theme, a.focus == paneSidebar, now),
		a.files.view(a.theme, a.focus == paneFiles),
		a.diff.view(a.theme, a.focus == paneDiff),
	)
	v.SetContent(body + "\n" + a.statusBar())
	return v
}

func (a App) statusBar() string {
	t := a.theme
	var left string
	if a.loading {
		left = "discovering repositories…"
	} else if w := a.sidebar.selectedWorktree(); w != nil {
		left = w.Path
		if w.Loaded && w.Snap.Err == nil {
			st := w.Snap.Status
			extra := []string{}
			if st.Upstream != "" {
				extra = append(extra, st.Upstream)
			}
			if w.Project.BaseBranch != "" && w.Snap.AheadBase+w.Snap.BehindBase > 0 {
				extra = append(extra, fmt.Sprintf("vs %s +%d/-%d", w.Project.BaseBranch, w.Snap.AheadBase, w.Snap.BehindBase))
			}
			if len(extra) > 0 {
				left += "  " + t.Dim.Render(strings.Join(extra, " · "))
			}
		}
	}
	if a.status != "" {
		left = a.status + "  " + t.Dim.Render(left)
	}
	right := t.Key.Render("?") + t.Dim.Render(" help  ") + t.Key.Render("q") + t.Dim.Render(" quit")
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return t.StatusBar.MaxWidth(a.width).Render(left)
	}
	return left + strings.Repeat(" ", gap) + right
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
