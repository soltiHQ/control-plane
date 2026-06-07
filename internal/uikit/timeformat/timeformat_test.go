package timeformat

import (
	"testing"
	"time"
)

func TestRelative(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{"zero value", time.Time{}, "—"},
		{"future treated as just now", now.Add(1 * time.Hour), "just now"},
		{"seconds", now.Add(-30 * time.Second), "just now"},
		{"one minute", now.Add(-1 * time.Minute), "1m ago"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"one hour", now.Add(-1 * time.Hour), "1h ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"one day", now.Add(-24 * time.Hour), "1d ago"},
		{"days", now.Add(-72 * time.Hour), "3d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Relative(tt.in); got != tt.want {
				t.Fatalf("Relative(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestUptime(t *testing.T) {
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{"negative is invalid", -1, "—"},
		{"zero seconds", 0, "0s"},
		{"seconds", 30, "30s"},
		{"minutes", 300, "5m"},
		{"hours and minutes", 2*3600 + 15*60, "2h 15m"},
		{"days and hours", 3*86400 + 4*3600, "3d 4h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Uptime(tt.in); got != tt.want {
				t.Fatalf("Uptime(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSessionZeroValue(t *testing.T) {
	if got := Session(time.Time{}); got != "—" {
		t.Fatalf("Session(zero) = %q, want %q", got, "—")
	}
}
