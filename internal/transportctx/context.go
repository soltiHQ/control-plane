// Package transportctx provides request-scoped context values shared by the HTTP and gRPC transport layers.
package transportctx

import (
	"context"
	"sync/atomic"

	"github.com/soltiHQ/control-plane/internal/auth/identity"
)

type (
	requestIDKey struct{}
	identityKey  struct{}
	errorKey     struct{}
)

const unknownRequestID = "unknown"

// WithIdentity stores authenticated identity in ctx.
func WithIdentity(ctx context.Context, id *identity.Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// WithRequestID stores request id in ctx.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// Identity returns identity from ctx (if any).
func Identity(ctx context.Context) (*identity.Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(*identity.Identity)
	return id, ok && id != nil
}

// RequestID returns request id from ctx (if any).
func RequestID(ctx context.Context) (string, bool) {
	rid, ok := ctx.Value(requestIDKey{}).(string)
	return rid, ok && rid != ""
}

// TryRequestID returns request id from ctx (if any).
func TryRequestID(ctx context.Context) string {
	if rid, ok := RequestID(ctx); ok {
		return rid
	}
	return unknownRequestID
}

// errorHolder is a request-scoped, mutable slot stored in context.
// Response helpers can record a short error reason AFTER the logger has already captured the context;
// the logger reads it back once the handler returns.
type errorHolder struct{ msg atomic.Pointer[string] }

func (h *errorHolder) set(msg string) { h.msg.Store(&msg) }

func (h *errorHolder) get() string {
	if p := h.msg.Load(); p != nil {
		return *p
	}
	return ""
}

// WithErrorSlot installs an empty error slot in ctx.
//
// Context values flow only downward, that's why a slot must be installed by the outermost layer (RequestID);
// a deeper one would be invisible to the logger:
//
//	RequestID   ── install ──┐   owns the slot (outermost)
//	     Logger ── read ◄────┤   reads it back after the handler returns
//	    handler ── write ────┘   SetError() somewhere in the middle
func WithErrorSlot(ctx context.Context) context.Context {
	return context.WithValue(ctx, errorKey{}, &errorHolder{})
}

// SetError writes a short error reason into the context slot.
func SetError(ctx context.Context, msg string) {
	if h, ok := ctx.Value(errorKey{}).(*errorHolder); ok {
		h.set(msg)
	}
}

// TryError returns the error reason from the context (empty if none).
func TryError(ctx context.Context) string {
	if h, ok := ctx.Value(errorKey{}).(*errorHolder); ok {
		return h.get()
	}
	return ""
}
