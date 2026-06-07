package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/felixge/httpsnoop"
	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// Recovery catches panics and writes an error response if headers haven't been sent yet.
func Recovery(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var headerWritten bool

			wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
				WriteHeader: func(original httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
					return func(code int) {
						headerWritten = true
						original(code)
					}
				},
				Write: func(original httpsnoop.WriteFunc) httpsnoop.WriteFunc {
					return func(b []byte) (int, error) {
						headerWritten = true
						return original(b)
					}
				},
			})
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				stack := debug.Stack()

				evt := logger.Error().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("panic", fmt.Sprintf("%v", rec)).
					Bytes("stack", stack)

				if rid, ok := transportctx.RequestID(r.Context()); ok {
					evt = evt.Str("request_id", rid)
				}
				evt.Msg("panic recovered")

				if headerWritten {
					if hj, ok := w.(http.Hijacker); ok {
						if conn, _, err := hj.Hijack(); err == nil {
							_ = conn.Close()
						}
					}
					return
				}
				response.Unavailable(w, r, httpctx.ModeFromRequest(r))
			}()
			next.ServeHTTP(wrapped, r)
		})
	}
}
