package proxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/soltiHQ/control-plane/domain/enum"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// maxAgentMessageBytes is the maximum gRPC message size (both send and
// receive) used for calls to agents. Must equal solti-api's MAX_REQUEST_BYTES
// (4 MiB) so both ends of the wire agree.
//
// The value matches grpc-go's own `MaxCallRecvMsgSize` default — we set it
// explicitly rather than inheriting the default because (a) the default can
// change between grpc-go versions, and (b) `MaxCallSendMsgSize` is NOT 4 MiB
// by default (it's `MaxInt32`), so capping sends here makes runaway
// allocations impossible. This is belt-and-suspenders but cheap.
//
// Sizing: a `Script`-mode task body is capped at 2 MiB in the model;
// base64 inflation × 4/3 + envelope overhead fits under 4 MiB with ~33% headroom.
const maxAgentMessageBytes = 4 * 1024 * 1024

// Pool manages shared outbound connections to agents.
//
// For HTTP it holds a single *http.Client whose Transport pools TCP connections.
// For gRPC it caches one *grpc.ClientConn per endpoint address.
type Pool struct {
	mu sync.RWMutex

	httpCli   *http.Client
	grpcConns map[string]*grpc.ClientConn
	grpcCreds credentials.TransportCredentials
}

// NewPool creates a Pool with a configured HTTP transport.
//
// clientTLS configures outbound TLS to agents (CP-as-client). When nil the pool
// stays plaintext: HTTP uses a minimal TLS-1.2 config (system roots, only
// engaged for https:// endpoints) and gRPC dials insecurely — preserving the
// pre-TLS behavior.
func NewPool(clientTLS *tls.Config) *Pool {
	httpTLS := clientTLS
	if httpTLS == nil {
		httpTLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	creds := credentials.TransportCredentials(insecure.NewCredentials())
	if clientTLS != nil {
		creds = credentials.NewTLS(clientTLS)
	}

	return &Pool{
		httpCli: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSClientConfig:     httpTLS,
				IdleConnTimeout:     90 * time.Second,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
			},
		},
		grpcConns: make(map[string]*grpc.ClientConn),
		grpcCreds: creds,
	}
}

// Get returns an AgentProxy for the given endpoint, selecting the implementation
// based on api version and endpoint type.
// Get returns an AgentProxy bound to the given endpoint and (optional) bearer
// token. An empty token means "send no credential" (auth disabled, or the agent
// is not yet enrolled). The proxy presents the token on every call.
func (p *Pool) Get(endpoint string, epType enum.EndpointType, apiVersion enum.APIVersion, token string) (AgentProxy, error) {
	switch apiVersion {
	case enum.APIVersionV1:
		return p.getV1(endpoint, epType, token)
	default:
		return nil, ErrUnsupportedAPIVersion
	}
}

func (p *Pool) getV1(endpoint string, epType enum.EndpointType, token string) (AgentProxy, error) {
	switch epType {
	case enum.EndpointHTTP:
		// Wrap the shared client so every request carries the bearer header;
		// keep the bare shared client when there is no token (zero overhead).
		var client httpClient = p.httpCli
		if token != "" {
			client = &bearerClient{inner: p.httpCli, token: token}
		}
		return &httpProxyV1{
			endpoint: strings.TrimRight(endpoint, "/"),
			client:   client,
		}, nil
	case enum.EndpointGRPC:
		conn, err := p.grpcConn(endpoint)
		if err != nil {
			return nil, err
		}
		return &grpcProxyV1{conn: conn, token: token}, nil
	default:
		return nil, ErrUnsupportedEndpointType
	}
}

// grpcConn returns a cached *grpc.ClientConn or creates one.
func (p *Pool) grpcConn(endpoint string) (*grpc.ClientConn, error) {
	// Normalise the agent-advertised endpoint: grpc-go's `NewClient`
	// wants a bare authority (`host:port`) and chokes on scheme-prefixed
	// URLs with "too many colons in address". Agents that accidentally
	// advertise `http://host:port` (legit for reqwest, wrong for gRPC)
	// still work after this normalisation.
	target := normalizeGrpcTarget(endpoint)

	p.mu.RLock()
	conn, ok := p.grpcConns[target]
	p.mu.RUnlock()
	if ok {
		return conn, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok = p.grpcConns[target]; ok {
		return conn, nil
	}
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(p.grpcCreds),
		// Align with solti-api's MAX_REQUEST_BYTES so a 5 MiB script body
		// (base64 → ~7 MiB on the wire) goes through cleanly. Without this
		// the agent's SubmitTaskResponse would still fit, but any response
		// approaching 4 MiB (ListTasks / ListTaskRuns on a busy agent)
		// would fail with `ResourceExhausted` on decode.
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxAgentMessageBytes),
			grpc.MaxCallSendMsgSize(maxAgentMessageBytes),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrDial, endpoint, err)
	}
	p.grpcConns[target] = conn
	return conn, nil
}

// normalizeGrpcTarget strips HTTP-style scheme prefixes and trailing
// slashes from an agent-advertised endpoint, turning it into the bare
// `host:port` target that `grpc.NewClient` expects.
//
// Extra tolerance for mixed-transport agent configs: an agent may
// advertise `http://host:port` (copy-pasted from an HTTP example), and
// we still want the grpc client to be able to call back via gRPC.
func normalizeGrpcTarget(endpoint string) string {
	e := strings.TrimPrefix(endpoint, "https://")
	e = strings.TrimPrefix(e, "http://")
	e = strings.TrimRight(e, "/")
	return e
}

// Close releases all pooled connections.
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	for ep, conn := range p.grpcConns {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("%w %s: %v", ErrClose, ep, err))
		}
	}
	p.grpcConns = nil
	p.httpCli.CloseIdleConnections()

	return errors.Join(errs...)
}
