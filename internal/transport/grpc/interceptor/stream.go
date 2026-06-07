package interceptor

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/soltiHQ/control-plane/internal/auth"
	"github.com/soltiHQ/control-plane/internal/auth/ratelimit"
	"github.com/soltiHQ/control-plane/internal/cluster"
	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// wrappedStream injects a modified context into a server stream so chained stream interceptors can attach values.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// StreamRecovery is the stream counterpart of UnaryRecovery.
// Catches panics inside streaming handlers and surfaces them as codes.Internal.
func StreamRecovery(logger zerolog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			stack := debug.Stack()
			evt := logger.Error().
				Str("method", info.FullMethod).
				Str("panic", fmt.Sprintf("%v", rec)).
				Bytes("stack", stack)
			if rid, ok := transportctx.RequestID(ss.Context()); ok {
				evt = evt.Str("request_id", rid)
			}
			evt.Msg("panic recovered")
			err = status.Error(codes.Internal, "internal error")
		}()
		return handler(srv, ss)
	}
}

// StreamRequestID injects a request id into the stream context, the same way UnaryRequestID does for unary RPCs.
func StreamRequestID() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ensureRequestID(ss.Context())
		ctx = transportctx.WithErrorSlot(ctx)
		return handler(srv, &wrappedStream{ServerStream: ss, ctx: ctx})
	}
}

// StreamLogger logs the start and end of every streaming RPC.
func StreamLogger(logger zerolog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		err := handler(srv, ss)

		evt := logger.Info().
			Str("method", info.FullMethod).
			Dur("duration", time.Since(start))
		if rid, ok := transportctx.RequestID(ss.Context()); ok {
			evt = evt.Str("request_id", rid)
		}
		if reason := transportctx.TryError(ss.Context()); reason != "" {
			evt = evt.Str("error", reason)
		}
		if err != nil {
			st, _ := status.FromError(err)
			evt.Str("code", st.Code().String()).Err(err).Msg("grpc stream")
		} else {
			evt.Str("code", "OK").Msg("grpc stream")
		}
		return err
	}
}

// StreamRateLimit applies per-peer failure-based throttling to streaming RPCs.
// Counts a stream as a failure only if the handler exits with an error.
func StreamRateLimit(limiter *ratelimit.Limiter) grpc.StreamServerInterceptor {
	if limiter == nil {
		return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			return handler(srv, ss)
		}
	}
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		key := ratelimit.IPKey(peerAddr(ss.Context()))
		if err := limiter.Check(key, time.Now()); errors.Is(err, auth.ErrRateLimited) {
			return status.Error(codes.ResourceExhausted, "rate limited")
		}
		err := handler(srv, ss)
		if err != nil {
			limiter.RecordFailure(key, time.Now())
		} else {
			limiter.Reset(key)
		}
		return err
	}
}

// StreamLeader rejects writes on followers: for streaming this means RPCs the IsWrite predicate marks as mutating.
func StreamLeader(leadership cluster.Leadership, opts LeaderOptions) grpc.StreamServerInterceptor {
	isWrite := opts.IsWrite
	if isWrite == nil {
		isWrite = neverWrite
	}
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !isWrite(info.FullMethod) || leadership.AmLeader() {
			return handler(srv, ss)
		}
		if addr := leadership.CurrentLeader(); addr != "" {
			_ = ss.SetHeader(metadata.Pairs("x-leader", addr))
		}
		return status.Error(codes.Unavailable, "not the leader")
	}
}

func neverWrite(string) bool { return false }
