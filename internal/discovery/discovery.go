// Package discovery finds git repositories under configured roots and groups
// their worktrees into projects.
package discovery

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spik3s/pyragit/internal/git"
)

// Project is one repository with all of its worktrees. Path is the main
// worktree (or the bare repo directory); CommonDir identifies the repository.
type Project struct {
	Name      string
	Path      string
	CommonDir string
	Worktrees []git.Worktree
}

// Options controls a scan.
type Options struct {
	Roots    []string // directories to walk
	Depth    int      // how many levels below a root to look (0 = root itself only)
	Explicit []string // repo paths to include regardless of roots
	Exclude  []string // directory base names or glob patterns to skip while walking
}

// Discover scans according to opts. Errors from individual repos are
// collected in the returned slice so one broken checkout does not hide the rest.
func Discover(ctx context.Context, opts Options) ([]Project, []error) {
	var candidates []string
	for _, root := range opts.Roots {
		candidates = append(candidates, FindRepoDirs(root, opts.Depth, opts.Exclude)...)
	}
	candidates = append(candidates, opts.Explicit...)

	var projects []Project
	var errs []error
	seen := map[string]bool{}
	for _, dir := range candidates {
		if ctx.Err() != nil {
			break
		}
		common, err := git.CommonDir(ctx, dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if seen[common] {
			continue
		}
		seen[common] = true
		wts, err := git.Worktrees(ctx, dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		p := Project{CommonDir: common, Worktrees: wts}
		if len(wts) > 0 {
			p.Path = wts[0].Path
		} else {
			p.Path = dir
		}
		p.Name = filepath.Base(p.Path)
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name != projects[j].Name {
			return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
		}
		return projects[i].Path < projects[j].Path
	})
	return projects, errs
}

// FindRepoDirs walks root up to depth levels and returns directories that
// contain a .git entry (directory for normal repos, file for linked worktrees
// and submodules). It does not descend into a repo once found.
func FindRepoDirs(root string, depth int, exclude []string) []string {
	root = filepath.Clean(root)
	if isRepo(root) {
		return []string{root}
	}
	var found []string
	var walk func(dir string, level int)
	walk = func(dir string, level int) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || excluded(e.Name(), exclude) {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if isRepo(p) {
				found = append(found, p)
				continue
			}
			if level < depth {
				walk(p, level+1)
			}
		}
	}
	walk(root, 1)
	return found
}

func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func excluded(name string, patterns []string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, pat := range patterns {
		pat = strings.TrimPrefix(pat, "**/")
		if ok, _ := filepath.Match(pat, name); ok || pat == name {
			return true
		}
	}
	return false
}
