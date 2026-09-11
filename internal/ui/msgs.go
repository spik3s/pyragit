package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/git"
	"github.com/spik3s/pyragit/internal/state"
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
