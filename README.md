# pyragit

A terminal UI for people who juggle many git projects, each with several
worktrees. It shows every project and worktree in one tree, flags which ones
have uncommitted work or need a push or pull, lets you review diffs, and runs
the everyday remote and worktree operations from one place.

It is built for the case where AI coding agents create worktrees in parallel
and you want one screen to see what they changed.

## Install

```bash
go install github.com/spik3s/pyragit/cmd/pyragit@latest
```

Or download a binary from the GitHub releases page. Requires `git` on PATH.

## Run

```bash
pyragit
```

On first run pyragit writes `~/.config/pyragit/config.toml` with the current
directory as the only scan root. Edit `scan_roots` to point at the folders that
hold your repositories, for example `["~/Developer"]`.

`pyragit --dump` prints the discovered projects and their status as plain text.

## Screen

```
┌ Projects ─────┐┌ Changes ─────────────┐┌ Diff ─────────────────────┐
│▾ api (3)      ││1 Changes 2 vs Base 3 Log│diff --git a/x b/x        │
│  main     ↓2  ││Unstaged (2)          ││...                        │
│  feat/a ●4 ↑1 ││M src/a.go            ││                           │
│  fix/b   zz ! ││? notes.md            ││                           │
└───────────────┘└──────────────────────┘└───────────────────────────┘
┌ fetch api ✓ 1.2s ───────────────────────────────────────────────────┐
│From github.com:me/api                                               │
└─────────────────────────────────────────────────────────────────────┘
```

Badges on a worktree row:

| Badge | Meaning |
|---|---|
| `●N` | N changed or untracked files |
| `↑a ↓b` | commits ahead of and behind the upstream branch |
| `!` | no upstream configured |
| `detached 544aa8e` (in place of a branch) | no branch checked out; press `n` to create one from that commit |
| `⚠` | merge conflicts |
| `zz` | no commit in `stale_after_days` and nothing uncommitted |
| `✗` | git failed in this worktree |
| `5m` `3h` `2d` | age of the last activity: the newer of the last commit and the last file change |

The Changes tab shows each file's modification time and the Log tab shows
each commit's time: clock time if today, `Sep 9 14:32` if this year, the date
otherwise. The status bar shows the full timestamp of the selected file or
commit.

## Keys

| Key | Action |
|---|---|
| `j` `k` `g` `G` | move in the focused pane |
| `tab` `shift+tab` `h` `l` | change focused pane |
| `enter` | open the selection in the next pane |
| `space` `z` | collapse or expand a project |
| `1` `2` `3` | Changes, vs Base, Log tab |
| `d` `u` | half page down or up in the diff |
| `w` | toggle whitespace-insensitive diff |
| `/` | filter projects and branches |
| `r` `R` | refresh the selected worktree, rediscover and refresh all |
| `f` `F` | fetch project, fetch all projects |
| `p` `P` | pull (fast-forward only), push |
| `c` `n` | checkout branch, new branch |
| `N` `D` | new worktree, remove worktree |
| `b` | set the project's base branch |
| `e` `s` `y` | open in editor, open shell here, copy path |
| `o` | toggle the output pane |
| `:` `ctrl+p` | command palette (includes push --set-upstream, force push, prune) |
| `ctrl+c` | cancel the running operation |
| `?` | help |
| `q` | quit |

## Config

`~/.config/pyragit/config.toml` (or `$XDG_CONFIG_HOME/pyragit/config.toml`,
or the path in `$PYRAGIT_CONFIG`):

```toml
scan_roots = ["~/Developer"]        # folders to scan for repositories
scan_depth = 2                      # how deep below each root to look
repos = ["~/work/special-repo"]     # repositories to include regardless of roots
exclude = ["node_modules", ".cache", "vendor"]
auto_fetch_interval = "0"           # e.g. "5m"; "0" disables background fetch
stale_after_days = 7
editor = ""                         # falls back to $VISUAL, then $EDITOR, then vim
worktree_dir_template = "{repo_parent}/{repo_name}-worktrees/{branch}"

[repos_config."~/Developer/api"]
base_branch = "develop"             # overrides origin/HEAD, main, master detection
```

The base branch is what the vs Base and Log tabs compare against: files and
commits between `merge-base(base, HEAD)` and `HEAD`.

## How it works

pyragit shells out to the system `git` binary and parses its porcelain output,
so hooks, SSH agents and credential helpers behave exactly as on the command
line. Prompts are disabled (`GIT_TERMINAL_PROMPT=0`); set up credential
helpers or SSH keys before pushing.

Worktrees refresh live through FSEvents on macOS and inotify on Linux, with a
300 ms debounce. Adding or removing a worktree from outside pyragit triggers
rediscovery.

## Not in this version

Staging, committing, stashing, hunk selection, conflict resolution, rebase and
merge UIs, side-by-side diffs, Windows.

## Development

```bash
make test
make run
```
