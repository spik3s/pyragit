// Package uistate persists small pieces of UI state between runs, such as
// collapsed projects and the last selected worktree.
package uistate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// State is what gets saved.
type State struct {
	Collapsed    []string `json:"collapsed"`   // project common dirs
	Pinned       []string `json:"pinned"`      // project common dirs kept at the top
	Selected     string   `json:"selected"`    // worktree path
	OutputOpen   bool     `json:"output_open"` // output pane visible
	IgnoreWS     bool     `json:"ignore_ws"`   // diff ignores whitespace
	LastFilesTab int      `json:"last_files_tab"`
}

// Path returns the state file location, honouring $XDG_STATE_HOME.
func Path() string {
	if p := os.Getenv("PYRAGIT_STATE"); p != "" {
		return p
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "pyragit", "state.json")
}

// Load reads the state file; a missing file yields an empty state.
func Load(path string) (State, error) {
	var st State
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	return st, nil
}

// Save writes the state file, creating parent directories.
func Save(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
