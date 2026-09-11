// Command pyragit is a terminal UI for managing many git projects and their worktrees.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/state"
	"github.com/spik3s/pyragit/internal/ui"
	"github.com/spik3s/pyragit/internal/uistate"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	dump := flag.Bool("dump", false, "print discovered projects and worktree status as text, then exit")
	cfgPath := flag.String("config", config.Path(), "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("pyragit", version)
		return
	}

	cwd, _ := os.Getwd()
	cfg, created, err := config.Load(*cfgPath, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pyragit:", err)
		os.Exit(1)
	}

	if *dump {
		os.Exit(runDump(cfg, created, *cfgPath))
	}

	app := ui.New(cfg, *cfgPath, created)
	app.StatePath = uistate.Path()
	if _, err := tea.NewProgram(app).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "pyragit:", err)
		os.Exit(1)
	}
}

func runDump(cfg config.Config, created bool, cfgPath string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if created {
		fmt.Printf("wrote default config to %s\n", cfgPath)
	}
	start := time.Now()
	store, errs := state.Load(ctx, cfg)
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	var mu sync.Mutex
	state.RefreshAll(ctx, store.All(), 8, func(snap state.Snapshot) {
		mu.Lock()
		defer mu.Unlock()
		store.Apply(snap)
	})
	for _, p := range store.Projects {
		fmt.Printf("%s  (%s) base=%s\n", p.Name, p.Path, p.BaseBranch)
		for _, w := range p.Worktrees {
			s := w.Snap
			if s.Err != nil {
				fmt.Printf("    %-30s ERROR %v\n", w.Branch, s.Err)
				continue
			}
			branch := s.Status.Branch
			if s.Status.Detached {
				branch = "(detached " + shortHash(w.Head) + ")"
			}
			fmt.Printf("    %-30s dirty=%d staged=%d untracked=%d conflicts=%d ahead=%d behind=%d upstream=%q vsBase=+%d/-%d last=%s  %s\n",
				branch, s.Status.UnstagedCount(), s.Status.StagedCount(), s.Status.UntrackedCount(), s.Status.ConflictCount(),
				s.Status.Ahead, s.Status.Behind, s.Status.Upstream, s.AheadBase, s.BehindBase,
				s.LastCommit.Format("2006-01-02"), w.Path)
		}
	}
	fmt.Printf("%d projects, %d worktrees in %s\n", len(store.Projects), len(store.All()), time.Since(start).Round(time.Millisecond))
	return 0
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}
