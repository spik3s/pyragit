// Package state holds the in-memory model of projects and worktrees that the
// UI renders from, plus the refresh logic that populates it.
package state

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spik3s/pyragit/internal/discovery"
	"github.com/spik3s/pyragit/internal/git"
)

// Snapshot is the result of refreshing one worktree.
type Snapshot struct {
	Path        string
	Status      git.Status
	AheadBase   int // commits on this branch not on the base branch
	BehindBase  int
	LastCommit  time.Time
	FileTimes   map[string]time.Time // modification time per changed path
	LastChange  time.Time            // newest FileTimes entry
	Err         error
	RefreshedAt time.Time
}

// LastActivity is the newer of the last commit and the last file change.
func (s Snapshot) LastActivity() time.Time {
	if s.LastChange.After(s.LastCommit) {
		return s.LastChange
	}
	return s.LastCommit
}

// Worktree is a git worktree plus its latest snapshot.
type Worktree struct {
	git.Worktree
	Project   *Project
	Snap      Snapshot
	Loaded    bool
	Loading   bool
	Size      int64 // bytes on disk; SizeKnown reports whether it has been measured
	SizeKnown bool
}

// TotalSize sums the measured sizes of a project's worktrees. known is false
// while any worktree is still unmeasured.
func (p *Project) TotalSize() (total int64, known bool) {
	known = true
	for _, w := range p.Worktrees {
		if !w.SizeKnown {
			known = false
			continue
		}
		total += w.Size
	}
	return total, known
}

// SetSize records a measured disk size.
func (s *Store) SetSize(path string, size int64) {
	if w := s.byPath[path]; w != nil {
		w.Size, w.SizeKnown = size, true
	}
}

// IsMain reports whether this is the repository's primary worktree.
func (w *Worktree) IsMain() bool { return w.Project != nil && w.Project.Path == w.Path }

// Project is a repository with its worktrees and UI state.
type Project struct {
	discovery.Project
	BaseBranch string
	Collapsed  bool
	Pinned     bool // shown at the top of the list
	Worktrees  []*Worktree
}

// Store is the root of the model.
type Store struct {
	Projects []*Project
	byPath   map[string]*Worktree
}

// New builds a store from discovered projects. baseFor returns the base
// branch to use for a project (already resolved from config or detection).
func New(projects []discovery.Project, baseFor func(discovery.Project) string) *Store {
	s := &Store{byPath: map[string]*Worktree{}}
	for _, dp := range projects {
		p := &Project{Project: dp}
		if baseFor != nil {
			p.BaseBranch = baseFor(dp)
		}
		for _, wt := range dp.Worktrees {
			if wt.Bare {
				continue
			}
			w := &Worktree{Worktree: wt, Project: p}
			p.Worktrees = append(p.Worktrees, w)
			s.byPath[wt.Path] = w
		}
		s.Projects = append(s.Projects, p)
	}
	return s
}

// Get returns the worktree at path, or nil.
func (s *Store) Get(path string) *Worktree { return s.byPath[path] }

// All returns every worktree in display order.
func (s *Store) All() []*Worktree {
	var out []*Worktree
	for _, p := range s.Projects {
		out = append(out, p.Worktrees...)
	}
	return out
}

// Apply stores a snapshot for its worktree. Unknown paths are ignored. The
// worktree's branch, head and detached flag follow the snapshot so a checkout
// made outside pyragit shows up without a rediscovery.
func (s *Store) Apply(snap Snapshot) {
	w := s.byPath[snap.Path]
	if w == nil {
		return
	}
	w.Snap = snap
	w.Loaded = true
	w.Loading = false
	if snap.Err == nil {
		w.Branch = snap.Status.Branch
		w.Detached = snap.Status.Detached
		if snap.Status.OID != "" {
			w.Head = snap.Status.OID
		}
	}
}

// SetLoading marks a worktree as refreshing.
func (s *Store) SetLoading(path string, loading bool) {
	if w := s.byPath[path]; w != nil {
		w.Loading = loading
	}
}

// Replace swaps in a newly discovered project list, keeping snapshots and UI
// state for worktrees and projects that still exist.
func (s *Store) Replace(projects []discovery.Project, baseFor func(discovery.Project) string) {
	old := s.byPath
	oldProjects := map[string]*Project{}
	for _, p := range s.Projects {
		oldProjects[p.CommonDir] = p
	}
	n := New(projects, baseFor)
	for _, p := range n.Projects {
		if op := oldProjects[p.CommonDir]; op != nil {
			p.Collapsed = op.Collapsed
			p.Pinned = op.Pinned
			if op.BaseBranch != "" {
				p.BaseBranch = op.BaseBranch
			}
		}
		for _, w := range p.Worktrees {
			if ow := old[w.Path]; ow != nil {
				w.Snap, w.Loaded, w.Loading = ow.Snap, ow.Loaded, ow.Loading
				w.Size, w.SizeKnown = ow.Size, ow.SizeKnown
			}
		}
	}
	s.Projects, s.byPath = n.Projects, n.byPath
}

// Refresh gathers status, base-branch distance and last commit time for one
// worktree. Errors are recorded in the snapshot rather than returned so a
// broken worktree still renders.
func Refresh(ctx context.Context, path, baseBranch string) Snapshot {
	snap := Snapshot{Path: path, RefreshedAt: time.Now()}
	st, err := git.GetStatus(ctx, path)
	if err != nil {
		snap.Err = err
		return snap
	}
	snap.Status = st
	snap.FileTimes = make(map[string]time.Time, len(st.Files))
	for _, f := range st.Files {
		if fi, err := os.Stat(filepath.Join(path, f.Path)); err == nil {
			snap.FileTimes[f.Path] = fi.ModTime()
			if fi.ModTime().After(snap.LastChange) {
				snap.LastChange = fi.ModTime()
			}
		}
	}
	if c, err := git.HeadCommit(ctx, path); err == nil {
		snap.LastCommit = c.When
	}
	if baseBranch != "" && st.Branch != baseBranch && git.RefExists(ctx, path, baseBranch) {
		if a, b, err := git.AheadBehind(ctx, path, baseBranch, "HEAD"); err == nil {
			snap.AheadBase, snap.BehindBase = a, b
		}
	}
	return snap
}

// RefreshAll refreshes every worktree in s concurrently with at most workers
// goroutines and calls emit for each snapshot as it completes. emit may be
// called from multiple goroutines.
func RefreshAll(ctx context.Context, wts []*Worktree, workers int, emit func(Snapshot)) {
	if workers < 1 {
		workers = 1
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, w := range wts {
		path, base := w.Path, w.Project.BaseBranch
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			emit(Refresh(ctx, path, base))
		}()
	}
	wg.Wait()
}
