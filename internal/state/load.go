package state

import (
	"context"

	"github.com/spik3s/pyragit/internal/config"
	"github.com/spik3s/pyragit/internal/discovery"
	"github.com/spik3s/pyragit/internal/git"
)

// DiscoverOptions builds discovery options from cfg.
func DiscoverOptions(cfg config.Config) discovery.Options {
	return discovery.Options{
		Roots:    cfg.ExpandedScanRoots(),
		Depth:    cfg.ScanDepth,
		Explicit: cfg.ExpandedRepos(),
		Exclude:  cfg.Exclude,
	}
}

// BaseResolver returns a function that picks a project's base branch: the
// config override if set, otherwise git's best guess.
func BaseResolver(ctx context.Context, cfg config.Config) func(discovery.Project) string {
	return func(p discovery.Project) string {
		if b := cfg.BaseBranchFor(p.Path); b != "" {
			return b
		}
		return git.DefaultBranch(ctx, p.Path)
	}
}

// Load discovers projects per cfg and builds a store with base branches
// resolved. Snapshots are not loaded yet; call RefreshAll for that.
func Load(ctx context.Context, cfg config.Config) (*Store, []error) {
	projects, errs := discovery.Discover(ctx, DiscoverOptions(cfg))
	return New(projects, BaseResolver(ctx, cfg)), errs
}
