package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/spik3s/pyragit/internal/git"
	"github.com/spik3s/pyragit/internal/state"
)

// action is a palette entry.
type action struct {
	id    string
	label string
	key   string // hint shown in the palette; also bound globally in onKey
}

var actions = []action{
	{"fetch", "Fetch", "f"},
	{"fetch-all", "Fetch all projects", "F"},
	{"pull", "Pull (fast-forward only)", "p"},
	{"push", "Push", "P"},
	{"push-upstream", "Push and set upstream", ""},
	{"push-force", "Force push (with lease)", ""},
	{"checkout", "Checkout branch…", "c"},
	{"new-branch", "New branch…", "n"},
	{"new-worktree", "New worktree…", "N"},
	{"remove-worktree", "Remove worktree", "D"},
	{"prune", "Prune worktrees", ""},
	{"set-base", "Set base branch…", "b"},
	{"editor", "Open in editor", "e"},
	{"shell", "Open shell here", "s"},
	{"copy-path", "Copy path", "y"},
	{"output", "Toggle output pane", "o"},
	{"refresh", "Refresh worktree", "r"},
	{"refresh-all", "Rediscover and refresh all", "R"},
}

func actionByKey(k string) *action {
	for i := range actions {
		if actions[i].key == k && k != "" {
			return &actions[i]
		}
	}
	return nil
}

func actionByLabel(label string) *action {
	for i := range actions {
		if paletteLabel(actions[i]) == label {
			return &actions[i]
		}
	}
	return nil
}

func paletteLabel(a action) string {
	if a.key == "" {
		return a.label
	}
	return fmt.Sprintf("%-32s %s", a.label, a.key)
}

type execDoneMsg struct{ err error }

