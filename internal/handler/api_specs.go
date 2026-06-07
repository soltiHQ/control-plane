package handler

import (
	"errors"
	"net/http"

	"github.com/segmentio/ksuid"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/service/spec"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/transport/http/route"
	"github.com/soltiHQ/control-plane/internal/transport/httpctx"
	"github.com/soltiHQ/control-plane/internal/uikit/htmx"
	"github.com/soltiHQ/control-plane/internal/uikit/policy"
	"github.com/soltiHQ/control-plane/internal/uikit/routepath"

	restv1 "github.com/soltiHQ/control-plane/api/rest/v1"
	wire "github.com/soltiHQ/control-plane/domain/wire"
	contentSpec "github.com/soltiHQ/control-plane/ui/templates/content/spec"
)

// Specs handles /api/v1/specs.
//
// Supported:
//   - GET  /api/v1/specs
//   - POST /api/v1/specs
func (a *API) Specs(w http.ResponseWriter, r *http.Request) {
	route.Resource(w, r, routepath.ApiSpecs,
		route.Endpoint{Method: http.MethodGet, Perm: enum.SpecsGet, Fn: a.specList},
		route.Endpoint{Method: http.MethodPost, Perm: enum.SpecsAdd, Fn: func(w http.ResponseWriter, r *http.Request, m httpctx.RenderMode) {
			a.specUpsert(w, r, m, "", modeCreate)
		}},
	)
}

// SpecsRouter handles /api/v1/specs/{id} and subroutes.
//
// Supported:
//   - GET    /api/v1/specs/{id}
//   - PUT    /api/v1/specs/{id}
//   - DELETE /api/v1/specs/{id}
//   - DELETE /api/v1/specs/{id}/force
//   - POST   /api/v1/specs/{id}/deploy
//   - GET    /api/v1/specs/{id}/sync
func (a *API) SpecsRouter(w http.ResponseWriter, r *http.Request) {
	route.Router(w, r, routepath.ApiSpec,
		route.Subroute{Action: "", Method: http.MethodGet, Perm: enum.SpecsGet, Fn: a.specDetails},
		route.Subroute{Action: "", Method: http.MethodPut, Perm: enum.SpecsEdit, Fn: func(w http.ResponseWriter, r *http.Request, m httpctx.RenderMode, id string) {
			a.specUpsert(w, r, m, id, modeUpdate)
		}},
		route.Subroute{Action: "", Method: http.MethodDelete, Perm: enum.SpecsEdit, Fn: a.specDelete},
		route.Subroute{Action: "force", Method: http.MethodDelete, Perm: enum.SpecsEdit, Fn: a.specForceDelete},
		route.Subroute{Action: "deploy", Method: http.MethodPost, Perm: enum.SpecsDeploy, Fn: a.specDeploy},
		route.Subroute{Action: "sync", Method: http.MethodGet, Perm: enum.SpecsGet, Fn: a.specRollouts},
	)
}

func (a *API) specList(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode) {
	limit := queryInt(r, "limit", 0)
	cursor := r.URL.Query().Get("cursor")
	q := r.URL.Query().Get("q")

	res, err := a.specSVC.List(r.Context(), spec.ListQuery{
		Limit:    limit,
		Cursor:   cursor,
		Criteria: storage.SpecQueryCriteria{Query: q},
	})
	if err != nil {
		a.logger.Error().Err(err).Msg("spec list failed")
		response.Unavailable(w, r, mode)
		return
	}

	items := mapSlice(res.Items, wire.SpecToREST)
	response.OK(w, r, mode, &responder.View{
		Data: restv1.ListSpecsResponse{
			Items:      items,
			NextCursor: res.NextCursor,
		},
		Component: contentSpec.List(res.Items, res.NextCursor, q),
	})
}

func (a *API) specDetails(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string) {
	ts, err := a.specSVC.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			response.NotFound(w, r, mode)
			return
		}
		a.logger.Error().Err(err).Str("spec", id).Msg("spec get failed")
		response.Unavailable(w, r, mode)
		return
	}

	states, err := a.specSVC.RolloutsBySpec(r.Context(), id)
	if err != nil {
		a.logger.Error().Err(err).Str("spec", id).Msg("spec rollouts failed")
		response.Unavailable(w, r, mode)
		return
	}

	dto := wire.RolloutSpecToREST(ts, states)
	response.OK(w, r, mode, &responder.View{
		Data:      dto,
		Component: contentSpec.Detail(dto, policy.BuildSpecDetail(a.identity(r))),
	})
}

