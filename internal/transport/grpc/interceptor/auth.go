package interceptor

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"github.com/soltiHQ/control-plane/domain/enum"
	"github.com/soltiHQ/control-plane/internal/auth/token"
	"github.com/soltiHQ/control-plane/internal/transport/grpc/status"
	"github.com/soltiHQ/control-plane/internal/transportctx"
)

// UnaryAuth returns a unary server interceptor that verifies the access token from metadata and stores the identity in context.
func UnaryAuth(verifier token.Verifier, skipMethods map[string]struct{}) grpc.UnaryServerInterceptor {
	if skipMethods == nil {
		skipMethods = make(map[string]struct{})
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, skip := skipMethods[info.FullMethod]; skip {
			return handler(ctx, req)
		}

		raw := extractBearerFromMetadata(ctx)
		if raw == "" {
			return nil, status.Errorf(ctx, codes.Unauthenticated, "missing token")
		}

		id, err := verifier.Verify(ctx, raw)
		if err != nil {
			return nil, status.Errorf(ctx, codes.Unauthenticated, "invalid token")
		}

		ctx = transportctx.WithIdentity(ctx, id)
		return handler(ctx, req)
	}
}

// UnaryRequirePermission returns a unary server interceptor that checks identity for a specific permission.
//
// Must be chained after UnaryAuth.
func UnaryRequirePermission(perm enum.Permission) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id, ok := transportctx.Identity(ctx)
		if !ok {
			return nil, status.Errorf(ctx, codes.Unauthenticated, "missing identity")
		}

		if !id.HasPermission(perm) {
			return nil, status.Errorf(ctx, codes.PermissionDenied, "insufficient permissions")
		}

		return handler(ctx, req)
	}
}

func extractBearerFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}

	const prefix = "bearer "
	v := vals[0]
	if len(v) < len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(v[len(prefix):])
}
