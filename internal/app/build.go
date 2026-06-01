package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	discoverv1 "github.com/soltiHQ/control-plane/api/gen/solti/discover/v1"
	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/auth/kit"
	"github.com/soltiHQ/control-plane/internal/auth/ratelimit"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/cluster/discovery"
	"github.com/soltiHQ/control-plane/internal/cluster/standalone"
	"github.com/soltiHQ/control-plane/internal/config"
	"github.com/soltiHQ/control-plane/internal/event"
	"github.com/soltiHQ/control-plane/internal/handler"
	"github.com/soltiHQ/control-plane/internal/loghub"
	"github.com/soltiHQ/control-plane/internal/proxy"
	raftpkg "github.com/soltiHQ/control-plane/internal/raft"
	"github.com/soltiHQ/control-plane/internal/service/access"
	"github.com/soltiHQ/control-plane/internal/service/agent"
	"github.com/soltiHQ/control-plane/internal/service/credential"
	"github.com/soltiHQ/control-plane/internal/service/role"
	"github.com/soltiHQ/control-plane/internal/service/session"
	"github.com/soltiHQ/control-plane/internal/service/spec"
	"github.com/soltiHQ/control-plane/internal/service/user"
	"github.com/soltiHQ/control-plane/internal/storage"
	"github.com/soltiHQ/control-plane/internal/storage/inmemory"
	"github.com/soltiHQ/control-plane/internal/transport/grpc/interceptor"
	"github.com/soltiHQ/control-plane/internal/transport/http/middleware"
	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
	"github.com/soltiHQ/control-plane/internal/transport/http/route"
	"github.com/soltiHQ/control-plane/internal/uikit/routepath"
)

// services groups all domain services constructed at startup.
type services struct {
	credential *credential.Service
	session    *session.Service
	access     *access.Service
	agent      *agent.Service
	spec       *spec.Service
	user       *user.Service
	role       *role.Service
}

func initServices(store storage.Storage, authModel *kit.Auth, logger zerolog.Logger) services {
	return services{
		access:     access.New(authModel, store, logger),
		credential: credential.New(store, logger),
		session:    session.New(store, logger),
		agent:      agent.New(store, logger),
		spec:       spec.New(store, logger),
		role:       role.New(store, logger),
		user:       user.New(store, logger),
	}
}

// buildCluster assembles the storage + leadership + event-hub trio declared
// by cfg. In raft mode the hub is also wired to replicate mutations through
// the Raft log. Returns the store/leadership/hub the rest of the app uses,
// plus a shutdown function for the Raft node (nil in standalone).
func buildCluster(cfg cluster.Config, logger zerolog.Logger) (storage.Storage, cluster.Leadership, *event.Hub, func(), error) {
	inner := inmemory.New()
	hub := event.NewHub(logger)
	switch cfg.Backend {
	case "", cluster.BackendStandalone:
		return inner, standalone.NewLeadership(), hub, nil, nil
	case cluster.BackendRaft:
		disco, err := buildDiscovery(cfg.Discovery)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		p, err := raftpkg.New(raftpkg.Config{
			NodeID:           cfg.Raft.NodeID,
			BindAddr:         cfg.Raft.BindAddr,
			AdvertiseAddr:    cfg.Raft.AdvertiseAddr,
			DataDir:          cfg.Raft.DataDir,
			ElectionTimeout:  msToDuration(cfg.Raft.ElectionTimeoutMs),
			HeartbeatTimeout: msToDuration(cfg.Raft.HeartbeatTimeoutMs),
		}, inner, hub, disco, logger)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return p.Store(), p.Leadership(), hub, func() { _ = p.Shutdown() }, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("cluster: unknown backend %q", cfg.Backend)
	}
}

func buildDiscovery(cfg cluster.DiscoveryConfig) (cluster.Discovery, error) {
	switch cfg.Driver {
	case "", "static":
		peers := make([]cluster.Peer, 0, len(cfg.Peers))
		for _, addr := range cfg.Peers {
			peers = append(peers, cluster.Peer{ID: addr, Address: addr})
		}
		return discovery.NewStatic(peers), nil
	case "dns":
		return discovery.NewDNS(cfg.Hostname, cfg.Port)
	default:
		return nil, fmt.Errorf("discovery: unknown driver %q", cfg.Driver)
	}
}

