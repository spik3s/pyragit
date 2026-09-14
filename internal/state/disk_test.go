package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiskUsage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big"), make([]byte, 300*1024), 0o644)
	n, err := DiskUsage(context.Background(), dir)
	if err != nil || n < 300*1024 || n > 2*1024*1024 {
		t.Errorf("got %d %v", n, err)
	}
	if _, err := DiskUsage(context.Background(), filepath.Join(dir, "missing")); err == nil {
		t.Error("expected error for missing path")
	}
}

func TestHumanSize(t *testing.T) {
	cases := map[int64]string{-1: "?", 100: "100B", 5 * 1024: "5K", 34 * 1024 * 1024: "34M", 1300 * 1024 * 1024: "1.3G", 12 * 1024 * 1024 * 1024: "12G"}
	for in, want := range cases {
		if got := HumanSize(in); got != want {
			t.Errorf("HumanSize(%d) = %q, want %q", in, got, want)
		}
	}
	if got := ExactSize(1300 * 1024 * 1024); got != "1.27 GB" {
		t.Errorf("ExactSize = %q", got)
	}
}
