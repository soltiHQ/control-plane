package httpctx

import (
	"context"

	"github.com/soltiHQ/control-plane/internal/transport/http/responder"
)

type responderKey struct{}

// fallback is an immutable sentinel returned when no responder was negotiated.
var fallback responder.Responder = responder.NewJSON()

// WithResponder stores the negotiated responder in ctx.
func WithResponder(ctx context.Context, r responder.Responder) context.Context {
	return context.WithValue(ctx, responderKey{}, r)
}

// Responder returns the negotiated responder from ctx.
func Responder(ctx context.Context) responder.Responder {
	if r, ok := ctx.Value(responderKey{}).(responder.Responder); ok && r != nil {
		return r
	}
	return fallback
}
