package sync

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/proxy"
	"github.com/soltiHQ/control-plane/internal/server/runner/base"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/errkind"
)

// proxyGetter is the small subset of *proxy.Pool that the sync runner actually needs.
type proxyGetter interface {
	Get(endpoint string, epType enum.EndpointType, apiVersion enum.APIVersion, token string) (proxy.AgentProxy, error)
}

// Runner periodically reconciles Rollout records against live state on agents.
type Runner struct {
	*base.Runner

	pool proxyGetter
	hub  *event.Hub

	logger zerolog.Logger
	store  storage.Storage
	cfg    Config
}

// New creates a sync runner.
func New(cfg Config, logger zerolog.Logger, store storage.Storage, pool *proxy.Pool, hub *event.Hub, leadership cluster.Leadership) (*Runner, error) {
	if store == nil {
		return nil, fmt.Errorf("sync: %w", storage.ErrNilStore)
	}
	if pool == nil {
		return nil, fmt.Errorf("sync: %w", proxy.ErrNilPool)
	}
	if hub == nil {
		return nil, fmt.Errorf("sync: %w", event.ErrNilHub)
	}
	if leadership == nil {
		return nil, fmt.Errorf("sync: nil leadership")
	}

	cfg = cfg.withDefaults()
	r := &Runner{
		logger: logger.With().Str("runner", cfg.Name).Logger(),
		store:  store,
		pool:   pool,
		cfg:    cfg,
		hub:    hub,
	}
	r.Runner = base.New(cfg.Name, cfg.TickInterval, leadership, logger, r.tick)
	return r, nil
}

func (r *Runner) tick(ctx context.Context) {
	r.reconcileRollouts(ctx)
	r.finalizeDeletedSpecs(ctx)
}

// reconcileRollouts picks up every actionable rollout and dispatches it by intent.
func (r *Runner) reconcileRollouts(ctx context.Context) {
	filter := r.store.BuildRolloutFilter(storage.RolloutQueryCriteria{
		Statuses: []enum.SyncStatus{
			enum.SyncStatusPending,
			enum.SyncStatusDrift,
			enum.SyncStatusFailed,
		},
	})
	res, err := r.store.ListRollouts(ctx, filter, storage.ListOptions{Limit: storage.MaxListLimit})
	if err != nil {
		r.logger.Error().Err(err).Msg("tick: list rollouts failed")
		return
	}

	var g errgroup.Group
	g.SetLimit(r.cfg.MaxConcurrency)

	for _, ss := range res.Items {
		if ss == nil {
			continue
		}
		if ss.Status() == enum.SyncStatusFailed && ss.Attempts() >= r.cfg.MaxRetries {
			continue
		}
		rolloutID := ss.ID()
		g.Go(func() error {
			pushCtx, cancel := context.WithTimeout(ctx, r.cfg.PushTimeout)
			defer cancel()
			r.reconcile(pushCtx, rolloutID)
			return nil
		})
	}
	_ = g.Wait()
}

// reconcile loads the rollout fresh (to pick up any changes since tick started) and dispatches by intent.
func (r *Runner) reconcile(ctx context.Context, rolloutID string) {
	ss, err := r.store.GetRollout(ctx, rolloutID)
	if err != nil {
		r.logger.Warn().Err(err).Str("rid", rolloutID).Msg("reconcile: load rollout failed")
		return
	}

	switch ss.Intent() {
	case enum.RolloutIntentUninstall:
		r.reconcileUninstall(ctx, ss)
	case enum.RolloutIntentInstall, enum.RolloutIntentUpdate:
		r.reconcileSubmit(ctx, ss)
	case enum.RolloutIntentNoop:
		// Filter should have excluded these, but belt-and-braces.
		return
	}
}

func (r *Runner) reconcileUninstall(ctx context.Context, ss *model.Rollout) {
	if ss.ActualTaskID() == "" {
		if err := r.store.DeleteRollout(ctx, ss.ID()); err != nil {
			r.logger.Error().Err(err).Str("rid", ss.ID()).Msg("reconcile/uninstall: delete rollout failed")
			return
		}
		r.hub.Notify(event.RefreshSpecs)
		return
	}

	ap, ok := r.getProxy(ctx, ss)
	if !ok {
		return
	}

	if err := ap.DeleteTask(ctx, ss.ActualTaskID()); err != nil && !isNotFound(err) {
		r.markFailed(ctx, ss, "delete task: "+err.Error())
		return
	}

	if err := r.store.DeleteRollout(ctx, ss.ID()); err != nil {
		r.logger.Error().Err(err).Str("rid", ss.ID()).Msg("reconcile/uninstall: delete rollout failed")
		return
	}
	r.logger.Info().
		Str("rid", ss.ID()).
		Str("spec_id", ss.SpecID()).
		Str("agent_id", ss.AgentID()).
		Msg("rollout uninstalled")
	r.hub.Notify(event.RefreshSpecs)
}

