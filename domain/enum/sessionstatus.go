package enum

import "time"

// SessionStatus describes the lifecycle state of an authenticated session.
type SessionStatus uint8

const (
	SessionStatusActive SessionStatus = iota
	SessionStatusExpired
	SessionStatusRevoked
)

// Priority returns the sort weight (lower = more important).
func (s SessionStatus) Priority() int { return int(s) }

// String returns the lowercase status label.
func (s SessionStatus) String() string {
	switch s {
	case SessionStatusActive:
		return "active"
	case SessionStatusExpired:
		return "expired"
	case SessionStatusRevoked:
		return "revoked"
	default:
		return "unknown"
	}
}

// DeriveSessionStatus determines session status from its flags.
func DeriveSessionStatus(revoked bool, expiresAt time.Time) SessionStatus {
	if revoked {
		return SessionStatusRevoked
	}
	if !expiresAt.IsZero() && expiresAt.Before(time.Now()) {
		return SessionStatusExpired
	}
	return SessionStatusActive
}
