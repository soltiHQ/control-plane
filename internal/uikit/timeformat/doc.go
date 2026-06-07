// Package timeformat provides human-readable time formatting helpers for the control-plane UI, consumed directly by templ templates.
//
// Each helper degrades gracefully on missing data (zero time, negative duration) by rendering an em dash so templates never show garbage:
//
//	Relative(t)    → "—", "just now", "5m ago", "2h ago", "3d ago"
//	Session(t)     → "—", "Jan 02, 15:04" (current year) / "Jan 02 2006, 15:04"
//	Uptime(secs)   → "—", "0s", "30s", "5m", "2h 15m", "3d 4h"
package timeformat
