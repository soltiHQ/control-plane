package transportctx

import (
	"strings"

	"github.com/segmentio/ksuid"
)

const (
	DefaultRequestIDHeader = "x-request-id"
	maxRequestIDLen        = 128
)

// NewRequestID generates a new request id.
func NewRequestID() string {
	return ksuid.New().String()
}

// NormalizeRequestID trims a client-supplied request id and accepts it if it matches [A-Za-z0-9._-]{1,128}.
func NormalizeRequestID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxRequestIDLen {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if !isRequestIDByte(s[i]) {
			return ""
		}
	}
	return s
}

func isRequestIDByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.':
		return true
	default:
		return false
	}
}