// runAction dispatches a palette action for the selected worktree.
func (a App) runAction(id string) (tea.Model, tea.Cmd) {
	w := a.sidebar.selectedWorktree()
	if w == nil && id != "fetch-all" && id != "output" && id != "refresh-all" && id != "prune" {
		a.status = "select a worktree first"
		return a, nil
	}
	if a.op != nil && a.op.running {
		switch id {
		case "fetch", "fetch-all", "pull", "push", "push-upstream", "push-force", "checkout", "new-branch", "new-worktree", "remove-worktree", "prune":
			a.status = "wait for " + a.op.name + " to finish (ctrl+c cancels)"
			return a, nil
		}
	}
	switch id {
	case "fetch":
		return a.beginOp("fetch "+w.Project.Name, w.Path, false, followUp{refresh: w.Project.Worktrees},
			func(ctx context.Context, onLine func(string)) error { return git.Fetch(ctx, w.Path, onLine) })
	case "fetch-all":
		if a.store == nil {
			return a, nil
		}
		return a.beginOp("fetch all", "", false, followUp{refresh: a.store.All()}, fetchAllOp(a.store.Projects))
	case "pull":
		return a.beginOp("pull "+w.Branch, w.Path, false, followUp{refresh: []*state.Worktree{w}},
			func(ctx context.Context, onLine func(string)) error { return git.Pull(ctx, w.Path, onLine) })
	case "push":
		if w.Loaded && !w.Snap.Status.HasUpstream() {
			return a.runAction("push-upstream")
		}
		return a.beginOp("push "+w.Branch, w.Path, false, followUp{refresh: []*state.Worktree{w}},
			func(ctx context.Context, onLine func(string)) error {
				return git.Push(ctx, w.Path, false, false, onLine)
			})
	case "push-upstream":
		return a.beginOp("push -u "+w.Branch, w.Path, false, followUp{refresh: []*state.Worktree{w}},
			func(ctx context.Context, onLine func(string)) error {
				return git.Push(ctx, w.Path, true, false, onLine)
			})
	case "push-force":
		a.prompt.openConfirm(promptConfirm, fmt.Sprintf("Force push %s to %s?", w.Branch, w.Snap.Status.Upstream), map[string]string{"action": "push-force"})
		return a, nil
	case "checkout":
		return a, branchesCmd(w.Path)
	case "new-branch":
		return a, a.prompt.open(promptNewBranch, "New branch from "+w.Branch, nil, "")
	case "new-worktree":
		cmd := a.prompt.open(promptNewWorktree, "New worktree for "+w.Project.Name+" (branch name)", nil, "")
		a.prompt.context["project"] = w.Project.Path
		return a, cmd
	case "remove-worktree":
		if w.IsMain() {
			a.status = "cannot remove the main worktree"
			return a, nil
		}
		title := fmt.Sprintf("Remove worktree %s (%s)?", w.Branch, w.Path)
		ctx := map[string]string{"action": "remove-worktree", "path": w.Path, "repo": w.Project.Path}
		if w.Loaded && w.Snap.Status.Dirty() {
			title = fmt.Sprintf("Worktree %s has %d uncommitted changes. Remove anyway (--force)?", w.Branch, len(w.Snap.Status.Files))
			ctx["force"] = "1"
		}
		a.prompt.openConfirm(promptConfirm, title, ctx)
		return a, nil
	case "prune":
		var projects []*state.Project
		if w != nil {
			projects = []*state.Project{w.Project}
		}
		return a.beginOp("prune worktrees", "", false, followUp{rediscover: true},
			func(ctx context.Context, onLine func(string)) error {
				for _, p := range projects {
					out, err := git.WorktreePrune(ctx, p.Path)
					for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
						if l != "" {
							onLine("[" + p.Name + "] " + l)
						}
					}
					if err != nil {
						return err
					}
				}
				onLine("done")
				return nil
			})
	case "set-base":
		return a, branchesCmd(w.Path)
	case "editor":
		editor := a.cfg.Editor
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vim"
		}
		parts := strings.Fields(editor)
		c := exec.Command(parts[0], append(parts[1:], ".")...)
		c.Dir = w.Path
		return a, tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err: err} })
	case "shell":
		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}
		c := exec.Command(shell)
		c.Dir = w.Path
		c.Env = append(os.Environ(), "PYRAGIT=1")
		return a, tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err: err} })
	case "copy-path":
		if err := clipboard.WriteAll(w.Path); err != nil {
			a.status = "copy failed: " + err.Error()
		} else {
			a.status = "copied " + w.Path
		}
		return a, nil
	case "output":
		a.output.visible = !a.output.visible
		a.layout()
		return a, nil
	case "refresh":
		w.Loading = true
		a.status = "refreshing " + w.Branch
		return a, refreshCmd(w.Path, w.Project.BaseBranch)
	case "refresh-all":
		if a.store == nil {
			return a, nil
		}
		wts := a.store.All()
		for _, wt := range wts {
			wt.Loading = true
		}
		a.status = "rediscovering and refreshing all"
		return a, tea.Batch(rediscoverCmd(a.cfg), refreshAllCmd(wts), sizeAllCmd(wts))
	}
	return a, nil
}

// beginOp starts an operation and shows the output pane unless quiet.
func (a App) beginOp(name, dir string, quiet bool, follow followUp, fn opFunc) (tea.Model, tea.Cmd) {
	o, cmd := startOp(name, dir, quiet, follow, fn)
	a.op = o
	a.output.reset(o)
	if !quiet && !a.output.visible {
		a.output.visible = true
		a.layout()
	}
	a.status = ""
	return a, tea.Batch(cmd, tickCmd())
}

func (a App) onOp(ev opEvent) (tea.Model, tea.Cmd) {
	if a.op == nil || ev.id != a.op.id {
		return a, nil
	}
	if !ev.done {
		a.op.lines = append(a.op.lines, ev.line)
		a.output.append(ev.line)
		return a, waitOpCmd(a.op)
	}
	o := a.op
	o.running = false
	o.err = ev.err
	o.took = ev.took
	o.cancel()
	a.output.render()
	if ev.err != nil {
		a.status = o.name + " failed: " + ev.err.Error()
		if o.quiet {
			a.output.visible = true
			a.layout()
		}
		return a, nil
	}
	a.status = o.name + " done"
	var cmds []tea.Cmd
	for _, w := range o.follow.refresh {
		w.Loading = true
		delete(a.mergeBase, w.Path)
	}
	if len(o.follow.refresh) > 0 {
		cmds = append(cmds, refreshAllCmd(o.follow.refresh))
	}
	if o.follow.rediscover {
		a.pendingSelect = o.follow.selectPath
		cmds = append(cmds, rediscoverCmd(a.cfg))
	}
	if a.files.tab != tabChanges {
		cmds = append(cmds, a.onSelectionChanged())
	}
	return a, tea.Batch(cmds...)
}

