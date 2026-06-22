// Package app is the composition root for the podium control-plane binary.
//
// It assembles cluster state, services, transports, and background runners
// into a single [App] aggregate with a clean Build / Run / Shutdown contract.
package app

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/soltiHQ/control-plane/internal/auth/kit"
	"github.com/soltiHQ/control-plane/internal/bootstrap"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/config"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/proxy"
	"github.com/soltiHQ/control-plane/internal/server"
	"github.com/soltiHQ/control-plane/internal/server/runner/grpcserver"
	"github.com/soltiHQ/control-plane/internal/server/runner/httpserver"
	"github.com/soltiHQ/control-plane/internal/server/runner/lifecycle"
	syncrunner "github.com/soltiHQ/control-plane/internal/server/runner/sync"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/http/response"
	"github.com/soltiHQ/control-plane/internal/uikit/htmx"
	"github.com/soltiHQ/control-plane/internal/uikit/routepath"

	pageSystem "github.com/soltiHQ/control-plane/ui/templates/page/system"
)

// leaderWaitTimeout caps how long [New] blocks waiting for any leader to be
// elected before falling back to follower-mode bootstrap behaviour.
const leaderWaitTimeout = 30 * time.Second

// App is the fully wired control-plane runtime.
type App struct {
	logger zerolog.Logger

	store        storage.Storage
	leadership   cluster.Leadership
	eventHub     *event.Hub
	raftShutdown func()
	proxyPool    *proxy.Pool
	authModel    *kit.Auth

	server *server.Server
}

// New assembles all dependencies and seeds initial data when this replica is
// the leader. Returned [App] is ready to [Run]. Caller must call [Shutdown]
// to release deferred resources (Raft node, proxy pool, event hub).
func New(ctx context.Context, cfg config.Config, logger zerolog.Logger) (*App, error) {
	store, leadership, eventHub, raftShutdown, err := buildCluster(cfg.Cluster, logger)
	if err != nil {
		return nil, err
	}

	authModel := kit.New(store, cfg.Auth)
	svc := initServices(store, authModel, logger)

	// Default users/roles/credentials seeding must run through the store.
	// In raft mode that means only the leader actually persists — followers
	// would get "not the leader" on Upsert. Solution: wait for any leader,
	// then only the leader seeds. If this replica is a follower, skip —
	// leader's writes will replicate here automatically.
	if waitForLeader(ctx, leadership, leaderWaitTimeout) && leadership.AmLeader() {
		if err = bootstrap.Run(ctx, logger, svc.role, svc.user, svc.credential); err != nil {
			closeOnError(raftShutdown, eventHub)
			return nil, err
		}
	} else {
		logger.Info().Msg("bootstrap skipped: this replica is a follower; leader seeds shared state")
	}

	// TLS: agent-facing listeners (HTTP-discovery, gRPC) get the full server config,
	// including mTLS when a client CA is set. The human-facing UI gets the SAME cert
	// but WITHOUT a client-certificate requirement, so enabling mTLS for agents does
	// not lock browsers (which have no client cert) out of the dashboard. One client
	// config for outbound calls to agents. All opt-in (nil = plaintext).
	serverTLS, err := cfg.TLS.Server.Build()
	if err != nil {
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}
	uiServerTLS, err := cfg.TLS.Server.BuildNoClientAuth()
	if err != nil {
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}
	clientTLS, err := cfg.TLS.Client.Build()
	if err != nil {
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	// Surface the actual state of BOTH directions so an operator can't silently
	// half-enable TLS (e.g. server on, but CP→agent still plaintext).
	logger.Info().
		Bool("server_tls", cfg.TLS.Server.Enabled()).
		Bool("server_mtls", cfg.TLS.Server.ClientCAFile != "").
		Bool("client_tls", cfg.TLS.Client.Enabled()).
		Bool("client_mtls", cfg.TLS.Client.CertFile != "").
		Msg("tls configuration")

	proxyPool := proxy.NewPool(clientTLS)

	htmx.Configure(cfg.Triggers)

	// UI → transport hooks: plug the templ-based error page and the login
	// redirect path into the generic response/responder layer. Without these
	// calls the binary still works — error HTML bodies are empty and 401
	// responses stay as JSON, which is the correct behaviour when the UI is
	// disabled.
	response.ErrorPageRenderer = pageSystem.ErrorPage
	responder.LoginPath = routepath.PageLogin

	lifecycleRunner, err := lifecycle.New(cfg.Lifecycle, logger, store, eventHub, leadership)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	syncRunner, err := syncrunner.New(cfg.Sync, logger, store, proxyPool, eventHub, leadership)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	mainHandler := buildMainHandler(cfg, logger, svc, authModel, proxyPool, eventHub, leadership)
	httpCfg := cfg.HTTP
	httpCfg.TLSConfig = uiServerTLS
	httpRunner, err := httpserver.New(httpCfg, logger, mainHandler)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	discoveryHandler := buildDiscoveryHandler(logger, svc.agent, eventHub, leadership, addrPort(cfg.HTTPDiscovery.Addr), authModel.Limiter, cfg.AgentAuth.Require, cfg.TLS.Server.Enabled())
	discoveryCfg := cfg.HTTPDiscovery
	discoveryCfg.TLSConfig = serverTLS
	httpDiscoveryRunner, err := httpserver.New(discoveryCfg, logger, discoveryHandler)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	grpcSrv := buildGRPCServer(logger, svc.agent, eventHub, leadership, authModel.Limiter, serverTLS, cfg.AgentAuth.Require)
	grpcRunner, err := grpcserver.New(cfg.GRPC, logger, grpcSrv)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	srv, err := server.New(cfg.Server, logger, httpRunner, httpDiscoveryRunner, grpcRunner, lifecycleRunner, syncRunner)
	if err != nil {
		proxyPool.Close()
		closeOnError(raftShutdown, eventHub)
		return nil, err
	}

	return &App{
		logger:       logger,
		store:        store,
		leadership:   leadership,
		eventHub:     eventHub,
		raftShutdown: raftShutdown,
		proxyPool:    proxyPool,
		authModel:    authModel,
		server:       srv,
	}, nil
}

// Run starts the server runners and blocks until ctx is cancelled or a runner
// exits unexpectedly. Returns the first error encountered, if any.
func (a *App) Run(ctx context.Context) error {
	return a.server.Run(ctx)
}

// Shutdown releases resources acquired by [New] in reverse order of creation.
// Bounded by ctx — if any step (Raft fsync, hub drain, pool close) hangs
// past the deadline, Shutdown returns and the process exits dirty. The
// caller logs that case; subsequent calls are no-ops.
//
// Returns an error if the deadline elapses before cleanup completes.
func (a *App) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if a.proxyPool != nil {
			a.proxyPool.Close()
			a.proxyPool = nil
		}
		if a.eventHub != nil {
			a.eventHub.Close()
			a.eventHub = nil
		}
		if a.raftShutdown != nil {
			a.raftShutdown()
			a.raftShutdown = nil
		}
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// closeOnError releases cluster-level resources when [New] aborts mid-build.
func closeOnError(raftShutdown func(), hub *event.Hub) {
	if hub != nil {
		hub.Close()
	}
	if raftShutdown != nil {
		raftShutdown()
	}
}