func (r *Runner) reconcileSubmit(ctx context.Context, ss *model.Rollout) {
	ts, ap, protoSpec, ok := r.prepareSubmit(ctx, ss)
	if !ok {
		return
	}

	wasInstalled := ss.ActualTaskID() != ""
	newID, err := ap.ApplyTask(ctx, proxy.TaskSubmission{Spec: protoSpec})
	if err != nil {
		r.markFailed(ctx, ss, "apply task: "+err.Error())
		return
	}

	r.markSynced(ctx, ss.ID(), ts.Generation(), newID)
	verb := "installed"
	if wasInstalled {
		verb = "updated"
	}
	r.logger.Info().
		Str("rid", ss.ID()).
		Str("spec_id", ss.SpecID()).
		Str("agent_id", ss.AgentID()).
		Str("task_id", newID).
		Int("generation", ts.Generation()).
		Msgf("rollout %s", verb)
}

// prepareSubmit loads spec + agent + proxy + proto conversion.
// Any failure at this stage calls markFailed and returns ok=false.
func (r *Runner) prepareSubmit(ctx context.Context, ss *model.Rollout) (*model.Spec, proxy.AgentProxy, *taskv1.CreateSpec, bool) {
	ts, err := r.store.GetSpec(ctx, ss.SpecID())
	if err != nil {
		r.markFailed(ctx, ss, "spec not found: "+err.Error())
		return nil, nil, nil, false
	}

	ap, ok := r.getProxy(ctx, ss)
	if !ok {
		return nil, nil, nil, false
	}

	protoSpec, err := proxy.SpecToProto(ts)
	if err != nil {
		r.markFailed(ctx, ss, "spec conversion: "+err.Error())
		return nil, nil, nil, false
	}

	return ts, ap, protoSpec, true
}

// getProxy resolves an AgentProxy for the rollout's target agent.
func (r *Runner) getProxy(ctx context.Context, ss *model.Rollout) (proxy.AgentProxy, bool) {
	ag, err := r.store.GetAgent(ctx, ss.AgentID())
	if err != nil {
		r.markFailed(ctx, ss, "agent not found: "+err.Error())
		return nil, false
	}
	ap, err := r.pool.Get(ag.Endpoint(), ag.EndpointType(), ag.APIVersion(), r.agentToken(ctx, ss.AgentID()))
	if err != nil {
		r.markFailed(ctx, ss, "proxy error: "+err.Error())
		return nil, false
	}
	return ap, true
}

// agentToken returns the bearer token to present to an agent, or "" if the
// agent has no credential (not enrolled, or auth disabled). Any lookup failure
// degrades to "" — the call proceeds without a token and the agent decides.
func (r *Runner) agentToken(ctx context.Context, agentID string) string {
	c, err := r.store.GetAgentCredential(ctx, agentID)
	if err != nil {
		return ""
	}
	return c.Token()
}

// finalizeDeletedSpecs removes any spec with DeletionRequested=true whose last rollout has drained.
func (r *Runner) finalizeDeletedSpecs(ctx context.Context) {
	specsRes, err := r.store.ListSpecs(ctx, nil, storage.ListOptions{Limit: storage.MaxListLimit})
	if err != nil {
		return
	}
	finalized := 0
	for _, ts := range specsRes.Items {
		if ts == nil || !ts.DeletionRequested() {
			continue
		}
		rolloutsRes, err := r.store.ListRollouts(ctx,
			r.store.BuildRolloutFilter(storage.RolloutQueryCriteria{SpecID: ts.ID()}),
			storage.ListOptions{Limit: 1},
		)
		if err != nil || len(rolloutsRes.Items) > 0 {
			continue
		}
		if err = r.store.DeleteSpec(ctx, ts.ID()); err != nil {
			r.logger.Error().Err(err).Str("spec_id", ts.ID()).Msg("finalize: delete spec failed")
			continue
		}
		r.logger.Info().Str("spec_id", ts.ID()).Msg("spec finalized (deletion complete)")
		finalized++
	}
	if finalized > 0 {
		r.hub.Notify(event.RefreshSpecs)
	}
}

func (r *Runner) markSynced(ctx context.Context, rID string, generation int, taskID string) {
	ss, err := r.store.GetRollout(ctx, rID)
	if err != nil {
		r.logger.Error().Err(err).Str("rid", rID).Msg("markSynced: get failed")
		return
	}
	ss.SetActualTaskID(taskID)
	ss.MarkSynced(generation)
	if err = r.store.UpsertRollout(ctx, ss); err != nil {
		r.logger.Error().Err(err).Str("rid", rID).Msg("markSynced: upsert failed")
		return
	}
	r.hub.Notify(event.RefreshSpecs)
}

func (r *Runner) markFailed(ctx context.Context, ss *model.Rollout, errMsg string) {
	ss.MarkFailed(errMsg)
	if err := r.store.UpsertRollout(ctx, ss); err != nil {
		r.logger.Error().Err(err).Str("rid", ss.ID()).Msg("markFailed: upsert failed")
		return
	}

	var specName string
	if ts, err := r.store.GetSpec(ctx, ss.SpecID()); err == nil {
		specName = ts.Name()
	}
	r.hub.Record(event.SyncFailed, event.Payload{
		ID: ss.SpecID(), Name: specName, Detail: ss.AgentID(), By: "sync",
	})
	r.hub.Notify(event.RefreshSpecs)
}

func isNotFound(err error) bool {
	return errkind.Classify(err) == errkind.NotFound
}
