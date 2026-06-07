package route

import (
	"net/http"
	"strings"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/transport/http/middleware"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
)

// CollectionHandler handles a collection-level request with render mode already resolved.
type CollectionHandler func(http.ResponseWriter, *http.Request, httpctx.RenderMode)

// EntityHandler handles an entity-level request with render mode and entity ID already resolved.
type EntityHandler func(http.ResponseWriter, *http.Request, httpctx.RenderMode, string)

// Endpoint pairs an HTTP method with its permission and collection handler.
type Endpoint struct {
	Method string
	Perm   enum.Permission
	Fn     CollectionHandler
}

// Subroute pairs an action name, HTTP method, permission, and entity handler.
// Action "" matches the root (e.g. /api/v1/users/{id}).
type Subroute struct {
	Action string
	Method string
	Perm   enum.Permission
	Fn     EntityHandler
}

// Guard wraps an http.HandlerFunc with a permission check and serves it.
func Guard(w http.ResponseWriter, r *http.Request, perm enum.Permission, fn http.HandlerFunc) {
	middleware.RequirePermission(perm)(fn).ServeHTTP(w, r)
}

// Resource handles a collection-style request: resolves render mode,
// optional exact-path check, method dispatch with permission guard.
func Resource(w http.ResponseWriter, r *http.Request, path string, routes ...Endpoint) {
	mode := httpctx.ModeFromRequest(r)
	if path != "" && r.URL.Path != path {
		response.NotFound(w, r, mode)
		return
	}
	for _, rt := range routes {
		if rt.Method == r.Method {
			Guard(w, r, rt.Perm, func(w http.ResponseWriter, r *http.Request) {
				rt.Fn(w, r, mode)
			})
			return
		}
	}
	response.NotAllowed(w, r, mode)
}

// Router handles /{prefix}/{id}[/{action}] dispatching: parses id and optional action, then matches against the route table.
func Router(w http.ResponseWriter, r *http.Request, prefix string, routes ...Subroute) {
	mode := httpctx.ModeFromRequest(r)
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if rest == "" {
		response.NotFound(w, r, mode)
		return
	}

	id, tail, _ := strings.Cut(rest, "/")
	if id == "" {
		response.NotFound(w, r, mode)
		return
	}

	action := ""
	if tail != "" {
		var extra string
		action, extra, _ = strings.Cut(tail, "/")
		if extra != "" {
			response.NotFound(w, r, mode)
			return
		}
	}

	found := false
	for _, rt := range routes {
		if rt.Action != action {
			continue
		}
		found = true
		if rt.Method == r.Method {
			Guard(w, r, rt.Perm, func(w http.ResponseWriter, r *http.Request) {
				rt.Fn(w, r, mode, id)
			})
			return
		}
	}
	if found {
		response.NotAllowed(w, r, mode)
	} else {
		response.NotFound(w, r, mode)
	}
}
