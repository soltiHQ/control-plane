// Package lifecycle implements a server.Runner that periodically checks agent liveness
//   - Transitions agents through status stages: (active → inactive → disconnected → deleted)
//
// Thresholds are expressed as multiples of each agent's heartbeat interval.
package lifecycle

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/domain/model"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
)

// Runner is a server.Runner that periodically checks agent liveness.
type Runner struct {
	hub        *event.Hub
	leadership cluster.Leadership

	logger zerolog.Logger
	store  storage.AgentStore
	cfg    Config

	stop    chan struct{}
	started atomic.Bool
}

// New creates a lifecycle runner.
func New(cfg Config, logger zerolog.Logger, store storage.AgentStore, hub *event.Hub, leadership cluster.Leadership) (*Runner, error) {
	if store == nil {
		return nil, fmt.Errorf("lifecycle: %w", storage.ErrNilStore)
	}
	if hub == nil {
		return nil, fmt.Errorf("lifecycle: %w", event.ErrNilHub)
	}
	if leadership == nil {
		return nil, fmt.Errorf("lifecycle: nil leadership")
	}
	cfg = cfg.withDefaults()
	return &Runner{
		logger:     logger.With().Str("runner", cfg.Name).Logger(),
		cfg:        cfg,
		store:      store,
		hub:        hub,
		leadership: leadership,
		stop:       make(chan struct{}),
	}, nil
}

// Name returns the runner name.
func (r *Runner) Name() string { return r.cfg.Name }

// Start runs the lifecycle check loop while this replica is leader.
func (r *Runner) Start(ctx context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	r.logger.Debug().
		Dur("tick", r.cfg.TickInterval).
		Int("inactive", r.cfg.InactiveMultiplier).
		Int("disconnect", r.cfg.DisconnectMultiplier).
		Int("delete", r.cfg.DeleteMultiplier).
		Int("max_concurrency", r.cfg.MaxConcurrency).
		Msg("lifecycle runner started")

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-r.stop:
			cancel()
		case <-runCtx.Done():
		}
	}()

	return r.leadership.WhenLeader(runCtx, func(leaderCtx context.Context) error {
		ticker := time.NewTicker(r.cfg.TickInterval)
		defer ticker.Stop()
		r.logger.Info().Msg("lifecycle runner acquired leadership")
		for {
			select {
			case <-ticker.C:
				r.tick()
			case <-leaderCtx.Done():
				return nil
			}
		}
	})
}

// Stop signals the runner to exit. Safe to call multiple times.
func (r *Runner) Stop(_ context.Context) error {
	if !r.started.Load() {
		return nil
	}
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	return nil
}

func (r *Runner) tick() {
	var (
		now    = time.Now()
		ctx    = context.Background()
		filter = inmemory.NewAgentFilter().StaleAtBefore(now)

		res, err = r.store.ListAgents(ctx, filter, storage.ListOptions{
			Limit: storage.MaxListLimit,
		})
	)
	if err != nil {
		r.logger.Error().Err(err).Msg("tick: list agents failed")
		return
	}

	var g errgroup.Group
	g.SetLimit(r.cfg.MaxConcurrency)

	for _, a := range res.Items {
		if a == nil {
			continue
		}

		g.Go(func() error {
			r.reconcile(ctx, now, a)
			return nil
		})
	}
	if err = g.Wait(); err != nil {
		r.logger.Error().Err(err).Msg("tick: reconcile failed")
	}
}

func (r *Runner) reconcile(ctx context.Context, now time.Time, a *model.Agent) {
	hb := a.HeartbeatInterval()
	if hb <= 0 {
		hb = r.cfg.DefaultHeartbeat
	}

	silence := now.Sub(a.LastSeenAt())
	switch {
	case silence > hb*time.Duration(r.cfg.DeleteMultiplier):
		if err := r.store.DeleteAgent(ctx, a.ID()); err != nil {
			r.logger.Warn().Err(err).Str("agent_id", a.ID()).Msg("reconcile: delete failed")
			return
		}
		r.logger.Info().
			Str("agent_id", a.ID()).
			Dur("silence", silence).
			Msg("agent deleted (stale)")

		r.hub.Record(event.AgentDeleted, event.Payload{ID: a.ID(), Name: a.Name(), By: "lifecycle"})
		r.hub.Notify(event.RefreshAgents)

	case silence > hb*time.Duration(r.cfg.DisconnectMultiplier):
		if a.Status() != enum.AgentStatusDisconnected {
			a.MarkStatus(enum.AgentStatusDisconnected)

			if err := r.store.UpsertAgent(ctx, a); err != nil {
				r.logger.Warn().Err(err).Str("agent_id", a.ID()).Msg("reconcile: upsert disconnected failed")
				return
			}
			r.logger.Info().
				Str("agent_id", a.ID()).
				Msg("agent → disconnected")

			r.hub.Record(event.AgentDisconnected, event.Payload{ID: a.ID(), Name: a.Name(), By: "lifecycle"})
			r.hub.Notify(event.RefreshAgents)
		}

	case silence > hb*time.Duration(r.cfg.InactiveMultiplier):
		if a.Status() != enum.AgentStatusInactive {
			a.MarkStatus(enum.AgentStatusInactive)

			if err := r.store.UpsertAgent(ctx, a); err != nil {
				r.logger.Warn().Err(err).Str("agent_id", a.ID()).Msg("reconcile: upsert inactive failed")
				return
			}
			r.logger.Info().
				Str("agent_id", a.ID()).
				Msg("agent → inactive")

			r.hub.Record(event.AgentInactive, event.Payload{ID: a.ID(), Name: a.Name(), By: "lifecycle"})
			r.hub.Notify(event.RefreshAgents)
		}
	}
}
