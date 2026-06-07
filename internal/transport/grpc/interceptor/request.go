package interceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// UnaryRequestID returns a unary server interceptor that ensures every request has a unique ID.
func UnaryRequestID() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = ensureRequestID(ctx)
		ctx = transportctx.WithErrorSlot(ctx)
		return handler(ctx, req)
	}
}

func ensureRequestID(ctx context.Context) context.Context {
	rid := extractRequestID(ctx)
	if rid == "" {
		rid = transportctx.NewRequestID()
	}

	ctx = transportctx.WithRequestID(ctx, rid)
	_ = grpc.SetHeader(ctx, metadata.Pairs(transportctx.DefaultRequestIDHeader, rid))
	return ctx
}

func extractRequestID(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(transportctx.DefaultRequestIDHeader)
	if len(vals) == 0 {
		return ""
	}
	return transportctx.NormalizeRequestID(vals[0])
}
