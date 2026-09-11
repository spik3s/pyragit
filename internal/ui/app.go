// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/state"
	"github.com/spik3s/pyragit/internal/uistate"
	"github.com/spik3s/pyragit/internal/watch"
)

const watchDebounce = 300 * time.Millisecond

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
	watcher   *watch.Watcher
	watchErr  string
	noWatch   bool // tests: skip the blocking watch subscription

	output        outputPane
	op            *op
	pendingSelect string     // worktree path to select after the next rediscovery
	branchesFor   promptKind // what the next branchesMsg should open
	autoFetch     time.Duration

	// StatePath is where UI state (collapsed projects, selection) persists.
	// Empty disables persistence.
	StatePath string
	saved     uistate.State
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
		output:  newOutputPane(),

		mergeBase: map[string]string{},
	}
}

type tickMsg time.Time

type autoFetchMsg time.Time

// tickInterval drives the elapsed-time display while an operation runs.
var tickInterval = time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func autoFetchCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return autoFetchMsg(t) })
}

func (a App) Init() tea.Cmd {
	cmds := []tea.Cmd{loadCmd(a.cfg), tea.RequestBackgroundColor}
	if a.StatePath != "" {
		if st, err := uistate.Load(a.StatePath); err == nil {
			cmds = append(cmds, func() tea.Msg { return st })
		}
	}
	if d, err := a.cfg.AutoFetch(); err == nil && d > 0 {
		cmds = append(cmds, autoFetchCmd(d))
	}
	return tea.Batch(cmds...)
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
	case uistate.State:
		a.saved = msg
		a.output.visible = msg.OutputOpen
		a.diff.ignoreWS = msg.IgnoreWS
		if msg.LastFilesTab >= 0 && msg.LastFilesTab <= int(tabLog) {
			a.files.tab = filesTab(msg.LastFilesTab)
		}
		a.layout()
		if a.store != nil {
			a.applySavedSelection()
			return a, a.onSelectionChanged()
		}
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
	case watchEventMsg:
		return a.onWatchEvent(watch.Event(msg))
	case rediscoveredMsg:
		return a.onRediscovered(msg)
	case savedConfigMsg:
		if msg.err != nil {
			a.status = "config save failed: " + msg.err.Error()
		}
		return a, nil
	case opMsg:
		return a.onOp(opEvent(msg))
	case tickMsg:
		if a.op != nil && a.op.running {
			return a, tickCmd()
		}
		return a, nil
	case execDoneMsg:
		if msg.err != nil {
			a.status = "external command: " + msg.err.Error()
		}
		if w := a.sidebar.selectedWorktree(); w != nil {
			w.Loading = true
			return a, refreshCmd(w.Path, w.Project.BaseBranch)
		}
		return a, nil
	case autoFetchMsg:
		d, _ := a.cfg.AutoFetch()
		next := autoFetchCmd(d)
		if a.store == nil || (a.op != nil && a.op.running) {
			return a, next
		}
		m, cmd := a.beginOp("auto-fetch", "", true, followUp{refresh: a.store.All()}, fetchAllOp(a.store.Projects))
		return m, tea.Batch(cmd, next)
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
	if a.branchesFor == promptCheckout {
		cmd := a.prompt.open(promptCheckout, "Checkout branch in "+w.Branch+" worktree", names, "")
		return a, cmd
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
	case promptPalette:
		if act := actionByLabel(msg.value); act != nil {
			return a.runAction(act.id)
		}
		return a, nil
	default:
		return a.onActionPrompt(msg)
	}
}

func (a *App) layout() {
	if a.width == 0 || a.height == 0 {
		return
	}
	bodyH := a.height - 2 // status bar + key bar
	if a.output.visible {
		oh := clamp(a.height/4, 5, 12)
		a.output.resize(a.width, oh)
		bodyH -= oh
	}
	if bodyH < 5 {
		bodyH = 5
	}
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
	a.applySavedSelection()
	a.status = fmt.Sprintf("%d projects, %d worktrees discovered in %s", len(a.store.Projects), len(wts), msg.took.Round(time.Millisecond))
	if a.created {
		a.status = "wrote default config to " + a.cfgPath + " · " + a.status
	}
	if len(msg.errs) > 0 {
		a.status += fmt.Sprintf(" · %d errors", len(msg.errs))
	}
	cmd := a.onSelectionChanged()
	var watchCmd tea.Cmd
	if a.watcher == nil && !a.noWatch {
		a.watcher = watch.New(watchDebounce, a.cfg.Exclude)
		watchCmd = waitWatchCmd(a.watcher)
	}
	a.rewatch()
	return a, tea.Batch(refreshAllCmd(wts), cmd, watchCmd)
}

