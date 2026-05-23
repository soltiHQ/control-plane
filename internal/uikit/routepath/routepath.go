// Package routepath declares all URL constants used by the control-plane UI and API.
//
// Constants are split into two groups:
//   - Page* - browser-facing paths served by the UI handler (HTML pages).
//   - Api*  - JSON/REST endpoints served by the API handler.
//
// Path-builder functions (var block) append an entity ID to a base path, keeping URL construction consistent
// and typo-free across handlers, templates, and Alpine.js fetch calls.
package routepath

import "net/url"

const (
	PageHome   = "/"
	PageLogin  = "/login"
	PageLogout = "/logout"

	PageUsers    = "/users"
	PageUserInfo = "/users/info/"

	PageAgents    = "/agents"
	PageAgentInfo = "/agents/info/"

	PageSpecs    = "/specs"
	PageSpecNew  = "/specs/new"
	PageSpecEdit = "/specs/edit/"
	PageSpecInfo = "/specs/info/"

	ApiSession = "/api/v1/session/"

	ApiUsers = "/api/v1/users"
	ApiUser  = "/api/v1/users/"

	ApiAgents = "/api/v1/agents"
	ApiAgent  = "/api/v1/agents/"

	ApiPermissions = "/api/v1/permissions"
	ApiRoles       = "/api/v1/roles"

	ApiSpecs = "/api/v1/specs"
	ApiSpec  = "/api/v1/specs/"

	ApiDashboard       = "/api/v1/dashboard"
	ApiDashboardIssues = "/api/v1/dashboard/issues"
	ApiEventStream     = "/api/v1/events/stream"
)

var (
	PageUserInfoByID     = func(id string) string { return PageUserInfo + id }
	ApiUserCrudOp        = func(id string) string { return ApiUser + id }
	ApiUserEnable        = func(id string) string { return ApiUser + id + "/enable" }
	ApiUserDisable       = func(id string) string { return ApiUser + id + "/disable" }
	ApiUserSessions      = func(id string) string { return ApiUser + id + "/sessions" }
	ApiUserPassword      = func(id string) string { return ApiUser + id + "/password" }
	ApiUserRevokeSession = func(id string) string { return ApiSession + id + "/revoke" }

	PageAgentInfoByID = func(id string) string { return PageAgentInfo + id }
	ApiAgentByID      = func(id string) string { return ApiAgent + id }
	ApiAgentLabels    = func(id string) string { return ApiAgent + id + "/labels" }
	ApiAgentTasks     = func(id string) string { return ApiAgent + id + "/tasks" }
	ApiAgentTaskLogs  = func(agentID, taskID string) string {
		return ApiAgent + agentID + "/tasks/" + taskID + "/logs/stream"
	}

	PageSpecInfoByID   = func(id string) string { return PageSpecInfo + id }
	PageSpecEditByID   = func(id string) string { return PageSpecEdit + id }
	ApiSpecByID        = func(id string) string { return ApiSpec + id }
	ApiSpecDeploy      = func(id string) string { return ApiSpec + id + "/deploy" }
	ApiSpecSync        = func(id string) string { return ApiSpec + id + "/sync" }
	ApiSpecForceDelete = func(id string) string { return ApiSpec + id + "/force" }
)

// CursorURL appends optional cursor and query parameters to a base API path.
func CursorURL(base, cursor, q string) string {
	v := url.Values{}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	if q != "" {
		v.Set("q", q)
	}
	qs := v.Encode()
	if qs == "" {
		return base
	}
	return base + "?" + qs
}
