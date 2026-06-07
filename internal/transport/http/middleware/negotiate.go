package middleware

import (
	"net/http"
	"strings"

	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
)

// Negotiate attaches the correct Responder + RenderMode to the request context.
//
// Policy:
//   - /api/* + NOT HTMX  -> JSON
//   - /api/* + HTMX      -> HTML (block)
//   - non-/api/*         -> HTML (page or block depending on HTMX)
func Negotiate(json *responder.JSONResponder, html *responder.HTMLResponder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mode := httpctx.ModeFromRequest(r)

			var resp responder.Responder
			if strings.HasPrefix(r.URL.Path, "/api/") && mode != httpctx.RenderBlock {
				resp = json
			} else {
				resp = html
			}

			next.ServeHTTP(w, r.WithContext(httpctx.WithResponder(r.Context(), resp)))
		})
	}
}