// rewatch points the watcher at the current set of worktrees and repos.
func (a *App) rewatch() {
	if a.watcher == nil || a.store == nil {
		return
	}
	var wts, commons []string
	for _, p := range a.store.Projects {
		commons = append(commons, p.CommonDir)
		for _, w := range p.Worktrees {
			wts = append(wts, w.Path)
		}
	}
	if err := a.watcher.Watch(wts, commons); err != nil {
		a.watchErr = "watch: " + err.Error()
		a.status = a.watchErr
	}
}

func (a App) onWatchEvent(ev watch.Event) (tea.Model, tea.Cmd) {
	var next tea.Cmd
	if a.watcher != nil {
		next = waitWatchCmd(a.watcher)
	}
	if a.store == nil {
		return a, next
	}
	switch ev.Kind {
	case watch.KindWorktree:
		if w := a.store.Get(ev.Path); w != nil {
			w.Loading = true
			return a, tea.Batch(next, refreshCmd(w.Path, w.Project.BaseBranch))
		}
	case watch.KindProject:
		for _, p := range a.store.Projects {
			if p.CommonDir == ev.Path {
				for _, w := range p.Worktrees {
					w.Loading = true
				}
				return a, tea.Batch(next, refreshAllCmd(p.Worktrees))
			}
		}
	case watch.KindWorktreeList:
		return a, tea.Batch(next, rediscoverCmd(a.cfg))
	}
	return a, next
}

func (a App) onRediscovered(msg rediscoveredMsg) (tea.Model, tea.Cmd) {
	if a.store == nil {
		return a, nil
	}
	a.store.Replace(msg.projects, state.BaseResolver(context.Background(), a.cfg))
	a.rewatch()
	a.sidebar.rebuild(a.store)
	if a.pendingSelect != "" {
		for i, r := range a.sidebar.rows {
			if r.worktree != nil && r.worktree.Path == a.pendingSelect {
				a.sidebar.cursor = i
				a.sidebar.clamp()
				a.pendingSelect = ""
				break
			}
		}
	}
	var fresh []*state.Worktree
	for _, w := range a.store.All() {
		if !w.Loaded && !w.Loading {
			w.Loading = true
			fresh = append(fresh, w)
		}
	}
	a.status = fmt.Sprintf("%d projects, %d worktrees", len(a.store.Projects), len(a.store.All()))
	return a, tea.Batch(refreshAllCmd(fresh), a.onSelectionChanged())
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
		a.diff.title = fmt.Sprintf("%s · %s · %s (%s)", short(e.hash), e.author, smartTime(e.when, time.Now()), ago(e.when))
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
	case keyCtrlC:
		if a.op != nil && a.op.running {
			a.op.cancel()
			a.status = "cancelling " + a.op.name
			return a, nil
		}
		return a.quit()
	case keyQuit:
		return a.quit()
	case keyPalette, keyPalette2:
		labels := make([]string, 0, len(actions))
		for _, act := range actions {
			labels = append(labels, paletteLabel(act))
		}
		return a, a.prompt.open(promptPalette, "Command palette", labels, "")
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
	// Action keys apply everywhere except where a pane uses the same key.
	if act := actionByKey(k); act != nil && !(a.focus == paneDiff && (k == "d" || k == "u")) {
		switch act.id {
		case "checkout":
			a.branchesFor = promptCheckout
		case "set-base":
			a.branchesFor = promptSetBase
		}
		return a.runAction(act.id)
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

// applySavedSelection restores collapsed projects and the selected worktree.
func (a *App) applySavedSelection() {
	collapsed := map[string]bool{}
	for _, c := range a.saved.Collapsed {
		collapsed[c] = true
	}
	for _, p := range a.store.Projects {
		if collapsed[p.CommonDir] {
			p.Collapsed = true
		}
	}
	a.sidebar.rebuild(a.store)
	if a.saved.Selected != "" {
		for i, r := range a.sidebar.rows {
			if r.worktree != nil && r.worktree.Path == a.saved.Selected {
				a.sidebar.cursor = i
				a.sidebar.clamp()
				break
			}
		}
	}
}

// persist writes UI state to disk when persistence is enabled.
func (a App) persist() {
	if a.StatePath == "" || a.store == nil {
		return
	}
	st := uistate.State{OutputOpen: a.output.visible, IgnoreWS: a.diff.ignoreWS, LastFilesTab: int(a.files.tab)}
	for _, p := range a.store.Projects {
		if p.Collapsed {
			st.Collapsed = append(st.Collapsed, p.CommonDir)
		}
	}
	if w := a.sidebar.selectedWorktree(); w != nil {
		st.Selected = w.Path
	}
	_ = uistate.Save(a.StatePath, st)
}

func (a App) quit() (tea.Model, tea.Cmd) {
	a.persist()
	if a.op != nil && a.op.running {
		a.op.cancel()
	}
	if a.watcher != nil {
		a.watcher.Close()
	}
	return a, tea.Quit
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
	case keyEnter:
		a.focus = paneFiles
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
		v.SetContent(helpView(a.theme, a.width, a.height-1) + "\n" + a.keyBar())
		return v
	}
	if a.prompt.active {
		v.SetContent(a.prompt.view(a.theme, a.width, a.height-1) + "\n" + a.keyBar())
		return v
	}
	now := time.Now()
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		a.sidebar.view(a.theme, a.focus == paneSidebar, now),
		a.files.view(a.theme, a.focus == paneFiles),
		a.diff.view(a.theme, a.focus == paneDiff),
	)
	if a.output.visible {
		body += "\n" + a.output.view(a.theme, false)
	}
	v.SetContent(body + "\n" + a.statusBar() + "\n" + a.keyBar())
	return v
}

