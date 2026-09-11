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

func TestDiscoverNestedAndSubmodule(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	parent := filepath.Join(root, "parent")
	os.MkdirAll(parent, 0o755)
	run(t, parent, "init", "-q", "-b", "main")
	run(t, parent, "commit", "-q", "--allow-empty", "-m", "init")
	// A plain repo nested inside the parent's working tree.
	nested := filepath.Join(parent, "projects", "nested")
	os.MkdirAll(nested, 0o755)
	run(t, nested, "init", "-q", "-b", "main")
	run(t, nested, "commit", "-q", "--allow-empty", "-m", "init")
	// A submodule: its .git is a file pointing into parent/.git/modules.
	src := filepath.Join(root, "sub-src")
	os.MkdirAll(src, 0o755)
	run(t, src, "init", "-q", "-b", "main")
	run(t, src, "commit", "-q", "--allow-empty", "-m", "init")
	run(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "-q", src, "projects/sub")
	sub := filepath.Join(parent, "projects", "sub")
	run(t, sub, "worktree", "add", "-q", "-b", "feat", filepath.Join(sub, ".claude", "worktrees", "feat"))

	ps, errs := Discover(context.Background(), Options{Roots: []string{root}, Depth: 3})
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	byName := map[string]Project{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	if _, ok := byName["nested"]; !ok {
		t.Errorf("nested repo not discovered: %v", names(ps))
	}
	sp, ok := byName["sub"]
	if !ok {
		t.Fatalf("submodule not discovered: %v", names(ps))
	}
	if sp.Path != sub {
		t.Errorf("submodule path = %q, want %q", sp.Path, sub)
	}
	if len(sp.Worktrees) != 2 || sp.Worktrees[0].Path != sub || sp.Worktrees[1].Branch != "feat" {
		t.Errorf("submodule worktrees: %+v", sp.Worktrees)
	}
	if len(ps) != 4 {
		t.Errorf("want 4 projects (parent, nested, sub, sub-src), got %v", names(ps))
	}
}

func names(ps []Project) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}
