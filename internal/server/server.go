// Package server provides unified lifecycle management for runtime components.
package server

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

type runnerExit struct {
	name string
	err  error
}

// Server orchestrates the lifecycle of multiple runners.
type Server struct {
	logger zerolog.Logger
	cfg    Config

	runners      []Runner
	shutdownOnce atomic.Bool
}

// New creates a Server.
func New(cfg Config, logger zerolog.Logger, runners ...Runner) (*Server, error) {
	if len(runners) == 0 {
		return nil, ErrNoRunners
	}
	cfg = cfg.withDefaults()

	var (
		seen = make(map[string]struct{}, len(runners))
		rs   = make([]Runner, 0, len(runners))
	)
	for _, r := range runners {
		if r == nil {
			return nil, ErrNilRunner
		}
		name := strings.TrimSpace(r.Name())
		if name == "" {
			return nil, fmt.Errorf("server: runner has empty name: %w", ErrEmptyRunnerName)
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateRunnerName, name)
		}
		seen[name] = struct{}{}
		rs = append(rs, r)
	}
	return &Server{
		runners: rs,
		cfg:     cfg,
		logger:  logger,
	}, nil
}

// Run starts all runners.
func (s *Server) Run(ctx context.Context) error {
	var (
		runCtx, runCancel = context.WithCancel(ctx)

		exitCh = make(chan runnerExit, 1)
		wg     sync.WaitGroup
		e      error
	)
	defer runCancel()

	wg.Add(len(s.runners))
	for _, r := range s.runners {
		r := r
		go func() {
			defer wg.Done()

			s.logger.Info().
				Str("runner", r.Name()).
				Msg("runner starting")

			err := r.Start(runCtx)
			select {
			case exitCh <- runnerExit{name: r.Name(), err: err}:
			default:
			}

			if err != nil && !errors.Is(err, context.Canceled) {
				s.logger.Error().
					Err(err).
					Str("runner", r.Name()).
					Msg("runner exited")
				return
			}
			s.logger.Info().
				Str("runner", r.Name()).
				Msg("runner exited")
		}()
	}

	select {
	case <-ctx.Done():
		e = ctx.Err()
	case ex := <-exitCh:
		if ex.err == nil {
			e = &RunnerExitedError{Runner: ex.name}
		} else if errors.Is(ex.err, context.Canceled) {
			e = ex.err
		} else {
			e = &RunnerError{Runner: ex.name, Phase: "start", Err: ex.err}
		}
	}
	runCancel()

	shutdownErr := s.Shutdown(context.Background())
	wg.Wait()

	if shutdownErr != nil {
		return errors.Join(e, shutdownErr)
	}
	return e
}

// Shutdown gracefully stops all runners.
func (s *Server) Shutdown(ctx context.Context) error {
	if !s.shutdownOnce.CompareAndSwap(false, true) {
		return nil
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()

	s.logger.Info().
		Str("timeout", s.cfg.ShutdownTimeout.String()).
		Int("runners", len(s.runners)).
		Msg("shutdown starting")

	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, r := range s.runners {
		wg.Add(1)
		go func() {
			defer wg.Done()

			s.logger.Info().Str("runner", r.Name()).Msg("runner stopping")

			if err := r.Stop(ctx); err != nil {
				wrapped := &RunnerError{Runner: r.Name(), Phase: "stop", Err: err}

				s.logger.Error().
					Err(err).
					Str("runner", r.Name()).
					Msg("runner stop failed")

				mu.Lock()
				errs = append(errs, wrapped)
				mu.Unlock()
			} else {
				s.logger.Info().Str("runner", r.Name()).Msg("runner stopped")
			}
		}()
	}
	wg.Wait()

	s.logger.Info().
		Str("elapsed", time.Since(start).String()).
		Msg("shutdown finished")
	return errors.Join(errs...)
}
