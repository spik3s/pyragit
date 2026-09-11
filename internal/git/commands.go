package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// GetStatus runs `git status --porcelain=v2 --branch` in dir.
func GetStatus(ctx context.Context, dir string) (Status, error) {
	out, err := Run(ctx, dir, "status", "--porcelain=v2", "--branch", "--untracked-files=normal")
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(out)
}

// Worktrees lists the worktrees of the repository containing dir.
func Worktrees(ctx context.Context, dir string) ([]Worktree, error) {
	out, err := Run(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(out)
}

// CommonDir returns the absolute path of the repository's shared .git directory.
func CommonDir(ctx context.Context, dir string) (string, error) {
	out, err := Run(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

// TopLevel returns the root of the worktree containing dir.
func TopLevel(ctx context.Context, dir string) (string, error) {
	out, err := Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(strings.TrimSpace(out)), nil
}

// MergeBase returns the merge base of a and b.
func MergeBase(ctx context.Context, dir, a, b string) (string, error) {
	out, err := Run(ctx, dir, "merge-base", a, b)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// AheadBehind returns how many commits `rev` is ahead of and behind `base`.
func AheadBehind(ctx context.Context, dir, base, rev string) (ahead, behind int, err error) {
	out, err := Run(ctx, dir, "rev-list", "--left-right", "--count", rev+"..."+base)
	if err != nil {
		return 0, 0, err
	}
	return ParseAheadBehind(out)
}

// DiffFiles lists files changed between from and to (`git diff --name-status from to`).
// With to == "" it compares from against the working tree.
func DiffFiles(ctx context.Context, dir, from, to string) ([]DiffFile, error) {
	args := []string{"diff", "--name-status", "-M", from}
	if to != "" {
		args = append(args, to)
	}
	out, err := Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return ParseNameStatus(out)
}

// DiffMode selects which diff to produce for a working-tree file.
type DiffMode int

const (
	DiffUnstaged  DiffMode = iota // worktree vs index
	DiffStaged                    // index vs HEAD
	DiffUntracked                 // whole file as additions
)

// Diff returns the unified diff for one working-tree path.
func Diff(ctx context.Context, dir, path string, mode DiffMode, ignoreWS bool) (string, error) {
	args := []string{"diff", "--no-color"}
	if ignoreWS {
		args = append(args, "-w")
	}
	switch mode {
	case DiffStaged:
		args = append(args, "--cached")
	case DiffUntracked:
		args = append(args, "--no-index", "--", "/dev/null", path)
		out, err := Run(ctx, dir, args...)
		// --no-index exits 1 when files differ, which is the normal case.
		var ge *Error
		if errors.As(err, &ge) && ge.ExitCode == 1 {
			return out, nil
		}
		return out, err
	}
	args = append(args, "--", path)
	return Run(ctx, dir, args...)
}

// DiffRange returns the unified diff of one path between two revisions.
func DiffRange(ctx context.Context, dir, from, to, path string, ignoreWS bool) (string, error) {
	args := []string{"diff", "--no-color", "-M"}
	if ignoreWS {
		args = append(args, "-w")
	}
	args = append(args, from, to, "--", path)
	return Run(ctx, dir, args...)
}

// Show returns the full patch of one commit.
func Show(ctx context.Context, dir, hash string, ignoreWS bool) (string, error) {
	args := []string{"show", "--no-color", "--stat", "--patch", "-M"}
	if ignoreWS {
		args = append(args, "-w")
	}
	return Run(ctx, dir, append(args, hash)...)
}

// Log returns commits in the range spec (e.g. "base..HEAD" or "HEAD"), newest first.
func Log(ctx context.Context, dir, rangeSpec string, limit int) ([]Commit, error) {
	args := []string{"log", "--format=" + LogFormat}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, rangeSpec, "--")
	out, err := Run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	return ParseLog(out)
}

// HeadCommit returns the commit at HEAD.
func HeadCommit(ctx context.Context, dir string) (Commit, error) {
	cs, err := Log(ctx, dir, "HEAD", 1)
	if err != nil {
		return Commit{}, err
	}
	if len(cs) == 0 {
		return Commit{}, errors.New("no commits")
	}
	return cs[0], nil
}

// Branches lists local branches.
func Branches(ctx context.Context, dir string) ([]Branch, error) {
	out, err := Run(ctx, dir, "branch", "--format="+BranchFormat)
	if err != nil {
		return nil, err
	}
	return ParseBranches(out)
}

// DefaultBranch guesses the repository's base branch: origin/HEAD's target,
// else main, else master. Returns "" when none exists.
func DefaultBranch(ctx context.Context, dir string) string {
	if out, err := Run(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		if name := strings.TrimPrefix(strings.TrimSpace(out), "origin/"); name != "" {
			return name
		}
	}
	for _, name := range []string{"main", "master"} {
		if _, err := Run(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name); err == nil {
			return name
		}
	}
	return ""
}

// RefExists reports whether ref resolves.
func RefExists(ctx context.Context, dir, ref string) bool {
	_, err := Run(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// Fetch, Pull and Push stream their output to onLine.

func Fetch(ctx context.Context, dir string, onLine func(string)) error {
	return RunStream(ctx, dir, onLine, "fetch", "--prune", "--progress")
}

func Pull(ctx context.Context, dir string, onLine func(string)) error {
	return RunStream(ctx, dir, onLine, "pull", "--ff-only", "--progress")
}

func Push(ctx context.Context, dir string, setUpstream, force bool, onLine func(string)) error {
	args := []string{"push", "--progress"}
	if setUpstream {
		args = append(args, "--set-upstream", "origin", "HEAD")
	}
	if force {
		args = append(args, "--force-with-lease")
	}
	return RunStream(ctx, dir, onLine, args...)
}

// Checkout switches dir to branch.
func Checkout(ctx context.Context, dir, branch string) error {
	_, err := Run(ctx, dir, "checkout", branch)
	return err
}

// CreateBranch creates and switches to a new branch from startPoint ("" = HEAD).
func CreateBranch(ctx context.Context, dir, name, startPoint string) error {
	args := []string{"checkout", "-b", name}
	if startPoint != "" {
		args = append(args, startPoint)
	}
	_, err := Run(ctx, dir, args...)
	return err
}

// WorktreeAdd creates a worktree at path. If newBranch is true a branch named
// `branch` is created from startPoint; otherwise `branch` must already exist.
func WorktreeAdd(ctx context.Context, repoDir, path, branch, startPoint string, newBranch bool) error {
	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "-b", branch, path)
		if startPoint != "" {
			args = append(args, startPoint)
		}
	} else {
		args = append(args, path, branch)
	}
	_, err := Run(ctx, repoDir, args...)
	return err
}

// WorktreeRemove removes a worktree; force discards uncommitted changes.
func WorktreeRemove(ctx context.Context, repoDir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := Run(ctx, repoDir, append(args, path)...)
	return err
}

// WorktreePrune removes stale worktree metadata and returns git's report.
func WorktreePrune(ctx context.Context, repoDir string) (string, error) {
	return Run(ctx, repoDir, "worktree", "prune", "--verbose")
}

// Info describes where a directory sits in a repository.
type Info struct {
	TopLevel  string // root of the working tree
	GitDir    string // this worktree's git dir
	CommonDir string // the repository's shared git dir
}

// IsMainWorktree reports whether dir is the repository's primary worktree
// (as opposed to a linked worktree).
func (i Info) IsMainWorktree() bool { return i.GitDir == i.CommonDir }

// RepoInfo resolves TopLevel, GitDir and CommonDir for dir in one call.
func RepoInfo(ctx context.Context, dir string) (Info, error) {
	out, err := Run(ctx, dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	if err != nil {
		return Info{}, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		return Info{}, fmt.Errorf("git rev-parse: unexpected output %q", out)
	}
	return Info{
		TopLevel:  filepath.Clean(lines[0]),
		GitDir:    filepath.Clean(lines[1]),
		CommonDir: filepath.Clean(lines[2]),
	}, nil
}
