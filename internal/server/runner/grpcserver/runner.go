package grpcserver

import (
	"context"
	"errors"
	"net"
	"sync/atomic"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

// Runner is a server.Runner implementation for grpc.Server.
type Runner struct {
	logger zerolog.Logger
	cfg    Config

	srv   *grpc.Server
	ln    net.Listener
	ready chan struct{}

	started atomic.Bool
}

// New creates a gRPC server runner.
//
// Srv must be a fully configured *grpc.Server (with interceptors, keepalive, etc.).
// The caller should do registration of services before passing srv here.
func New(cfg Config, logger zerolog.Logger, srv *grpc.Server) (*Runner, error) {
	cfg = cfg.withDefaults()

	if srv == nil {
		return nil, ErrNilServer
	}
	if cfg.Addr == "" {
		return nil, ErrEmptyAddr
	}

	return &Runner{
		logger: logger,
		cfg:    cfg,
		srv:    srv,
		ready:  make(chan struct{}),
	}, nil
}

// Name returns the runner name.
func (r *Runner) Name() string { return r.cfg.Name }

// Start binds the listener and serves until Stop() shuts it down.
func (r *Runner) Start(_ context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	ln, err := net.Listen(r.cfg.Network, r.cfg.Addr)
	if err != nil {
		close(r.ready)
		return err
	}
	r.ln = ln
	close(r.ready)

	r.logger.Info().
		Str("runner", r.cfg.Name).
		Str("addr", r.cfg.Addr).
		Msg("grpc server listening")

	err = r.srv.Serve(ln)
	if err == nil || errors.Is(err, grpc.ErrServerStopped) {
		return nil
	}
	return err
}

// Stop attempts graceful shutdown and falls back to hard stop when ctx expires.
func (r *Runner) Stop(ctx context.Context) error {
	if !r.started.Load() {
		return nil
	}

	select {
	case <-r.ready:
	case <-ctx.Done():
		return ctx.Err()
	}

	srv := r.srv
	if srv == nil {
		return nil
	}

	r.logger.Info().
		Str("runner", r.cfg.Name).
		Msg("grpc server shutdown requested")

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.GracefulStop()
	}()

	select {
	case <-done:
		r.logger.Info().
			Str("runner", r.cfg.Name).
			Msg("grpc server stopped")
		return nil
	case <-ctx.Done():
		srv.Stop()
		r.logger.Warn().
			Str("runner", r.cfg.Name).
			Msg("grpc server hard-stopped (timeout)")
		return ctx.Err()
	}
}
