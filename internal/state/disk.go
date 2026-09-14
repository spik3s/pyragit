package state

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DiskUsage returns the on-disk size of path in bytes using `du -sk`, which
// counts allocated blocks and does not double-count hard links.
func DiskUsage(ctx context.Context, path string) (int64, error) {
	out, err := exec.CommandContext(ctx, "du", "-sk", path).Output()
	if err != nil && len(out) == 0 {
		return 0, fmt.Errorf("du %s: %w", path, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return 0, fmt.Errorf("du %s: empty output", path)
	}
	kb, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("du %s: %w", path, err)
	}
	return kb * 1024, nil
}

// HumanSize renders bytes compactly: 512K, 34M, 1.2G.
func HumanSize(b int64) string {
	switch {
	case b < 0:
		return "?"
	case b < 1024:
		return fmt.Sprintf("%dB", b)
	case b < 1024*1024:
		return fmt.Sprintf("%dK", b/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%dM", b/(1024*1024))
	case b < 10*1024*1024*1024:
		return fmt.Sprintf("%.1fG", float64(b)/(1024*1024*1024))
	}
	return fmt.Sprintf("%dG", b/(1024*1024*1024))
}

// ExactSize renders bytes with two decimals in the largest fitting unit.
func ExactSize(b int64) string {
	const k = 1024.0
	f := float64(b)
	switch {
	case b < 0:
		return "unknown"
	case f < k:
		return fmt.Sprintf("%d B", b)
	case f < k*k:
		return fmt.Sprintf("%.1f KB", f/k)
	case f < k*k*k:
		return fmt.Sprintf("%.1f MB", f/k/k)
	}
	return fmt.Sprintf("%.2f GB", f/k/k/k)
}