// onActionPrompt handles prompt results for operations.
func (a App) onActionPrompt(msg promptResultMsg) (tea.Model, tea.Cmd) {
	w := a.sidebar.selectedWorktree()
	switch msg.kind {
	case promptCheckout:
		if w == nil {
			return a, nil
		}
		branch := msg.value
		return a.beginOp("checkout "+branch, w.Path, false, followUp{refresh: []*state.Worktree{w}},
			func(ctx context.Context, onLine func(string)) error {
				if err := git.Checkout(ctx, w.Path, branch); err != nil {
					return err
				}
				onLine("switched to " + branch)
				return nil
			})
	case promptNewBranch:
		if w == nil {
			return a, nil
		}
		name := msg.value
		return a.beginOp("new branch "+name, w.Path, false, followUp{refresh: []*state.Worktree{w}},
			func(ctx context.Context, onLine func(string)) error {
				if err := git.CreateBranch(ctx, w.Path, name, ""); err != nil {
					return err
				}
				onLine("created and switched to " + name)
				return nil
			})
	case promptNewWorktree:
		if w == nil {
			return a, nil
		}
		branch := msg.value
		p := w.Project
		path := worktreePathFor(a.cfg.WorktreeDirTemplate, p.Path, branch)
		base := p.BaseBranch
		return a.beginOp("new worktree "+branch, p.Path, false, followUp{rediscover: true, selectPath: path},
			func(ctx context.Context, onLine func(string)) error {
				exists := git.RefExists(ctx, p.Path, "refs/heads/"+branch)
				onLine("path: " + path)
				var err error
				if exists {
					onLine("branch exists; checking it out")
					err = git.WorktreeAdd(ctx, p.Path, path, branch, "", false)
				} else {
					start := base
					if start == "" {
						start = "HEAD"
					}
					onLine("creating branch from " + start)
					err = git.WorktreeAdd(ctx, p.Path, path, branch, start, true)
				}
				if err != nil {
					return err
				}
				onLine("created " + path)
				return nil
			})
	case promptConfirm:
		if !msg.yes {
			return a, nil
		}
		switch msg.context["action"] {
		case "push-force":
			if w == nil {
				return a, nil
			}
			return a.beginOp("force push "+w.Branch, w.Path, false, followUp{refresh: []*state.Worktree{w}},
				func(ctx context.Context, onLine func(string)) error {
					return git.Push(ctx, w.Path, false, true, onLine)
				})
		case "remove-worktree":
			path, repo, force := msg.context["path"], msg.context["repo"], msg.context["force"] == "1"
			return a.beginOp("remove worktree "+filepath.Base(path), repo, false, followUp{rediscover: true},
				func(ctx context.Context, onLine func(string)) error {
					if err := git.WorktreeRemove(ctx, repo, path, force); err != nil {
						return err
					}
					onLine("removed " + path)
					return nil
				})
		}
	}
	return a, nil
}

// worktreePathFor expands the worktree_dir_template.
func worktreePathFor(template, repoPath, branch string) string {
	if template == "" {
		template = "{repo_parent}/{repo_name}-worktrees/{branch}"
	}
	r := strings.NewReplacer(
		"{repo_parent}", filepath.Dir(repoPath),
		"{repo_name}", filepath.Base(repoPath),
		"{repo}", repoPath,
		"{branch}", branch,
	)
	return filepath.Clean(r.Replace(template))
}
