package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/state"
	"github.com/spik3s/pyragit/internal/uistate"
	"github.com/spik3s/pyragit/internal/watch"
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

func fleet(t *testing.T) string {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	one := filepath.Join(root, "one")
	os.MkdirAll(one, 0o755)
	run(t, one, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(one, "a.txt"), []byte("line1\nline2\n"), 0o644)
	run(t, one, "add", "a.txt")
	run(t, one, "commit", "-q", "-m", "init")
	feat := filepath.Join(root, "one-worktrees", "feat")
	run(t, one, "worktree", "add", "-q", "-b", "feat", feat)
	os.WriteFile(filepath.Join(feat, "a.txt"), []byte("line1\nchanged\n"), 0o644)
	os.WriteFile(filepath.Join(feat, "new.txt"), []byte("new\n"), 0o644)
	return root
}

// drive sends msgs through Update, executing returned commands synchronously
// (Batch cmds are expanded) until no commands remain.
func drive(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	queue := append([]tea.Msg{}, msgs...)
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		// Batches may nest; expand them instead of feeding them to Update.
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					if r := c(); r != nil {
						queue = append(queue, r)
					}
				}
			}
			continue
		}
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd == nil {
			continue
		}
		if out := cmd(); out != nil {
			queue = append(queue, out)
		}
	}
	return m
}

func TestAppRendersFleet(t *testing.T) {
	root := fleet(t)
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store}, tea.KeyPressMsg{Code: 'j', Text: "j"})
	view := ansi.Strip(m.View().Content)
	t.Log("\n" + view)
	hhmm := time.Now().Format("15:04")
	for _, want := range []string{"▾ one (2)", "◆ main", "└ feat", "●2", "Unstaged (1)", "Untracked (1)", "+changed", "1 Changes",
		"●2 ! now", "modified " + time.Now().Format("2006-01-02"),
		"feat · 1 unstaged · 1 untracked · no upstream · last commit just now"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
	if ok, _ := regexp.MatchString(`M a\.txt\s+`+hhmm, view); !ok {
		t.Errorf("time column missing for a.txt:\n%s", view)
	}
	if !strings.Contains(view, "f Fetch ") || !strings.Contains(view, "N New wt ") || !strings.Contains(view, "q Quit ") {
		t.Errorf("sidebar key bar missing:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Errorf("view has %d lines, want 24", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 120 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}

	// Move into files pane, select the untracked file, check the diff follows.
	m = drive(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"}, tea.KeyPressMsg{Code: 'j', Text: "j"})
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "+new") {
		t.Errorf("diff did not follow selection:\n%s", view)
	}
	if !strings.Contains(view, "↵ Diff ") || !strings.Contains(view, "y Copy path ") || strings.Contains(view, "f Fetch ") {
		t.Errorf("files key bar wrong:\n%s", view)
	}
	m = drive(t, m, key("l"))
	if v := ansi.Strip(m.View().Content); !strings.Contains(v, "j/k Scroll ") || !strings.Contains(v, "↵ Back ") {
		t.Errorf("diff key bar wrong:\n%s", v)
	}
	m = drive(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.(App).focus != paneFiles {
		t.Error("enter in diff pane should return to files pane")
	}
	// Help overlay.
	m = drive(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if !strings.Contains(ansi.Strip(m.View().Content), "pyragit keys") {
		t.Error("help not shown")
	}
}

func key(s string) tea.KeyPressMsg {
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func TestBaseAndLogTabs(t *testing.T) {
	root := fleet(t)
	feat := filepath.Join(root, "one-worktrees", "feat")
	run(t, feat, "add", "a.txt")
	run(t, feat, "commit", "-q", "-m", "feat: change a")
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Default(root)
	app := New(cfg, cfgPath, false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store}, key("j"), key("2"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "vs main") || !strings.Contains(view, "M a.txt") || !strings.Contains(view, "+changed") {
		t.Errorf("vs base view wrong:\n%s", view)
	}
	m = drive(t, m, key("3"))
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "feat: change a") || !strings.Contains(view, "ahead of main") || !strings.Contains(view, "diff --git") {
		t.Errorf("log view wrong:\n%s", view)
	}
	if !strings.Contains(view, "· t · "+time.Now().Format("15:04")) || !strings.Contains(view, "committed "+time.Now().Format("2006-01-02")) {
		t.Errorf("log times missing:\n%s", view)
	}
	t.Log("\n" + view)
	// Set base branch to feat via the picker: main is now 0 ahead of feat... use main worktree.
	m = drive(t, m, key("k"))
	m = drive(t, m, key("b"))
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "Base branch for one") || !strings.Contains(view, "feat") {
		t.Errorf("picker not shown:\n%s", view)
	}
	m = drive(t, m, key("f"), tea.KeyPressMsg{Code: tea.KeyEnter})
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "set to feat") || !strings.Contains(view, "ahead of feat") {
		t.Errorf("base not applied:\n%s", view)
	}
	data, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(data), `base_branch = "feat"`) {
		t.Errorf("config not saved:\n%s", data)
	}
	// Esc closes the picker without changes.
	m = drive(t, m, key("b"))
	m = drive(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if strings.Contains(ansi.Strip(m.View().Content), "Base branch for") {
		t.Error("picker did not close on esc")
	}
}

func TestWatchEventsRefreshAndRediscover(t *testing.T) {
	root := fleet(t)
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store})
	one := filepath.Join(root, "one")
	// A file edit in the main worktree shows up after a worktree event.
	os.WriteFile(filepath.Join(one, "b.txt"), []byte("b\n"), 0o644)
	m = drive(t, m, watchEventMsg{Kind: watch.KindWorktree, Path: one})
	if !strings.Contains(ansi.Strip(m.View().Content), "? b.txt") {
		t.Errorf("edit not picked up:\n%s", ansi.Strip(m.View().Content))
	}
	// A new worktree appears after rediscovery.
	run(t, one, "worktree", "add", "-q", "-b", "other", filepath.Join(root, "one-worktrees", "other"))
	m = drive(t, m, watchEventMsg{Kind: watch.KindWorktreeList, Path: filepath.Join(one, ".git")})
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "other") || !strings.Contains(view, "one (3)") {
		t.Errorf("new worktree not discovered:\n%s", view)
	}
}

