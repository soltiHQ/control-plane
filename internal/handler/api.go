package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/auth/identity"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/loghub"
	"github.com/soltiHQ/control-plane/internal/proxy"
	"github.com/soltiHQ/control-plane/internal/service"
	"github.com/soltiHQ/control-plane/internal/service/access"
	"github.com/soltiHQ/control-plane/internal/service/agent"
	"github.com/soltiHQ/control-plane/internal/service/credential"
	"github.com/soltiHQ/control-plane/internal/service/session"
	"github.com/soltiHQ/control-plane/internal/service/spec"
	"github.com/soltiHQ/control-plane/internal/service/user"
	"github.com/soltiHQ/control-plane/internal/transport/http/middleware"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/http/route"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/transportctx"
	"github.com/soltiHQ/control-plane/internal/uikit/routepath"

	restv1 "github.com/soltiHQ/control-plane/api/rest/v1"
	wire "github.com/soltiHQ/control-plane/domain/wire"
)

// upsertMode selects create/update branch in upsert handlers.
type upsertMode uint8

const (
	modeCreate upsertMode = iota
	modeUpdate
)

// API handlers.
type API struct {
	credentialSVC *credential.Service
	sessionSVC    *session.Service
	accessSVC     *access.Service
	agentSVC      *agent.Service
	specSVC       *spec.Service
	userSVC       *user.Service
	proxyPool     *proxy.Pool
	hub           *event.Hub
	logHub        *loghub.Hub
	streamLimit   *middleware.StreamLimiter

	logger zerolog.Logger
}

// NewAPI creates a new API handler.
func NewAPI(
	logger zerolog.Logger,
	userSVC *user.Service,
	accessSVC *access.Service,
	sessionSVC *session.Service,
	credentialSVC *credential.Service,
	agentSVC *agent.Service,
	specSVC *spec.Service,
	proxyPool *proxy.Pool,
	hub *event.Hub,
	logHub *loghub.Hub,
	streamLimit *middleware.StreamLimiter,
) *API {
	if accessSVC == nil {
		panic(service.ErrNilService)
	}
	if userSVC == nil {
		panic(service.ErrNilService)
	}
	if sessionSVC == nil {
		panic(service.ErrNilService)
	}
	if credentialSVC == nil {
		panic(service.ErrNilService)
	}
	if agentSVC == nil {
		panic(service.ErrNilService)
	}
	if specSVC == nil {
		panic(service.ErrNilService)
	}
	if proxyPool == nil {
		panic(proxy.ErrNilPool)
	}
	if hub == nil {
		panic(event.ErrNilHub)
	}
	return &API{
		logger: logger.With().Str("handler", "api").Logger(),

		credentialSVC: credentialSVC,
		sessionSVC:    sessionSVC,
		accessSVC:     accessSVC,
		agentSVC:      agentSVC,
		specSVC:       specSVC,
		userSVC:       userSVC,
		proxyPool:     proxyPool,
		hub:           hub,
		logHub:        logHub,
		streamLimit:   streamLimit,
	}
}

// Routes registers API routes.
// Auth runs at mux level once. Permissions are enforced per-method/per-subroute below.
func (a *API) Routes(mux *http.ServeMux, auth route.BaseMW, _ route.PermMW, common ...route.BaseMW) {
	route.HandleFunc(mux, routepath.ApiUsers, a.Users, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiUser, a.UsersRouter, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiSession, a.SessionsRouter, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiAgents, a.Agents, append(common, auth)...)
	route.HandleFunc(mux, "GET /api/v1/agents/{id}/tasks/{taskID}/logs/stream", a.AgentTaskLogsStream, append(common, auth, middleware.StreamLimit(a.streamLimit))...)
	route.HandleFunc(mux, routepath.ApiAgent, a.AgentsRouter, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiSpecs, a.Specs, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiSpec, a.SpecsRouter, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiDashboard, a.Dashboard, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiDashboardIssues, a.IssuesDelete, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiPermissions, a.Permissions, append(common, auth)...)
	route.HandleFunc(mux, routepath.ApiRoles, a.Roles, append(common, auth)...)
}

// Permissions handles GET /api/v1/permissions.
func (a *API) Permissions(w http.ResponseWriter, r *http.Request) {
	route.Resource(w, r, "",
		route.Endpoint{Method: http.MethodGet, Perm: enum.UsersEdit, Fn: a.permissionsList},
	)
}

// Roles handles GET /api/v1/roles.
func (a *API) Roles(w http.ResponseWriter, r *http.Request) {
	route.Resource(w, r, "",
		route.Endpoint{Method: http.MethodGet, Perm: enum.UsersEdit, Fn: a.rolesList},
	)
}

func (a *API) permissionsList(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	perms := a.accessSVC.GetPermissions()

	items := make([]string, 0, len(perms))
	for _, p := range perms {
		items = append(items, wire.PermissionToREST(p))
	}
	response.OK(w, r, mode, &responder.View{
		Data: restv1.ListPermissionsResponse{Items: items},
	})
}

func (a *API) rolesList(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	roles, err := a.accessSVC.GetRoles(r.Context())
	if err != nil {
		a.logger.Error().Err(err).Msg("roles list failed")
		response.Unavailable(w, r, mode)
		return
	}

	items := mapSlice(roles, wire.RoleToREST)
	response.OK(w, r, mode, &responder.View{
		Data: restv1.ListRolesResponse{Items: items},
	})
}

// identity returns the authenticated identity from the request context.
func (a *API) identity(r *http.Request) *identity.Identity {
	id, _ := transportctx.Identity(r.Context())
	return id
}

// actor returns the display name of the authenticated user from the request context.
func (a *API) actor(r *http.Request) string {
	if id := a.identity(r); id != nil {
		return id.Name
	}
	return "unknown"
}

// decodeJSON reads and decodes a JSON request body into v.
func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	err := json.NewDecoder(r.Body).Decode(&v)
	return v, err
}

// queryInt reads a query parameter as int, returning fallback if absent or invalid.
func queryInt(r *http.Request, key string, fallback int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n
	}
	return fallback
}

// mapSlice converts a slice of pointers using fn, skipping nils.
func mapSlice[T any, U any](src []*T, fn func(*T) U) []U {
	out := make([]U, 0, len(src))
	for _, v := range src {
		if v == nil {
			continue
		}
		out = append(out, fn(v))
	}
	return out
}
