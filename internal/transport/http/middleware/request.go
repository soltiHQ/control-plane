package middleware

import (
	"net/http"

	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// RequestID attaches a unique request ID to the request context and installs
// the error slot. As the outermost request-scoped middleware it owns both, so
// inner layers (handlers, response helpers) and the logger share the same slot.
func RequestID() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rid := transportctx.NormalizeRequestID(r.Header.Get(transportctx.DefaultRequestIDHeader))
			if rid == "" {
				rid = transportctx.NewRequestID()
			}
			w.Header().Set(transportctx.DefaultRequestIDHeader, rid)

			ctx := transportctx.WithRequestID(r.Context(), rid)
			ctx = transportctx.WithErrorSlot(ctx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
