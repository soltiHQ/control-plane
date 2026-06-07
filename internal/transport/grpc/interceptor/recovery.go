package interceptor

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/soltiHQ/control-plane/internal/transport/grpc/status"
	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// UnaryRecovery returns a unary server interceptor that catches panics and returns codes.Internal to the client.
func UnaryRecovery(logger zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
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

			if rid, ok := transportctx.RequestID(ctx); ok {
				evt = evt.Str("request_id", rid)
			}

			resp = nil
			evt.Msg("panic recovered")
			err = status.Errorf(ctx, codes.Internal, "internal error")
		}()

		return handler(ctx, req)
	}
}
