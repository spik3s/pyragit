// Package git is a thin wrapper over the system git binary with parsers for
// its porcelain output. All parsers are pure functions over strings.
package git

import "time"

// FileStatus is one entry from `git status --porcelain=v2`.
type FileStatus struct {
	Path      string
	OrigPath  string // set for renames/copies
	Staged    byte   // index status: '.', 'M', 'A', 'D', 'R', 'C', 'U'
	Unstaged  byte   // worktree status
	Untracked bool
	Conflict  bool
}

// Status is the parsed result of `git status --porcelain=v2 --branch`.
type Status struct {
	OID      string
	Branch   string // empty when detached
	Upstream string
	Ahead    int
	Behind   int
	Detached bool
	Files    []FileStatus
}

// HasUpstream reports whether the branch tracks a remote branch.
func (s Status) HasUpstream() bool { return s.Upstream != "" }

// Dirty reports whether there are any tracked changes or untracked files.
func (s Status) Dirty() bool { return len(s.Files) > 0 }

// StagedCount counts files with a staged change.
func (s Status) StagedCount() int {
	n := 0
	for _, f := range s.Files {
		if !f.Untracked && !f.Conflict && f.Staged != '.' {
			n++
		}
	}
	return n
}

// UnstagedCount counts files with an unstaged change.
func (s Status) UnstagedCount() int {
	n := 0
	for _, f := range s.Files {
		if !f.Untracked && !f.Conflict && f.Unstaged != '.' {
			n++
		}
	}
	return n
}

// UntrackedCount counts untracked files.
func (s Status) UntrackedCount() int {
	n := 0
	for _, f := range s.Files {
		if f.Untracked {
			n++
		}
	}
	return n
}

// ConflictCount counts unmerged files.
func (s Status) ConflictCount() int {
	n := 0
	for _, f := range s.Files {
		if f.Conflict {
			n++
		}
	}
	return n
}

// Worktree is one entry from `git worktree list --porcelain`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short name, empty when detached or bare
	Bare     bool
	Detached bool
	Locked   string // reason, or "locked" when no reason given; empty when unlocked
	Prunable string // reason; empty when not prunable
}

// Commit is one entry from `git log`.
type Commit struct {
	Hash    string
	Short   string
	Author  string
	When    time.Time
	Subject string
}

// DiffFile is one entry from `git diff --name-status`.
type DiffFile struct {
	Status   byte // 'M', 'A', 'D', 'R', 'C', 'T', 'U'
	Path     string
	OrigPath string
}

// Branch is one local branch from `git branch`.
type Branch struct {
	Name     string
	Upstream string
	Current  bool
}
