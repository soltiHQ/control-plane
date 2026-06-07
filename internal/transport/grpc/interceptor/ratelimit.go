package interceptor

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/auth/ratelimit"
)

// UnaryRateLimit is the gRPC equivalent of [middleware.RateLimit]: per-IP failure-based throttle using the same [ratelimit.Limiter].
//
// Semantics:
//
//   - Before handler: Check(ipKey) - blocked → codes.ResourceExhausted.
//   - After handler: non-nil err → RecordFailure(ipKey). Success → Reset.
func UnaryRateLimit(limiter *ratelimit.Limiter) grpc.UnaryServerInterceptor {
	if limiter == nil {
		return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			return handler(ctx, req)
		}
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		key := ratelimit.IPKey(peerAddr(ctx))
		now := time.Now()

		if err := limiter.Check(key, now); errors.Is(err, auth.ErrRateLimited) {
			return nil, status.Error(codes.ResourceExhausted, "rate limited")
		}

		resp, err := handler(ctx, req)
		if err != nil {
			limiter.RecordFailure(key, time.Now())
		} else {
			limiter.Reset(key)
		}
		return resp, err
	}
}

func peerAddr(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p == nil || p.Addr == nil {
		return "unknown"
	}
	return p.Addr.String()
}
