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

// Users.

func PageUserInfoByID(id string) string     { return PageUserInfo + id }
func ApiUserByID(id string) string          { return ApiUser + id }
func ApiUserEnable(id string) string        { return ApiUser + id + "/enable" }
func ApiUserDisable(id string) string       { return ApiUser + id + "/disable" }
func ApiUserSessions(id string) string      { return ApiUser + id + "/sessions" }
func ApiUserPassword(id string) string      { return ApiUser + id + "/password" }
func ApiUserRevokeSession(id string) string { return ApiSession + id + "/revoke" }

// Agents.

func PageAgentInfoByID(id string) string { return PageAgentInfo + id }
func ApiAgentByID(id string) string      { return ApiAgent + id }
func ApiAgentLabels(id string) string    { return ApiAgent + id + "/labels" }
func ApiAgentTasks(id string) string     { return ApiAgent + id + "/tasks" }

func ApiAgentTaskLogs(agentID, taskID string) string {
	return ApiAgent + agentID + "/tasks/" + taskID + "/logs/stream"
}

// Specs.

func PageSpecInfoByID(id string) string   { return PageSpecInfo + id }
func PageSpecEditByID(id string) string   { return PageSpecEdit + id }
func ApiSpecByID(id string) string        { return ApiSpec + id }
func ApiSpecDeploy(id string) string      { return ApiSpec + id + "/deploy" }
func ApiSpecSync(id string) string        { return ApiSpec + id + "/sync" }
func ApiSpecForceDelete(id string) string { return ApiSpec + id + "/force" }

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