func msToDuration(ms int) time.Duration {
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// waitForLeader polls until any leader is elected or timeout elapses.
// Returns true if a leader exists at return (which might be us or a peer).
// Used on startup so subsequent leader-only work (bootstrap, singleton
// runners) runs against a stable cluster, not a half-formed one.
func waitForLeader(ctx context.Context, l cluster.Leadership, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if l.AmLeader() || l.CurrentLeader() != "" {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return l.AmLeader() || l.CurrentLeader() != ""
}

func buildMainHandler(cfg config.Config, logger zerolog.Logger, svc services, authModel *kit.Auth, proxyPool *proxy.Pool, eventHub *event.Hub, leadership cluster.Leadership) http.Handler {
	var (
		logHub        = buildLogHub(logger, svc.agent, proxyPool, cfg.Streams)
		streamLimit   = middleware.NewStreamLimiter(cfg.Streams.MaxPerIP)
		apiHandler    = handler.NewAPI(logger, svc.user, svc.access, svc.session, svc.credential, svc.agent, svc.spec, proxyPool, eventHub, logHub, streamLimit)
		authMW        = middleware.Auth(authModel.Verifier, authModel.Session)
		uiHandler     = handler.NewUI(logger, svc.access, svc.spec, eventHub)
		staticHandler = handler.NewStatic(logger)
		logMW         = middleware.Logger(logger)
		ridMW         = middleware.RequestID()
		mux           = http.NewServeMux()

		permMW = route.PermMW(func(p enum.Permission) route.BaseMW {
			return middleware.RequirePermission(p)
		})
	)
	apiHandler.Routes(mux, authMW, permMW, ridMW, logMW)
	uiHandler.Routes(mux, authMW, permMW)
	staticHandler.Routes(mux)

	route.HandleFunc(mux, routepath.ApiEventStream, eventHub.SSEHandler(), ridMW, logMW, authMW)

	var h http.Handler = mux
	h = middleware.Negotiate(responder.NewJSON(), responder.NewHTML())(h)
	h = middleware.Leader(leadership, middleware.LeaderOptions{
		ForwardPort: addrPort(cfg.HTTP.Addr),
	})(h)
	h = middleware.RateLimit(authModel.Limiter)(h)
	h = middleware.CORS(cfg.CORS)(h)
	h = middleware.Recovery(logger)(h)
	return h
}

func buildDiscoveryHandler(logger zerolog.Logger, agentSVC *agent.Service, eventHub *event.Hub, leadership cluster.Leadership, httpPort int, limiter *ratelimit.Limiter) http.Handler {
	var (
		httpDiscovery = handler.NewHTTPDiscovery(logger, agentSVC, eventHub)
		mux           = http.NewServeMux()
	)
	mux.HandleFunc("/api/v1/discovery/sync", httpDiscovery.Sync)

	var h http.Handler = mux
	h = middleware.Leader(leadership, middleware.LeaderOptions{ForwardPort: httpPort})(h)
	h = middleware.RateLimit(limiter)(h)
	h = middleware.Recovery(logger)(h)
	h = middleware.Logger(logger)(h)
	h = middleware.RequestID()(h)
	return h
}

// buildLogHub wires the (agent, task) log fanout hub. The OpenFunc resolves
// the agent, gets its proxy, and opens the underlying StreamTaskLogs — all
// inside a closure so the hub itself stays decoupled from agent/proxy.
func buildLogHub(logger zerolog.Logger, agentSVC *agent.Service, proxyPool *proxy.Pool, cfg middleware.StreamsConfig) *loghub.Hub {
	open := func(ctx context.Context, agentID, taskID string) (<-chan *taskv1.StreamTaskLogsResponse, error) {
		ag, err := agentSVC.Get(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if ag.Endpoint() == "" {
			return nil, fmt.Errorf("agent %q has no endpoint", agentID)
		}
		p, err := proxyPool.Get(ag.Endpoint(), ag.EndpointType(), ag.APIVersion())
		if err != nil {
			return nil, err
		}
		return p.StreamTaskLogs(ctx, taskID)
	}
	return loghub.New(open, loghub.Options{SubscriberBuffer: cfg.SubscriberBuffer})
}

// addrPort extracts the port number from a listen address like ":8080" or
// "0.0.0.0:8080". Returns 0 if the address is malformed.
func addrPort(addr string) int {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return 0
	}
	n, err := strconv.Atoi(addr[i+1:])
	if err != nil {
		return 0
	}
	return n
}

func buildGRPCServer(logger zerolog.Logger, agentSVC *agent.Service, eventHub *event.Hub, leadership cluster.Leadership, limiter *ratelimit.Limiter) *grpc.Server {
	// Write methods (must run on leader). Sync is the only agent-facing
	// mutation; everything else is pure read.
	writeMethods := map[string]struct{}{
		discoverv1.DiscoverService_Sync_FullMethodName: {},
	}
	isWrite := func(full string) bool { _, ok := writeMethods[full]; return ok }

	var (
		srv = grpc.NewServer(
			grpc.ChainUnaryInterceptor(
				interceptor.UnaryRecovery(logger),
				interceptor.UnaryRequestID(),
				interceptor.UnaryLogger(logger),
				interceptor.UnaryRateLimit(limiter),
				interceptor.UnaryLeader(leadership, interceptor.LeaderOptions{IsWrite: isWrite}),
			),
			grpc.ChainStreamInterceptor(
				interceptor.StreamRecovery(logger),
				interceptor.StreamRequestID(),
				interceptor.StreamLogger(logger),
				interceptor.StreamRateLimit(limiter),
				interceptor.StreamLeader(leadership, interceptor.LeaderOptions{IsWrite: isWrite}),
			),
		)
		grpcDiscovery = handler.NewGRPCDiscovery(logger, agentSVC, eventHub)
	)
	discoverv1.RegisterDiscoverServiceServer(srv, grpcDiscovery)
	return srv
}
