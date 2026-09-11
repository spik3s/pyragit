package git

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseStatus parses `git status --porcelain=v2 --branch` output.
func ParseStatus(out string) (Status, error) {
	var st Status
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case '#':
			parseStatusHeader(&st, line)
		case '1', '2', 'u':
			f, err := parseStatusEntry(line)
			if err != nil {
				return st, err
			}
			st.Files = append(st.Files, f)
		case '?':
			st.Files = append(st.Files, FileStatus{Path: line[2:], Untracked: true})
		case '!':
			// ignored file; not tracked in the UI
		}
	}
	return st, nil
}

func parseStatusHeader(st *Status, line string) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return
	}
	switch fields[1] {
	case "branch.oid":
		st.OID = fields[2]
	case "branch.head":
		if fields[2] == "(detached)" {
			st.Detached = true
		} else {
			st.Branch = fields[2]
		}
	case "branch.upstream":
		st.Upstream = fields[2]
	case "branch.ab":
		if len(fields) >= 4 {
			st.Ahead, _ = strconv.Atoi(strings.TrimPrefix(fields[2], "+"))
			st.Behind, _ = strconv.Atoi(strings.TrimPrefix(fields[3], "-"))
		}
	}
}

func parseStatusEntry(line string) (FileStatus, error) {
	// Field counts before the path, per git-status(1) porcelain v2.
	var nFields int
	switch line[0] {
	case '1':
		nFields = 8
	case '2':
		nFields = 9
	case 'u':
		nFields = 10
	}
	fields := strings.SplitN(line, " ", nFields+1)
	if len(fields) != nFields+1 {
		return FileStatus{}, fmt.Errorf("git status: malformed entry %q", line)
	}
	xy := fields[1]
	if len(xy) != 2 {
		return FileStatus{}, fmt.Errorf("git status: malformed XY in %q", line)
	}
	f := FileStatus{Staged: xy[0], Unstaged: xy[1], Path: fields[nFields]}
	switch line[0] {
	case '2':
		if i := strings.IndexByte(f.Path, '\t'); i >= 0 {
			f.Path, f.OrigPath = f.Path[:i], f.Path[i+1:]
		}
	case 'u':
		f.Conflict = true
	}
	return f, nil
}

// ParseWorktrees parses `git worktree list --porcelain` output.
func ParseWorktrees(out string) ([]Worktree, error) {
	var wts []Worktree
	var cur *Worktree
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			cur = nil
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		if key == "worktree" {
			wts = append(wts, Worktree{Path: val})
			cur = &wts[len(wts)-1]
			continue
		}
		if cur == nil {
			return nil, fmt.Errorf("git worktree: attribute %q before any worktree", line)
		}
		switch key {
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "locked":
			if val == "" {
				val = "locked"
			}
			cur.Locked = val
		case "prunable":
			if val == "" {
				val = "prunable"
			}
			cur.Prunable = val
		}
	}
	return wts, nil
}

// ParseAheadBehind parses `git rev-list --left-right --count A...B` output.
func ParseAheadBehind(out string) (ahead, behind int, err error) {
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("git rev-list: unexpected output %q", out)
	}
	if ahead, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, fmt.Errorf("git rev-list: %w", err)
	}
	if behind, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, fmt.Errorf("git rev-list: %w", err)
	}
	return ahead, behind, nil
}

// LogFormat is the --format argument that ParseLog understands.
const LogFormat = "%H%x00%h%x00%an%x00%at%x00%s%x00"

// ParseLog parses `git log --format=LogFormat` output.
func ParseLog(out string) ([]Commit, error) {
	var cs []Commit
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\x00")
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 5 {
			return nil, fmt.Errorf("git log: malformed line %q", line)
		}
		ts, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git log: bad timestamp in %q", line)
		}
		cs = append(cs, Commit{
			Hash: fields[0], Short: fields[1], Author: fields[2],
			When: time.Unix(ts, 0), Subject: fields[4],
		})
	}
	return cs, nil
}

// ParseNameStatus parses `git diff --name-status` output.
func ParseNameStatus(out string) ([]DiffFile, error) {
	var fs []DiffFile
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || fields[0] == "" {
			return nil, fmt.Errorf("git diff: malformed line %q", line)
		}
		f := DiffFile{Status: fields[0][0], Path: fields[1]}
		if (f.Status == 'R' || f.Status == 'C') && len(fields) == 3 {
			f.OrigPath, f.Path = fields[1], fields[2]
		}
		fs = append(fs, f)
	}
	return fs, nil
}

// BranchFormat is the --format argument that ParseBranches understands.
const BranchFormat = "%(refname:short)%09%(upstream:short)%09%(HEAD)"

// ParseBranches parses `git branch --format=BranchFormat` output.
func ParseBranches(out string) ([]Branch, error) {
	var bs []Branch
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			return nil, fmt.Errorf("git branch: malformed line %q", line)
		}
		bs = append(bs, Branch{Name: fields[0], Upstream: fields[1], Current: fields[2] == "*"})
	}
	return bs, nil
}