func TestOperations(t *testing.T) {
	tickInterval = 5 * time.Millisecond
	t.Cleanup(func() { tickInterval = time.Second })
	root := fleet(t)
	one := filepath.Join(root, "one")
	remote := filepath.Join(root, "remote.git")
	run(t, root, "init", "-q", "--bare", remote)
	run(t, one, "remote", "add", "origin", remote)
	run(t, one, "push", "-q", "-u", "origin", "main")

	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 30}, loadedMsg{store: store})

	// Fetch via palette.
	m = drive(t, m, key(":"))
	if !strings.Contains(ansi.Strip(m.View().Content), "Command palette") {
		t.Fatal("palette not open")
	}
	m = drive(t, m, key("f"), key("e"), key("t"), key("c"), key("h"), tea.KeyPressMsg{Code: tea.KeyEnter})
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "fetch one done") || !strings.Contains(view, "fetch one ✓") {
		t.Errorf("fetch did not complete:\n%s", view)
	}

	// Commit on main and push with P.
	os.WriteFile(filepath.Join(one, "p.txt"), []byte("p\n"), 0o644)
	run(t, one, "add", "p.txt")
	run(t, one, "commit", "-q", "-m", "push me")
	m = drive(t, m, key("r"))
	if !strings.Contains(ansi.Strip(m.View().Content), "↑1") {
		t.Errorf("ahead badge missing:\n%s", ansi.Strip(m.View().Content))
	}
	m = drive(t, m, key("P"))
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "push main done") || strings.Contains(view, "↑1") {
		t.Errorf("push did not complete:\n%s", view)
	}

	// New worktree via N prompt, then remove it via D + confirm.
	m = drive(t, m, key("N"))
	m = drive(t, m, key("w"), key("i"), key("p"), tea.KeyPressMsg{Code: tea.KeyEnter})
	view = ansi.Strip(m.View().Content)
	wtPath := filepath.Join(root, "one-worktrees", "wip")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree not created:\n%s", view)
	}
	if !strings.Contains(view, "one (3)") || !strings.Contains(view, "wip") {
		t.Errorf("new worktree not shown/selected:\n%s", view)
	}
	app2 := m.(App)
	if w := app2.sidebar.selectedWorktree(); w == nil || w.Path != wtPath {
		t.Errorf("new worktree not selected: %+v", w)
	}
	m = drive(t, m, key("D"))
	if !strings.Contains(ansi.Strip(m.View().Content), "Remove worktree wip") {
		t.Fatalf("confirm not shown:\n%s", ansi.Strip(m.View().Content))
	}
	m = drive(t, m, key("y"))
	view = ansi.Strip(m.View().Content)
	if _, err := os.Stat(wtPath); err == nil {
		t.Errorf("worktree not removed:\n%s", view)
	}
	if strings.Contains(view, "one (3)") {
		t.Errorf("sidebar not updated after removal:\n%s", view)
	}
	// Main worktree cannot be removed.
	m = drive(t, m, key("g"), key("j"), key("D"))
	if !strings.Contains(ansi.Strip(m.View().Content), "cannot remove the main worktree") {
		t.Errorf("main worktree guard missing:\n%s", ansi.Strip(m.View().Content))
	}
	t.Log("\n" + ansi.Strip(m.View().Content))
}

