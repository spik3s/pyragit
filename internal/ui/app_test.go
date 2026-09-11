package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/state"
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
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd == nil {
			continue
		}
		out := cmd()
		switch o := out.(type) {
		case nil:
		case tea.BatchMsg:
			for _, c := range o {
				if c != nil {
					if r := c(); r != nil {
						queue = append(queue, r)
					}
				}
			}
		default:
			queue = append(queue, out)
		}
	}
	return m
}

func TestAppRendersFleet(t *testing.T) {
	root := fleet(t)
	cfg := config.Default(root)
	app := New(cfg, "/dev/null", false)
	store, _ := state.Load(context.Background(), cfg)
	m := drive(t, app, tea.WindowSizeMsg{Width: 120, Height: 24}, loadedMsg{store: store}, tea.KeyPressMsg{Code: 'j', Text: "j"})
	view := ansi.Strip(m.View().Content)
	t.Log("\n" + view)
	for _, want := range []string{"▾ one (2)", "main", "feat", "●2", "Unstaged (1)", "Untracked (1)", "+changed", "1 Changes"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
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