func (a *API) specUpsert(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string, action upsertMode) {
	in, err := decodeJSON[restv1.SpecCreateRequest](r)
	if err != nil {
		response.BadRequest(w, r, mode)
		return
	}

	var ts *model.Spec

	switch action {
	case modeCreate:
		if in.Name == "" || in.Slot == "" {
			response.BadRequest(w, r, mode)
			return
		}
		x, err := model.NewSpec(ksuid.New().String(), in.Name, in.Slot)
		if err != nil {
			response.BadRequest(w, r, mode)
			return
		}
		ts = x
	case modeUpdate:
		x, err := a.specSVC.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				response.NotFound(w, r, mode)
				return
			}
			a.logger.Error().Err(err).Str("spec", id).Msg("spec get failed")
			response.Unavailable(w, r, mode)
			return
		}
		if in.Name != "" {
			x.SetName(in.Name)
		}
		// Slot is immutable after create — it is the task's identity/lane on
		// the agent, and ApplyTask supersedes only within a slot, so changing
		// it would orphan the old task. A different slot is a new spec, not an
		// edit. Echoing the same slot is fine; a different value is rejected.
		if in.Slot != "" && in.Slot != x.Slot() {
			response.BadRequest(w, r, mode)
			return
		}
		ts = x
	default:
		response.BadRequest(w, r, mode)
		return
	}

	// Kind
	if in.KindType != "" {
		ts.SetKindType(enum.TaskKindType(in.KindType))
	}
	if in.KindConfig != nil {
		ts.SetKindConfig(in.KindConfig)
	}

	// Lifecycle
	if in.TimeoutMs > 0 {
		ts.SetTimeoutMs(in.TimeoutMs)
	}
	if in.RestartType != "" {
		ts.SetRestartType(enum.RestartType(in.RestartType))
	}
	if in.IntervalMs > 0 {
		ts.SetIntervalMs(in.IntervalMs)
	}
	if action == modeCreate {
		ts.SetBackoff(model.BackoffConfig{
			Jitter:  enum.JitterStrategy(in.Jitter),
			FirstMs: in.BackoffFirstMs,
			MaxMs:   in.BackoffMaxMs,
			Factor:  in.BackoffFactor,
		})
	} else if in.Jitter != "" || in.BackoffFirstMs > 0 || in.BackoffMaxMs > 0 || in.BackoffFactor > 0 {
		b := ts.Backoff()
		if in.Jitter != "" {
			b.Jitter = enum.JitterStrategy(in.Jitter)
		}
		if in.BackoffFirstMs > 0 {
			b.FirstMs = in.BackoffFirstMs
		}
		if in.BackoffMaxMs > 0 {
			b.MaxMs = in.BackoffMaxMs
		}
		if in.BackoffFactor > 0 {
			b.Factor = in.BackoffFactor
		}
		ts.SetBackoff(b)
	}

	// Targets
	if action == modeCreate {
		if len(in.Targets) > 0 {
			ts.SetTargets(in.Targets)
		}
		if len(in.TargetLabels) > 0 {
			ts.SetTargetLabels(in.TargetLabels)
		}
		if len(in.RunnerLabels) > 0 {
			ts.SetRunnerLabels(in.RunnerLabels)
		}
	} else {
		if in.Targets != nil {
			ts.SetTargets(in.Targets)
		}
		if in.TargetLabels != nil {
			ts.SetTargetLabels(in.TargetLabels)
		}
		if in.RunnerLabels != nil {
			ts.SetRunnerLabels(in.RunnerLabels)
		}
	}

	if action == modeCreate {
		if err := a.specSVC.Create(r.Context(), ts); err != nil {
			a.logger.Error().Err(err).Msg("spec create failed")
			response.Unavailable(w, r, mode)
			return
		}
		a.logger.Info().Str("spec", ts.ID()).Str("name", ts.Name()).Msg("spec created")
		a.hub.Record(event.SpecCreated, event.Payload{ID: ts.ID(), Name: ts.Name()})
		a.hub.Notify(event.RefreshSpecs)
		htmx.Redirect(w, routepath.PageSpecs)
		response.NoContent(w, r)
		return
	}

	if err = a.specSVC.Upsert(r.Context(), ts, in.Version); err != nil {
		var conflict *spec.ConflictError
		if errors.As(err, &conflict) {
			a.logger.Warn().Str("spec", id).
				Int("expected", conflict.Expected).
				Int("actual", conflict.Actual).
				Msg("spec update rejected: version conflict")
			response.Conflict(w, r, mode, conflict.Error())
			return
		}
		if errors.Is(err, storage.ErrInvalidArgument) {
			response.BadRequest(w, r, mode)
			return
		}
		a.logger.Error().Err(err).Str("spec", id).Msg("spec update failed")
		response.Unavailable(w, r, mode)
		return
	}
	a.logger.Info().Str("spec", id).Msg("spec updated")
	a.hub.Record(event.SpecUpdated, event.Payload{ID: id, Name: ts.Name()})
	a.hub.Notify(event.RefreshSpecs)
	// Mirror the create path: after Save Changes send the user back to
	// the specs list. The edit form's Alpine submit handler honors the
	// HX-Redirect header. The old htmx.Trigger(SpecUpdate) no longer
	// fires from this response — redirect obviates it, and the list page
	// picks up the updated spec through its own poll/SSE stream.
	htmx.Redirect(w, routepath.PageSpecs)
	response.NoContent(w, r)
}

