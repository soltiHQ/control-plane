package base

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/soltiHQ/control-plane/internal/cluster"
)

// TickFunc runs a single reconciliation pass.
//
// ctx is the leader context:
// it is canceled the moment this replica loses leadership or the server shuts down.
type TickFunc func(ctx context.Context)

// Runner is a leader-gated periodic server.Runner.
//
// While leader, it invokes tick every interval;
// on followers it parks inside the leadership gate without ticking.
type Runner struct {
	name       string
	interval   time.Duration
	leadership cluster.Leadership
	tick       TickFunc
	logger     zerolog.Logger

	stop    chan struct{} // closed by Stop to request exit
	done    chan struct{} // closed when Start's loop has fully returned
	started atomic.Bool
}

// New builds a periodic runner.
func New(name string, interval time.Duration, leadership cluster.Leadership, logger zerolog.Logger, tick TickFunc) *Runner {
	switch {
	case interval <= 0:
		panic("base: non-positive interval")
	case leadership == nil:
		panic("base: nil leadership")
	case tick == nil:
		panic("base: nil tick")
	}
	return &Runner{
		name:       name,
		interval:   interval,
		leadership: leadership,
		tick:       tick,
		logger:     logger.With().Str("runner", name).Logger(),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Name returns the runner name.
func (r *Runner) Name() string { return r.name }

// Start runs the leader-gated tick loop until Stop is called or ctx is canceled.
func (r *Runner) Start(ctx context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	defer close(r.done)

	r.logger.Info().Dur("tick", r.interval).Msg("runner started")

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
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		r.logger.Info().Msg("runner acquired leadership")
		for {
			select {
			case <-ticker.C:
				r.tick(leaderCtx)
			case <-leaderCtx.Done():
				return nil
			}
		}
	})
}

// Stop requests shutdown and blocks until the tick loop has fully drained or ctx expires, whichever comes first.
func (r *Runner) Stop(ctx context.Context) error {
	if !r.started.Load() {
		return nil
	}
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
