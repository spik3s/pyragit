// Package config loads and saves the pyragit TOML configuration.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// RepoConfig holds per-repository overrides, keyed by repo path.
type RepoConfig struct {
	BaseBranch string `toml:"base_branch"`
}

// Config is the on-disk configuration.
type Config struct {
	ScanRoots           []string              `toml:"scan_roots"`
	ScanDepth           int                   `toml:"scan_depth"`
	Repos               []string              `toml:"repos"`
	Exclude             []string              `toml:"exclude"`
	AutoFetchInterval   string                `toml:"auto_fetch_interval"`
	StaleAfterDays      int                   `toml:"stale_after_days"`
	Editor              string                `toml:"editor"`
	WorktreeDirTemplate string                `toml:"worktree_dir_template"`
	RepoConfigs         map[string]RepoConfig `toml:"repos_config"`
}

// Default returns the configuration used when no file exists. cwd becomes the
// single scan root.
func Default(cwd string) Config {
	return Config{
		ScanRoots:           []string{cwd},
		ScanDepth:           2,
		Exclude:             []string{"node_modules", ".cache", "vendor"},
		AutoFetchInterval:   "0",
		StaleAfterDays:      7,
		WorktreeDirTemplate: "{repo_parent}/{repo_name}-worktrees/{branch}",
		RepoConfigs:         map[string]RepoConfig{},
	}
}

// Path returns the config file location, honouring $XDG_CONFIG_HOME.
func Path() string {
	if p := os.Getenv("PYRAGIT_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pyragit", "config.toml")
}

// Load reads path. When the file does not exist it returns Default(cwd) and
// created == true after writing that default to disk.
func Load(path, cwd string) (cfg Config, created bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg = Default(cwd)
		if err := Save(path, cfg); err != nil {
			return cfg, false, err
		}
		return cfg, true, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	cfg = Default(cwd)
	cfg.ScanRoots = nil
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, false, fmt.Errorf("%s: %w", path, err)
	}
	if cfg.RepoConfigs == nil {
		cfg.RepoConfigs = map[string]RepoConfig{}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, false, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, false, nil
}

// Save writes cfg to path, creating parent directories.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Validate checks values that would break the app.
func (c Config) Validate() error {
	if c.ScanDepth < 0 || c.ScanDepth > 10 {
		return errors.New("scan_depth must be between 0 and 10")
	}
	if _, err := c.AutoFetch(); err != nil {
		return err
	}
	if len(c.ScanRoots) == 0 && len(c.Repos) == 0 {
		return errors.New("scan_roots or repos must list at least one path")
	}
	return nil
}

// AutoFetch returns the fetch interval; 0 means disabled.
func (c Config) AutoFetch() (time.Duration, error) {
	s := strings.TrimSpace(c.AutoFetchInterval)
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("auto_fetch_interval: %w", err)
	}
	if d < 0 {
		return 0, errors.New("auto_fetch_interval must not be negative")
	}
	return d, nil
}

// ExpandedScanRoots returns scan roots with ~ expanded and cleaned.
func (c Config) ExpandedScanRoots() []string { return expandAll(c.ScanRoots) }

// ExpandedRepos returns explicit repos with ~ expanded and cleaned.
func (c Config) ExpandedRepos() []string { return expandAll(c.Repos) }

// BaseBranchFor returns the configured base branch override for repoPath, or "".
func (c Config) BaseBranchFor(repoPath string) string {
	for k, rc := range c.RepoConfigs {
		if Expand(k) == repoPath {
			return rc.BaseBranch
		}
	}
	return ""
}

// SetBaseBranch records a per-repo base branch override.
func (c *Config) SetBaseBranch(repoPath, branch string) {
	if c.RepoConfigs == nil {
		c.RepoConfigs = map[string]RepoConfig{}
	}
	for k := range c.RepoConfigs {
		if Expand(k) == repoPath {
			rc := c.RepoConfigs[k]
			rc.BaseBranch = branch
			c.RepoConfigs[k] = rc
			return
		}
	}
	c.RepoConfigs[repoPath] = RepoConfig{BaseBranch: branch}
}

// Expand replaces a leading ~ with the home directory and cleans the path.
func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	return filepath.Clean(os.ExpandEnv(p))
}

func expandAll(ps []string) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, Expand(p))
	}
	return out
}
