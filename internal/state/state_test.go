package state

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/discovery"
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

// fleet: repo "one" with main + feat worktree (feat has 1 commit ahead and a
// dirty file); repo "two" clean on main.
func fleet(t *testing.T) (root string) {
	root, _ = filepath.EvalSymlinks(t.TempDir())
	one := filepath.Join(root, "one")
	os.MkdirAll(one, 0o755)
	run(t, one, "init", "-q", "-b", "main")
	run(t, one, "commit", "-q", "--allow-empty", "-m", "init")
	feat := filepath.Join(root, "one-worktrees", "feat")
	run(t, one, "worktree", "add", "-q", "-b", "feat", feat)
	run(t, feat, "commit", "-q", "--allow-empty", "-m", "feat work")
	os.WriteFile(filepath.Join(feat, "dirty.txt"), []byte("x"), 0o644)
	two := filepath.Join(root, "two")
	os.MkdirAll(two, 0o755)
	run(t, two, "init", "-q", "-b", "master")
	run(t, two, "commit", "-q", "--allow-empty", "-m", "init")
	return root
}

func TestLoadAndRefreshAll(t *testing.T) {
	root := fleet(t)
	cfg := config.Default(root)
	cfg.RepoConfigs = map[string]config.RepoConfig{}
	ctx := context.Background()
	s, errs := Load(ctx, cfg)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(s.Projects) != 2 || s.Projects[0].Name != "one" || s.Projects[0].BaseBranch != "main" || s.Projects[1].BaseBranch != "master" {
		t.Fatalf("projects: %+v %+v", s.Projects[0], s.Projects[1])
	}
	all := s.All()
	if len(all) != 3 || !all[0].IsMain() || all[1].IsMain() {
		t.Fatalf("worktrees: %d", len(all))
	}
	var mu sync.Mutex
	RefreshAll(ctx, all, 2, func(snap Snapshot) {
		mu.Lock()
		defer mu.Unlock()
		s.Apply(snap)
	})
	feat := s.Get(filepath.Join(root, "one-worktrees", "feat"))
	if feat == nil || !feat.Loaded || feat.Snap.Err != nil {
		t.Fatalf("feat: %+v", feat)
	}
	if feat.Snap.AheadBase != 1 || feat.Snap.BehindBase != 0 || feat.Snap.Status.UntrackedCount() != 1 || feat.Snap.LastCommit.IsZero() {
		t.Errorf("feat snapshot: %+v", feat.Snap)
	}
	if mt := feat.Snap.FileTimes["dirty.txt"]; mt.IsZero() || feat.Snap.LastChange != mt || feat.Snap.LastActivity() != mt {
		t.Errorf("file times: %+v lastChange=%v activity=%v", feat.Snap.FileTimes, feat.Snap.LastChange, feat.Snap.LastActivity())
	}
	if main := s.Get(filepath.Join(root, "one")); !main.Snap.LastActivity().Equal(main.Snap.LastCommit) {
		t.Errorf("clean worktree activity should be last commit: %+v", main.Snap)
	}
	main := s.Get(filepath.Join(root, "one"))
	if main.Snap.AheadBase != 0 || main.Snap.Status.Dirty() {
		t.Errorf("main snapshot: %+v", main.Snap)
	}

	// Replace keeps snapshots and collapsed state.
	s.Projects[0].Collapsed = true
	projects, _ := discovery.Discover(ctx, DiscoverOptions(cfg))
	s.Replace(projects, BaseResolver(ctx, cfg))
	if !s.Projects[0].Collapsed || !s.Get(feat.Path).Loaded {
		t.Error("Replace lost state")
	}
}

func TestApplyUpdatesBranch(t *testing.T) {
	root := fleet(t)
	feat := filepath.Join(root, "one-worktrees", "feat")
	s, _ := Load(context.Background(), config.Default(root))
	run(t, feat, "checkout", "-q", "-b", "renamed")
	s.Apply(Refresh(context.Background(), feat, "main"))
	w := s.Get(feat)
	if w.Branch != "renamed" || w.Detached {
		t.Errorf("branch not updated: %+v", w.Worktree)
	}
	run(t, feat, "checkout", "-q", "--detach")
	s.Apply(Refresh(context.Background(), feat, "main"))
	if w.Branch != "" || !w.Detached || w.Head == "" {
		t.Errorf("detached state not updated: %+v", w.Worktree)
	}
}

func TestRefreshMissingDir(t *testing.T) {
	snap := Refresh(context.Background(), "/nonexistent/path", "main")
	if snap.Err == nil {
		t.Error("expected error")
	}
}