func (a *API) specDelete(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string) {
	err := a.specSVC.Delete(r.Context(), id)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		a.logger.Error().Err(err).Str("spec", id).Msg("spec delete failed")
		response.Unavailable(w, r, mode)
		return
	}
	a.logger.Info().Str("spec", id).Msg("spec deleted")
	a.hub.Notify(event.RefreshSpecs)
	htmx.Redirect(w, routepath.PageSpecs)
	response.NoContent(w, r)
}

// specForceDelete drops the spec and its rollouts immediately, without
// waiting for agents to uninstall their tasks. Logs a warning because
// any running task on an agent becomes orphaned: the agent keeps it
// alive until a human intervenes. UI surfaces this option only when a
// normal Delete got stuck on retry-exhausted Uninstall rollouts.
func (a *API) specForceDelete(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string) {
	if err := a.specSVC.ForceDelete(r.Context(), id); err != nil && !errors.Is(err, storage.ErrNotFound) {
		a.logger.Error().Err(err).Str("spec", id).Msg("spec force-delete failed")
		response.Unavailable(w, r, mode)
		return
	}
	a.logger.Warn().Str("spec", id).Msg("spec force-deleted; agent tasks may be orphaned")
	a.hub.Notify(event.RefreshSpecs)
	htmx.Redirect(w, routepath.PageSpecs)
	response.NoContent(w, r)
}

func (a *API) specDeploy(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string) {
	if err := a.specSVC.Deploy(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			response.NotFound(w, r, mode)
			return
		}
		var unknown *spec.UnknownTargetsError
		if errors.As(err, &unknown) {
			a.logger.Warn().Str("spec", id).Strs("missing_agents", unknown.Agents).Msg("spec deploy rejected")
			response.BadRequest(w, r, mode)
			return
		}
		a.logger.Error().Err(err).Str("spec", id).Msg("spec deploy failed")
		response.Unavailable(w, r, mode)
		return
	}

	rollouts, err := a.specSVC.RolloutsBySpec(r.Context(), id)
	if err != nil {
		a.logger.Warn().Err(err).Str("spec", id).Msg("spec deployed but rollout query failed")
	} else {
		a.logger.Info().Str("spec", id).Int("rollouts", len(rollouts)).Msg("spec deployed")
	}

	var specName string
	if ts, err := a.specSVC.Get(r.Context(), id); err == nil {
		specName = ts.Name()
	}
	a.hub.Record(event.SpecDeployed, event.Payload{ID: id, Name: specName})
	htmx.Trigger(w, event.RefreshSpecs)
	a.hub.Notify(event.RefreshSpecs)
	response.NoContent(w, r)
}

func (a *API) specRollouts(w http.ResponseWriter, r *http.Request, mode httpctx.RenderMode, id string) {
	states, err := a.specSVC.RolloutsBySpec(r.Context(), id)
	if err != nil {
		a.logger.Error().Err(err).Str("spec", id).Msg("spec rollouts failed")
		response.Unavailable(w, r, mode)
		return
	}

	a.logger.Info().Str("spec", id).Int("count", len(states)).Msg("spec rollouts loaded")

	items := mapSlice(states, wire.RolloutEntryToREST)
	response.OK(w, r, mode, &responder.View{
		Data:      items,
		Component: contentSpec.Rollouts(items),
	})
}
