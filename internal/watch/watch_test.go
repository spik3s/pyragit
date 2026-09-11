package watch

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
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

func expect(t *testing.T, w *Watcher, want Event) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-w.Events():
			if ev == want {
				return
			}
			t.Logf("skipping event %+v", ev)
		case <-deadline:
			t.Fatalf("timed out waiting for %+v", want)
		}
	}
}

func TestWatcher(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	repo := filepath.Join(root, "repo")
	os.MkdirAll(filepath.Join(repo, "src"), 0o755)
	os.MkdirAll(filepath.Join(repo, "node_modules", "x"), 0o755)
	run(t, repo, "init", "-q", "-b", "main")
	run(t, repo, "commit", "-q", "--allow-empty", "-m", "init")
	common := filepath.Join(repo, ".git")

	w := New(100*time.Millisecond, []string{"node_modules"})
	defer w.Close()
	if err := w.Watch([]string{repo}, []string{common}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let FSEvents settle

	os.WriteFile(filepath.Join(repo, "src", "a.go"), []byte("x"), 0o644)
	expect(t, w, Event{Kind: KindWorktree, Path: repo})

	run(t, repo, "add", "src/a.go")
	expect(t, w, Event{Kind: KindProject, Path: common})

	// Excluded dir produces nothing within the debounce window.
	drain(w)
	os.WriteFile(filepath.Join(repo, "node_modules", "x", "b.js"), []byte("x"), 0o644)
	select {
	case ev := <-w.Events():
		t.Errorf("unexpected event %+v for excluded path", ev)
	case <-time.After(400 * time.Millisecond):
	}

	wt := filepath.Join(root, "repo-feat")
	run(t, repo, "worktree", "add", "-q", "-b", "feat", wt)
	expect(t, w, Event{Kind: KindWorktreeList, Path: common})

	if err := w.Watch([]string{repo, wt}, []string{common}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	drain(w)
	os.WriteFile(filepath.Join(wt, "c.txt"), []byte("x"), 0o644)
	expect(t, w, Event{Kind: KindWorktree, Path: wt})
}

func drain(w *Watcher) {
	for {
		select {
		case <-w.Events():
		default:
			return
		}
	}
}
