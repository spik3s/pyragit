package discovery

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	mk := func(rel string) string {
		p := filepath.Join(root, rel)
		os.MkdirAll(p, 0o755)
		run(t, p, "init", "-q", "-b", "main")
		run(t, p, "commit", "-q", "--allow-empty", "-m", "init")
		return p
	}
	alpha := mk("alpha")
	beta := mk("clients/beta")
	mk("too/deep/gamma") // depth 3, must be skipped with Depth: 2
	os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o755)
	run(t, filepath.Join(root, "node_modules", "pkg"), "init", "-q")
	// Linked worktree of alpha placed inside the scan root: must dedupe.
	run(t, alpha, "worktree", "add", "-q", "-b", "feat", filepath.Join(root, "alpha-worktrees", "feat"))
	outside := t.TempDir()
	outside, _ = filepath.EvalSymlinks(outside)
	delta := mk(filepath.Join("..", filepath.Base(outside), "delta")) // outside the root

	ps, errs := Discover(context.Background(), Options{
		Roots: []string{root}, Depth: 2, Explicit: []string{delta}, Exclude: []string{"node_modules"},
	})
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if len(ps) != 3 || names[0] != "alpha" || names[1] != "beta" || names[2] != "delta" {
		t.Fatalf("projects: %v", names)
	}
	if ps[0].Path != alpha || len(ps[0].Worktrees) != 2 || ps[0].Worktrees[1].Branch != "feat" {
		t.Errorf("alpha: %+v", ps[0])
	}
	if ps[1].Path != beta || ps[1].CommonDir != filepath.Join(beta, ".git") {
		t.Errorf("beta: %+v", ps[1])
	}
}

func TestFindRepoDirsRootIsRepo(t *testing.T) {
	dir := t.TempDir()
	run(t, dir, "init", "-q")
	if got := FindRepoDirs(dir, 2, nil); len(got) != 1 || got[0] != filepath.Clean(dir) {
		t.Errorf("got %v", got)
	}
}
