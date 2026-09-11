package ui

import (
	"testing"
	"time"
)

func TestSmartTime(t *testing.T) {
	now := time.Date(2026, 9, 11, 15, 0, 0, 0, time.Local)
	cases := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 9, 11, 14, 32, 0, 0, time.Local), "14:32"},
		{time.Date(2026, 9, 9, 8, 5, 0, 0, time.Local), "Sep 9 08:05"},
		{time.Date(2025, 3, 1, 8, 5, 0, 0, time.Local), "2025-03-01"},
		{time.Time{}, ""},
	}
	for _, c := range cases {
		if got := smartTime(c.in, now); got != c.want {
			t.Errorf("smartTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := absTime(time.Date(2026, 9, 11, 14, 32, 5, 0, time.Local)); got != "2026-09-11 14:32:05" {
		t.Errorf("absTime = %q", got)
	}
}