func TestWorktreePathFor(t *testing.T) {
	got := worktreePathFor("", "/home/u/dev/repo", "feat/x")
	if got != "/home/u/dev/repo-worktrees/feat/x" {
		t.Errorf("got %q", got)
	}
	got = worktreePathFor("{repo}/.wt/{branch}", "/r", "b")
	if got != "/r/.wt/b" {
		t.Errorf("got %q", got)
	}
}

func TestDetachedWorktreeLabel(t *testing.T) {
	root := fleet(t)
	one := filepath.Join(root, "one")
	run(t, one, "worktree", "add", "-q", "--detach", filepath.Join(root, "one-worktrees", "det"))
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store})
	view := ansi.Strip(m.View().Content)
	if ok, _ := regexp.MatchString(`detached [0-9a-f]{7}`, view); !ok || strings.Contains(view, "HEAD") {
		t.Errorf("detached label wrong:\n%s", view)
	}
}

func TestPinProject(t *testing.T) {
	root := fleet(t)
	two := filepath.Join(root, "two")
	os.MkdirAll(two, 0o755)
	run(t, two, "init", "-q", "-b", "main")
	run(t, two, "commit", "-q", "--allow-empty", "-m", "init")
	statePath := filepath.Join(t.TempDir(), "state.json")
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	app.StatePath = statePath
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store})
	view := ansi.Strip(m.View().Content)
	if strings.Index(view, "one") > strings.Index(view, "two") {
		t.Fatalf("expected alphabetical order first:\n%s", view)
	}
	// Move to "two" (rows: one, main, feat, two) and pin it.
	m = drive(t, m, key("G"), key("*"))
	view = ansi.Strip(m.View().Content)
	if strings.Index(view, "two ★") > strings.Index(view, "▾ one") || !strings.Contains(view, "pinned two") {
		t.Errorf("pinned project not at top:\n%s", view)
	}
	data, _ := os.ReadFile(statePath)
	if !strings.Contains(string(data), filepath.Join(two, ".git")) {
		t.Errorf("pin not persisted:\n%s", data)
	}
	// Restart with the saved state: pin is restored.
	app2 := New(cfg, "/dev/null", false)
	app2.noWatch = true
	app2.StatePath = statePath
	store2, _ := state.Load(context.Background(), cfg)
	saved, _ := uistate.Load(statePath)
	m2 := drive(t, app2, tea.WindowSizeMsg{Width: 120, Height: 24}, saved, loadedMsg{store: store2})
	view = ansi.Strip(m2.View().Content)
	if strings.Index(view, "two ★") > strings.Index(view, "▾ one") {
		t.Errorf("pin not restored:\n%s", view)
	}
	// Unpin returns to alphabetical order.
	m2 = drive(t, m2, key("g"), key("*"))
	view = ansi.Strip(m2.View().Content)
	if strings.Contains(view, "★") || strings.Index(view, "▾ one") > strings.Index(view, "▾ two") {
		t.Errorf("unpin failed:\n%s", view)
	}
}

func TestBranchLabelFollowsCheckout(t *testing.T) {
	root := fleet(t)
	feat := filepath.Join(root, "one-worktrees", "feat")
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store})
	// Switch the linked worktree to a new branch outside pyragit, then let the
	// watcher-style project event trigger a refresh (not a rediscovery).
	run(t, feat, "checkout", "-q", "-b", "renamed")
	m = drive(t, m, watchEventMsg{Kind: watch.KindProject, Path: filepath.Join(root, "one", ".git")})
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "└ renamed") || strings.Contains(view, "└ feat") {
		t.Errorf("sidebar label did not follow checkout:\n%s", view)
	}
	run(t, feat, "checkout", "-q", "--detach")
	m = drive(t, m, watchEventMsg{Kind: watch.KindProject, Path: filepath.Join(root, "one", ".git")})
	view = ansi.Strip(m.View().Content)
	if ok, _ := regexp.MatchString(`└ detached [0-9a-f]{7}`, view); !ok {
		t.Errorf("sidebar label did not follow detach:\n%s", view)
	}
}

func TestDiskSizes(t *testing.T) {
	root := fleet(t)
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	app.noWatch = true
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 160, Height: 24}, loadedMsg{store: store})
	view := ansi.Strip(m.View().Content)
	if ok, _ := regexp.MatchString(`▾ one \(2\)\s+\d+K`, view); !ok {
		t.Errorf("project total size missing:\n%s", view)
	}
	if ok, _ := regexp.MatchString(`◆ main.*\d+K│`, view); !ok {
		t.Errorf("worktree size missing:\n%s", view)
	}
	if !strings.Contains(view, "KB on disk (incl. .git)") {
		t.Errorf("status size missing:\n%s", view)
	}
}