func (a App) statusBar() string {
	t := a.theme
	right := ""
	avail := a.width - 1

	// Pieces in priority order; the path is shortened first when space runs out.
	var head []string
	if a.op != nil && a.op.running {
		head = append(head, t.BadgeDirty.Render("⟳ "+a.op.name))
	}
	if a.status != "" {
		head = append(head, a.status)
	}
	var extra []string
	var path string
	if a.loading {
		head = append(head, "discovering repositories…")
	} else if w := a.sidebar.selectedWorktree(); w != nil {
		path = w.Path
		if w.Loaded && w.Snap.Err == nil {
			st := w.Snap.Status
			if st.Upstream != "" {
				extra = append(extra, st.Upstream)
			}
			if w.Project.BaseBranch != "" && w.Snap.AheadBase+w.Snap.BehindBase > 0 {
				extra = append(extra, fmt.Sprintf("vs %s +%d/-%d", w.Project.BaseBranch, w.Snap.AheadBase, w.Snap.BehindBase))
			}
			if e := a.files.selected(); e != nil && !e.when.IsZero() {
				switch a.files.tab {
				case tabChanges:
					extra = append(extra, "modified "+absTime(e.when))
				case tabLog:
					extra = append(extra, "committed "+absTime(e.when))
				}
			}
		}
	}
	headStr := strings.Join(head, "  ")
	extraStr := strings.Join(extra, " · ")
	used := lipgloss.Width(headStr)
	if extraStr != "" {
		used += lipgloss.Width(extraStr) + 2
	}
	if path != "" {
		room := avail - used - 2
		if room < 12 {
			path = ""
		} else if lipgloss.Width(path) > room {
			path = "…" + ansi.TruncateLeft(path, lipgloss.Width(path)-room+1, "")
		}
	}
	left := headStr
	if path != "" {
		left = joinNonEmpty(left, t.Dim.Render(path))
	}
	if extraStr != "" {
		left = joinNonEmpty(left, t.Dim.Render(extraStr))
	}
	gap := a.width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		return ansi.Truncate(left, a.width, "…")
	}
	return left + strings.Repeat(" ", gap) + right
}

func joinNonEmpty(a, b string) string {
	if a == "" {
		return b
	}
	return a + "  " + b
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
