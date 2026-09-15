package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/discovery"
	"github.com/spik3s/pyragit/internal/git"
	"github.com/spik3s/pyragit/internal/state"
	"github.com/spik3s/pyragit/internal/watch"
)

// Messages flowing into App.Update.

type loadedMsg struct {
	store *state.Store
	errs  []error
	took  time.Duration
}

type snapshotMsg state.Snapshot

type diffMsg struct {
	key     diffKey
	content string
	err     error
}

// diffKey identifies which diff a diffMsg carries so stale results are dropped.
type diffKey struct {
	worktree string
	tab      filesTab
	path     string
	mode     git.DiffMode
	ignoreWS bool
	rev      string
}

// refreshSem bounds concurrent git status runs across all refresh commands.
var refreshSem = make(chan struct{}, 8)

// sizeSem bounds concurrent du runs; they are disk-heavy.
var sizeSem = make(chan struct{}, 2)

type sizeMsg struct {
	path string
	size int64
	err  error
}

func sizeCmd(path string) tea.Cmd {
	return func() tea.Msg {
		sizeSem <- struct{}{}
		defer func() { <-sizeSem }()
		n, err := state.DiskUsage(context.Background(), path)
		return sizeMsg{path: path, size: n, err: err}
	}
}

func sizeAllCmd(wts []*state.Worktree) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(wts))
	for _, w := range wts {
		cmds = append(cmds, sizeCmd(w.Path))
	}
	return tea.Batch(cmds...)
}

func loadCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		store, errs := state.Load(context.Background(), cfg)
		return loadedMsg{store: store, errs: errs, took: time.Since(start)}
	}
}

func refreshCmd(path, base string) tea.Cmd {
	return func() tea.Msg {
		refreshSem <- struct{}{}
		defer func() { <-refreshSem }()
		return snapshotMsg(state.Refresh(context.Background(), path, base))
	}
}

func refreshAllCmd(wts []*state.Worktree) tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(wts))
	for _, w := range wts {
		cmds = append(cmds, refreshCmd(w.Path, w.Project.BaseBranch))
	}
	return tea.Batch(cmds...)
}

func workingDiffCmd(key diffKey) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Diff(context.Background(), key.worktree, key.path, key.mode, key.ignoreWS)
		return diffMsg{key: key, content: out, err: err}
	}
}

// Phase 3: branch diff and log.

type baseFilesMsg struct {
	worktree  string
	base      string
	mergeBase string
	files     []git.DiffFile
	err       error
}

type logMsg struct {
	worktree  string
	base      string
	mergeBase string
	commits   []git.Commit
	err       error
}

type branchesMsg struct {
	worktree string
	branches []git.Branch
	err      error
}

// resolveMergeBase returns the merge base of base and HEAD, or HEAD's parent
// chain root when base is unset or missing (so the view still shows something).
func resolveMergeBase(ctx context.Context, dir, base string) (string, error) {
	if base == "" {
		return "", errNoBase
	}
	if !git.RefExists(ctx, dir, base) {
		return "", fmt.Errorf("base branch %q not found", base)
	}
	return git.MergeBase(ctx, dir, base, "HEAD")
}

var errNoBase = errors.New("no base branch configured; press b to set one")

func baseFilesCmd(worktree, base string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		mb, err := resolveMergeBase(ctx, worktree, base)
		if err != nil {
			return baseFilesMsg{worktree: worktree, base: base, err: err}
		}
		files, err := git.DiffFiles(ctx, worktree, mb, "HEAD")
		return baseFilesMsg{worktree: worktree, base: base, mergeBase: mb, files: files, err: err}
	}
}

func logCmd(worktree, base string, limit int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		mb, err := resolveMergeBase(ctx, worktree, base)
		if err != nil {
			return logMsg{worktree: worktree, base: base, err: err}
		}
		commits, err := git.Log(ctx, worktree, mb+"..HEAD", limit)
		return logMsg{worktree: worktree, base: base, mergeBase: mb, commits: commits, err: err}
	}
}

func rangeDiffCmd(key diffKey) tea.Cmd {
	return func() tea.Msg {
		out, err := git.DiffRange(context.Background(), key.worktree, key.rev, "HEAD", key.path, key.ignoreWS)
		return diffMsg{key: key, content: out, err: err}
	}
}

func showCmd(key diffKey) tea.Cmd {
	return func() tea.Msg {
		out, err := git.Show(context.Background(), key.worktree, key.rev, key.ignoreWS)
		return diffMsg{key: key, content: out, err: err}
	}
}

func branchesCmd(worktree string) tea.Cmd {
	return func() tea.Msg {
		bs, err := git.Branches(context.Background(), worktree)
		return branchesMsg{worktree: worktree, branches: bs, err: err}
	}
}

type stashListMsg struct {
	worktree string
	stashes  []git.Stash
	err      error
}

func stashListCmd(worktree string) tea.Cmd {
	return func() tea.Msg {
		ss, err := git.StashList(context.Background(), worktree)
		return stashListMsg{worktree: worktree, stashes: ss, err: err}
	}
}

func stashShowCmd(key diffKey) tea.Cmd {
	return func() tea.Msg {
		out, err := git.StashShow(context.Background(), key.worktree, key.rev, key.ignoreWS)
		return diffMsg{key: key, content: out, err: err}
	}
}

type savedConfigMsg struct{ err error }

func saveConfigCmd(path string, cfg config.Config) tea.Cmd {
	return func() tea.Msg { return savedConfigMsg{err: config.Save(path, cfg)} }
}

// Phase 4: live refresh.

type watchEventMsg watch.Event

type rediscoveredMsg struct {
	projects []discovery.Project
	errs     []error
}

// waitWatchCmd blocks until the watcher emits an event. Re-issue it after
// each watchEventMsg to keep the subscription alive.
func waitWatchCmd(w *watch.Watcher) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-w.Events()
		if !ok {
			return nil
		}
		return watchEventMsg(ev)
	}
}

func rediscoverCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		projects, errs := discovery.Discover(context.Background(), state.DiscoverOptions(cfg))
		return rediscoveredMsg{projects: projects, errs: errs}
	}
}
