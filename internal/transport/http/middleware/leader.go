package middleware

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/soltiHQ/control-plane/internal/cluster"
)

// defaultForwardTimeout bounds a single proxied writing to the leader when LeaderOptions.ForwardTimeout is left unset.
const defaultForwardTimeout = 30 * time.Second

// defaultWriteMethods: mutations require leader authority.
var defaultWriteMethods = map[string]struct{}{
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

// LeaderOptions tunes the Leader middleware.
type LeaderOptions struct {
	// IsWrite overrides the default write detection.
	IsWrite func(*http.Request) bool

	// ForwardPort is the TCP port on which the OTHER replicas serve this HTTP endpoint.
	//
	// When non-zero, the middleware transparently reverse-proxies write requests to the current leader's host at this port.
	// When zero, the middleware returns 503 + X-Leader and expects the client (or ingress with retry) to recover.
	//
	// Leadership.CurrentLeader() returns the Raft TCP address (host:raftPort)
	// Take only the host and substitute ForwardPort; the same middleware works for main API and discovery.
	ForwardPort int

	// ForwardTimeout caps a single proxied write to the leader (time to first response header).
	// Without it a hung leader would pin the follower's request goroutine indefinitely.
	ForwardTimeout time.Duration

	// TargetTLS reports whether the leader's target listener serves TLS. The proxy
	// scheme must follow the TARGET listener, not how the inbound request arrived —
	// otherwise a plaintext request forwarded to a TLS listener (or vice versa) fails
	// the handshake ("client sent an HTTP request to an HTTPS server").
	TargetTLS bool
}

// Leader routes write requests to the cluster leader.
//
//   - Reads: pass through.
//   - Writes on leader: pass through.
//   - Writes on the follower with ForwardPort == 0: 503 + X-Leader header.
//   - Writes on follower with ForwardPort > 0: transparent reverse-proxy to http://<leader-host>:<ForwardPort>.
//
// In a standalone deployment AmLeader is always true, so the middleware adds essentially zero overhead.
func Leader(leadership cluster.Leadership, opts LeaderOptions) func(http.Handler) http.Handler {
	isWrite := opts.IsWrite
	if isWrite == nil {
		isWrite = defaultIsWrite
	}

	timeout := opts.ForwardTimeout
	if timeout <= 0 {
		timeout = defaultForwardTimeout
	}
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		Proxy:                 http.ProxyFromEnvironment,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: timeout,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isWrite(r) || leadership.AmLeader() {
				next.ServeHTTP(w, r)
				return
			}
			leaderAddr := leadership.CurrentLeader()
			if leaderAddr == "" {
				http.Error(w, "cluster: no leader", http.StatusServiceUnavailable)
				return
			}
			if opts.ForwardPort == 0 {
				w.Header().Set("X-Leader", leaderAddr)
				http.Error(w, "cluster: not leader", http.StatusServiceUnavailable)
				return
			}

			target, err := buildTarget(leaderAddr, opts.ForwardPort, opts.TargetTLS)
			if err != nil {
				http.Error(w, "cluster: bad leader addr", http.StatusBadGateway)
				return
			}
			proxy := httputil.NewSingleHostReverseProxy(target)
			proxy.Transport = transport
			proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
				w.Header().Set("X-Leader", leaderAddr)
				http.Error(w, "cluster: leader unreachable", http.StatusBadGateway)
			}
			proxy.ServeHTTP(w, r)
		})
	}
}

func buildTarget(leaderAddr string, port int, tls bool) (*url.URL, error) {
	host := leaderAddr
	if i := strings.LastIndex(leaderAddr, ":"); i > 0 {
		host = leaderAddr[:i]
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")

	scheme := "http"
	if tls {
		scheme = "https"
	}
	u, err := url.Parse(scheme + "://" + hostWithPort(host, port))
	if err != nil {
		return nil, err
	}
	return u, nil
}

func hostWithPort(host string, port int) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + strconv.Itoa(port)
	}
	return host + ":" + strconv.Itoa(port)
}

func defaultIsWrite(r *http.Request) bool {
	_, ok := defaultWriteMethods[r.Method]
	return ok
}
