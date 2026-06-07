package timeformat

import (
	"fmt"
	"time"
)

// Relative formats a timestamp as a human-readable relative time string.
// (e.g. "—", "just now", "5m ago", "2h ago", "3d ago").
func Relative(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// Session formats a session timestamp.
func Session(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	now := time.Now()
	if t.Year() == now.Year() {
		return t.Format("Jan 02, 15:04")
	}
	return t.Format("Jan 02 2006, 15:04")
}

// Uptime formats a duration in seconds as a compact uptime string.
// (e.g. "—", "30s", "5m", "2h 15m", "3d 4h").
func Uptime(s int64) string {
	switch {
	case s < 0:
		return "—"
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh %dm", s/3600, (s%3600)/60)
	default:
		return fmt.Sprintf("%dd %dh", s/86400, (s%86400)/3600)
	}
}
