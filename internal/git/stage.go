package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Stage adds paths to the index. Untracked and conflicted files are staged too.
func Stage(ctx context.Context, dir string, paths ...string) error {
	_, err := Run(ctx, dir, append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// StageAll stages every change including untracked files.
func StageAll(ctx context.Context, dir string) error {
	_, err := Run(ctx, dir, "add", "-A")
	return err
}

// Unstage removes paths from the index, keeping working tree changes.
func Unstage(ctx context.Context, dir string, paths ...string) error {
	_, err := Run(ctx, dir, append([]string{"reset", "-q", "--"}, paths...)...)
	return err
}

// UnstageAll clears the index back to HEAD.
func UnstageAll(ctx context.Context, dir string) error {
	_, err := Run(ctx, dir, "reset", "-q")
	return err
}

// DiscardFile throws away changes to one tracked path (index and worktree).
func DiscardFile(ctx context.Context, dir, path string) error {
	if _, err := Run(ctx, dir, "reset", "-q", "--", path); err != nil {
		// A path that is new in the index has nothing in HEAD; reset still
		// unstages it, but exits non-zero on some git versions. Continue.
		_ = err
	}
	_, err := Run(ctx, dir, "checkout", "--", path)
	return err
}

// DeleteUntracked removes an untracked path, including directories.
func DeleteUntracked(ctx context.Context, dir, path string) error {
	_, err := Run(ctx, dir, "clean", "-f", "-d", "-q", "--", path)
	return err
}

// DiscardAll resets the worktree to HEAD and deletes untracked files and dirs.
func DiscardAll(ctx context.Context, dir string) error {
	if _, err := Run(ctx, dir, "reset", "-q", "--hard"); err != nil {
		return err
	}
	_, err := Run(ctx, dir, "clean", "-f", "-d", "-q")
	return err
}

// CreateCommit records the index with message. amend rewrites HEAD instead.
func CreateCommit(ctx context.Context, dir, message string, amend bool) (string, error) {
	args := []string{"commit", "-q", "-m", message}
	if amend {
		args = append(args, "--amend")
	}
	out, err := Run(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

// HeadSubject returns the subject line of HEAD.
func HeadSubject(ctx context.Context, dir string) string {
	out, _ := Run(ctx, dir, "log", "-1", "--format=%s")
	return strings.TrimSpace(out)
}

// Stash is one entry from `git stash list`.
type Stash struct {
	Index   int // 0 is the newest
	Ref     string
	When    time.Time
	Message string
}

// StashFormat is the --format that ParseStashList understands.
const StashFormat = "%gd%x00%ct%x00%s"

// ParseStashList parses `git stash list --format=StashFormat` output.
func ParseStashList(out string) ([]Stash, error) {
	var ss []Stash
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 3 {
			return nil, fmt.Errorf("git stash list: malformed line %q", line)
		}
		idx := -1
		if i := strings.Index(f[0], "{"); i >= 0 {
			idx, _ = strconv.Atoi(strings.TrimSuffix(f[0][i+1:], "}"))
		}
		ts, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git stash list: bad timestamp in %q", line)
		}
		ss = append(ss, Stash{Index: idx, Ref: f[0], When: time.Unix(ts, 0), Message: f[2]})
	}
	return ss, nil
}

// StashList lists stashes, newest first.
func StashList(ctx context.Context, dir string) ([]Stash, error) {
	out, err := Run(ctx, dir, "stash", "list", "--format="+StashFormat)
	if err != nil {
		return nil, err
	}
	return ParseStashList(out)
}

// StashPush saves the working tree. untracked includes untracked files.
func StashPush(ctx context.Context, dir, message string, untracked bool) (string, error) {
	args := []string{"stash", "push", "-q"}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if message != "" {
		args = append(args, "-m", message)
	}
	out, err := Run(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

// StashShow returns the patch of one stash, untracked files included.
func StashShow(ctx context.Context, dir, ref string, ignoreWS bool) (string, error) {
	args := []string{"stash", "show", "-p", "--include-untracked", "--no-color"}
	if ignoreWS {
		args = append(args, "-w")
	}
	return Run(ctx, dir, append(args, ref)...)
}

func StashPop(ctx context.Context, dir, ref string) error {
	_, err := Run(ctx, dir, "stash", "pop", "-q", ref)
	return err
}

func StashApply(ctx context.Context, dir, ref string) error {
	_, err := Run(ctx, dir, "stash", "apply", "-q", ref)
	return err
}

func StashDrop(ctx context.Context, dir, ref string) error {
	_, err := Run(ctx, dir, "stash", "drop", "-q", ref)
	return err
}
