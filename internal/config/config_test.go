package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "config.toml")
	cfg, created, err := Load(p, "/work")
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if len(cfg.ScanRoots) != 1 || cfg.ScanRoots[0] != "/work" || cfg.ScanDepth != 2 {
		t.Errorf("default: %+v", cfg)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "scan_roots") {
		t.Errorf("file not written: %s", data)
	}
	cfg2, created, err := Load(p, "/other")
	if err != nil || created || cfg2.ScanRoots[0] != "/work" {
		t.Errorf("reload: %+v %v %v", cfg2, created, err)
	}
}

func TestLoadParsesOverrides(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte(`
scan_roots = ["~/Developer"]
scan_depth = 3
repos = ["~/x"]
auto_fetch_interval = "5m"
stale_after_days = 14

[repos_config."~/Developer/foo"]
base_branch = "develop"
`), 0o644)
	cfg, _, err := Load(p, "/cwd")
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ExpandedScanRoots(); got[0] != "/home/u/Developer" {
		t.Errorf("roots: %v", got)
	}
	if got := cfg.ExpandedRepos(); got[0] != "/home/u/x" {
		t.Errorf("repos: %v", got)
	}
	if d, _ := cfg.AutoFetch(); d != 5*time.Minute {
		t.Errorf("auto fetch: %v", d)
	}
	if cfg.StaleAfterDays != 14 || cfg.ScanDepth != 3 {
		t.Errorf("cfg: %+v", cfg)
	}
	if cfg.BaseBranchFor("/home/u/Developer/foo") != "develop" || cfg.BaseBranchFor("/nope") != "" {
		t.Errorf("base branch lookup failed")
	}
	// Defaults still apply for unset keys.
	if cfg.WorktreeDirTemplate == "" || len(cfg.Exclude) == 0 {
		t.Errorf("defaults lost: %+v", cfg)
	}
	cfg.SetBaseBranch("/home/u/Developer/foo", "trunk")
	cfg.SetBaseBranch("/home/u/bar", "dev")
	if cfg.BaseBranchFor("/home/u/Developer/foo") != "trunk" || cfg.BaseBranchFor("/home/u/bar") != "dev" || len(cfg.RepoConfigs) != 2 {
		t.Errorf("set base: %+v", cfg.RepoConfigs)
	}
}

func TestValidate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte(`scan_roots = ["/a"]`+"\n"+`auto_fetch_interval = "soon"`), 0o644)
	if _, _, err := Load(p, "/cwd"); err == nil {
		t.Error("expected error for bad interval")
	}
	os.WriteFile(p, []byte(`scan_depth = 1`), 0o644)
	if _, _, err := Load(p, "/cwd"); err == nil {
		t.Error("expected error for no roots")
	}
}
