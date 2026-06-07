package middleware

import (
	"errors"
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/auth/ratelimit"
)

// RateLimit wraps the handler with a per-IP failure-based rate limit.
//
// Semantics (match the login-flow Limiter used by [auth/ratelimit]):
//
//   - Before the handler: Check(ipKey) - if blocked, respond 429 and skip the handler entirely.
//   - After the handler: if status >= 400, RecordFailure(ipKey).
//     On 2xx/3xx Reset(ipKey) so legitimate clients don't accumulate counts.
func RateLimit(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
	if limiter == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := ratelimit.IPKey(r.RemoteAddr)
			now := time.Now()

			if err := limiter.Check(key, now); errors.Is(err, auth.ErrRateLimited) {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}

			m := httpsnoop.CaptureMetrics(next, w, r)
			if m.Code >= 400 {
				limiter.RecordFailure(key, time.Now())
			} else {
				limiter.Reset(key)
			}
		})
	}
}
