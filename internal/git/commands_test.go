package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mustGit runs git in dir for test setup, failing the test on error.
func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// NewTestRepo creates a repo with one commit on main and returns its path.
func NewTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dir, _ = filepath.EvalSymlinks(dir)
	mustGit(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README"), []byte("hi\n"), 0o644)
	mustGit(t, dir, "add", "README")
	mustGit(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func TestStatusAndWorktrees(t *testing.T) {
	ctx := context.Background()
	repo := NewTestRepo(t)
	os.WriteFile(filepath.Join(repo, "new.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(repo, "README"), []byte("changed\n"), 0o644)

	st, err := GetStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || st.UntrackedCount() != 1 || st.UnstagedCount() != 1 {
		t.Errorf("status: %+v", st)
	}

	wtPath := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-feat")
	if err := WorktreeAdd(ctx, repo, wtPath, "feat", "", true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(wtPath) })
	wts, err := Worktrees(ctx, repo)
	if err != nil || len(wts) != 2 || wts[1].Branch != "feat" {
		t.Fatalf("worktrees: %+v %v", wts, err)
	}
	cd1, _ := CommonDir(ctx, repo)
	cd2, _ := CommonDir(ctx, wtPath)
	if cd1 != cd2 || cd1 != filepath.Join(repo, ".git") {
		t.Errorf("common dir mismatch: %q %q", cd1, cd2)
	}
	if top, _ := TopLevel(ctx, filepath.Join(wtPath)); top != wtPath {
		t.Errorf("toplevel %q want %q", top, wtPath)
	}

	// Commit on feat, then check ahead/behind vs main and the branch diff.
	os.WriteFile(filepath.Join(wtPath, "feat.txt"), []byte("f\n"), 0o644)
	mustGit(t, wtPath, "add", "feat.txt")
	mustGit(t, wtPath, "commit", "-q", "-m", "feat commit")
	ahead, behind, err := AheadBehind(ctx, wtPath, "main", "HEAD")
	if err != nil || ahead != 1 || behind != 0 {
		t.Errorf("ahead/behind: %d %d %v", ahead, behind, err)
	}
	mb, _ := MergeBase(ctx, wtPath, "main", "HEAD")
	fs, err := DiffFiles(ctx, wtPath, mb, "HEAD")
	if err != nil || len(fs) != 1 || fs[0].Path != "feat.txt" || fs[0].Status != 'A' {
		t.Errorf("diff files: %+v %v", fs, err)
	}
	cs, err := Log(ctx, wtPath, mb+"..HEAD", 0)
	if err != nil || len(cs) != 1 || cs[0].Subject != "feat commit" {
		t.Errorf("log: %+v %v", cs, err)
	}
	if d, err := DiffRange(ctx, wtPath, mb, "HEAD", "feat.txt", false); err != nil || !strings.Contains(d, "+f") {
		t.Errorf("diff range: %q %v", d, err)
	}
	if d, err := Show(ctx, wtPath, cs[0].Hash, false); err != nil || !strings.Contains(d, "feat commit") {
		t.Errorf("show: %v", err)
	}
	if DefaultBranch(ctx, wtPath) != "main" {
		t.Errorf("default branch: %q", DefaultBranch(ctx, wtPath))
	}
	bs, err := Branches(ctx, wtPath)
	if err != nil || len(bs) != 2 {
		t.Errorf("branches: %+v %v", bs, err)
	}

	// Working tree diffs in the main repo.
	if d, err := Diff(ctx, repo, "README", DiffUnstaged, false); err != nil || !strings.Contains(d, "+changed") {
		t.Errorf("unstaged diff: %q %v", d, err)
	}
	if d, err := Diff(ctx, repo, "new.txt", DiffUntracked, false); err != nil || !strings.Contains(d, "+x") {
		t.Errorf("untracked diff: %q %v", d, err)
	}

	if err := WorktreeRemove(ctx, repo, wtPath, false); err != nil {
		t.Fatal(err)
	}
	if wts, _ := Worktrees(ctx, repo); len(wts) != 1 {
		t.Errorf("worktree not removed: %+v", wts)
	}
}

func TestRunErrorAndStream(t *testing.T) {
	ctx := context.Background()
	repo := NewTestRepo(t)
	_, err := Run(ctx, repo, "checkout", "nope")
	var ge *Error
	if !asError(err, &ge) || ge.ExitCode == 0 || !strings.Contains(ge.Error(), "nope") {
		t.Errorf("expected *Error, got %v", err)
	}
	var lines []string
	err = RunStream(ctx, repo, func(s string) { lines = append(lines, s) }, "log", "--oneline")
	if err != nil || len(lines) != 1 {
		t.Errorf("stream: %v %v", lines, err)
	}
	err = RunStream(ctx, repo, func(string) {}, "fetch", "nonexistent-remote")
	if !asError(err, &ge) {
		t.Errorf("stream error: %v", err)
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}
