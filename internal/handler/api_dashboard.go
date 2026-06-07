package handler

import (
	"net/http"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/service/agent"
	"github.com/soltiHQ/control-plane/internal/service/spec"
	"github.com/soltiHQ/control-plane/internal/service/user"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/uikit/htmx"
	"github.com/soltiHQ/control-plane/internal/uikit/routepath"

	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	contentHome "github.com/soltiHQ/control-plane/ui/templates/content/home"
)

// Dashboard handles GET /api/v1/dashboard.
func (a *API) Dashboard(w http.ResponseWriter, r *http.Request) {
	mode := httpctx.ModeFromRequest(r)
	if r.URL.Path != routepath.ApiDashboard {
		response.NotFound(w, r, mode)
		return
	}
	if r.Method != http.MethodGet {
		response.NotAllowed(w, r, mode)
		return
	}
	ctx := r.Context()

	agents, err := a.agentSVC.List(ctx, agent.ListQuery{Limit: storage.MaxListLimit})
	if err != nil {
		a.logger.Error().Err(err).Msg("dashboard: agent list failed")
		response.Unavailable(w, r, mode)
		return
	}

	var active, inactive, disconnected int
	for _, ag := range agents.Items {
		switch ag.Status() {
		case enum.AgentStatusActive:
			active++
		case enum.AgentStatusInactive:
			inactive++
		case enum.AgentStatusDisconnected:
			disconnected++
		}
	}

	specs, err := a.specSVC.List(ctx, spec.ListQuery{Limit: storage.MaxListLimit})
	if err != nil {
		a.logger.Error().Err(err).Msg("dashboard: spec list failed")
		response.Unavailable(w, r, mode)
		return
	}

	users, err := a.userSVC.List(ctx, user.ListQuery{Limit: storage.MaxListLimit})
	if err != nil {
		a.logger.Error().Err(err).Msg("dashboard: user list failed")
		response.Unavailable(w, r, mode)
		return
	}

	rollouts, err := a.specSVC.Rollouts(ctx, storage.RolloutQueryCriteria{})
	if err != nil {
		a.logger.Error().Err(err).Msg("dashboard: rollouts list failed")
		response.Unavailable(w, r, mode)
		return
	}

	var synced, pending, failed, drift int
	for _, r := range rollouts {
		switch r.Status() {
		case enum.SyncStatusSynced:
			synced++
		case enum.SyncStatusPending:
			pending++
		case enum.SyncStatusFailed:
			failed++
		case enum.SyncStatusDrift:
			drift++
		}
	}

	stats := contentHome.DashboardStats{
		TotalAgents:   len(agents.Items),
		TotalSpecs:    len(specs.Items),
		TotalUsers:    len(users.Items),
		TotalRollouts: len(rollouts),

		ActiveAgents:       active,
		InactiveAgents:     inactive,
		DisconnectedAgents: disconnected,
		SyncedRollouts:     synced,
		PendingRollouts:    pending,
		FailedRollouts:     failed,
		DriftRollouts:      drift,

		Events: a.hub.RecentEvents(30),
		Issues: contentHome.GroupIssues(a.hub.RecentIssues(100)),
	}
	response.OK(w, r, mode, &responder.View{
		Data:      stats,
		Component: contentHome.Dashboard(stats),
	})
}

// IssuesDelete handles DELETE /api/v1/dashboard/issues.
func (a *API) IssuesDelete(w http.ResponseWriter, r *http.Request) {
	mode := httpctx.ModeFromRequest(r)
	if r.Method != http.MethodDelete {
		response.NotAllowed(w, r, mode)
		return
	}

	kind := r.FormValue("kind")
	entity := r.FormValue("entity")
	if kind == "" || entity == "" {
		response.BadRequestMsg(w, r, mode, "kind and entity are required")
		return
	}

	n := a.hub.DeleteIssues(kind, entity)
	if n > 0 {
		name := r.FormValue("name")
		if name == "" {
			name = entity
		}
		a.hub.Record(event.IssueClosed, event.Payload{
			By:   a.actor(r),
			Name: name,
		})
	}
	htmx.Trigger(w, event.RefreshDashboard)
	a.hub.Notify(event.RefreshDashboard)
	response.NoContent(w, r)
}
