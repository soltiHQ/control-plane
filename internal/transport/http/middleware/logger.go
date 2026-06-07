package middleware

import (
	"net/http"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/internal/auth/httpcookie"

	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// Logger returns middleware that logs every completed request.
func Logger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			var (
				m   = httpsnoop.CaptureMetrics(next, w, r)
				evt = logger.Info()
			)
			if m.Code >= 500 {
				evt = logger.Error()
			} else if m.Code >= 400 {
				evt = logger.Warn()
			}

			evt.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", m.Code).
				Int64("bytes", m.Written).
				Str("duration", time.Since(start).String())

			if rid, ok := transportctx.RequestID(r.Context()); ok {
				evt = evt.Str("request_id", rid)
			}
			if sid, err := httpcookie.GetSessionID(r); err == nil {
				evt = evt.Str("session_id", sid.Value)
			}
			if reason := transportctx.TryError(r.Context()); reason != "" {
				evt = evt.Str("error", reason)
			}

			evt.Msg("http request")
		})
	}
}
