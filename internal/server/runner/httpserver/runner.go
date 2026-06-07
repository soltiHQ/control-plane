// Package httpserver implements a server.Runner that manages the lifecycle
// of an [http.Server]:
//   - Builds the server from a provided [http.Handler] and timeout config
//   - Binds a TCP listener on the configured address
//   - Graceful shutdown via [http.Server.Shutdown] with hard-close fallback.
package httpserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/rs/zerolog"
)

// Runner is a server.Runner implementation for http.Server.
type Runner struct {
	logger  zerolog.Logger
	handler http.Handler
	cfg     Config

	srv       *http.Server
	ln        net.Listener
	ready     chan struct{}
	cancelCtx context.CancelFunc

	started atomic.Bool
}

// New creates an HTTP server runner.
func New(cfg Config, logger zerolog.Logger, handler http.Handler) (*Runner, error) {
	if handler == nil {
		return nil, ErrNilHandler
	}

	cfg = cfg.withDefaults()
	if cfg.Addr == "" {
		return nil, ErrEmptyAddr
	}

	return &Runner{
		handler: handler,
		logger:  logger,
		cfg:     cfg,
		ready:   make(chan struct{}),
	}, nil
}

// Name returns the runner name.
func (r *Runner) Name() string { return r.cfg.Name }

// Start binds the listener and serves until Stop() shuts it down.
func (r *Runner) Start(_ context.Context) error {
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	ln, err := net.Listen("tcp", r.cfg.Addr)
	if err != nil {
		close(r.ready)
		return err
	}
	r.ln = ln

	r.srv = &http.Server{
		Addr:    r.cfg.Addr,
		Handler: r.handler,

		ReadHeaderTimeout: r.cfg.ReadHeaderTimeout,
		ReadTimeout:       r.cfg.ReadTimeout,
		WriteTimeout:      r.cfg.WriteTimeout,
		IdleTimeout:       r.cfg.IdleTimeout,
		MaxHeaderBytes:    r.cfg.MaxHeaderBytes,

		BaseContext: func(l net.Listener) context.Context {
			parent := context.Background()
			if r.cfg.BaseContext != nil {
				parent = r.cfg.BaseContext(l)
			}
			ctx, cancel := context.WithCancel(parent)
			r.cancelCtx = cancel
			return ctx
		},
		ConnContext: r.cfg.ConnContext,
	}
	close(r.ready)

	tlsEnabled := r.cfg.TLSConfig != nil
	r.logger.Info().
		Str("runner", r.cfg.Name).
		Str("addr", r.cfg.Addr).
		Bool("tls", tlsEnabled).
		Msg("http server listening")

	if tlsEnabled {
		r.srv.TLSConfig = r.cfg.TLSConfig
		// Cert/key come from TLSConfig.Certificates, so the file args are empty.
		err = r.srv.ServeTLS(ln, "", "")
	} else {
		err = r.srv.Serve(ln)
	}
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop gracefully shuts down the server within the ctx deadline.
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
		Msg("http server shutdown requested")

	if r.cancelCtx != nil {
		r.cancelCtx()
	}

	if err := srv.Shutdown(ctx); err != nil {
		_ = srv.Close()
		r.logger.Warn().
			Err(err).
			Str("runner", r.cfg.Name).
			Msg("http server hard-closed (timeout)")
		return err
	}

	r.logger.Info().
		Str("runner", r.cfg.Name).
		Msg("http server stopped")
	return nil
}
