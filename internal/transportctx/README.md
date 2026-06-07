# internal/transportctx

Transport context values are shared across HTTP and gRPC layers.
  
The package owns three typed context keys - **Identity**, **RequestID**, and **ErrorSlot** - and provides getters/setters for each.
Because the keys are unexported structs, no other package can collide with them.

## What goes into context
| Value                 | Writer                                               | Reader                                       |
|-----------------------|------------------------------------------------------|----------------------------------------------|
| `*identity.Identity`  | Auth middleware / interceptor                        | Handlers, loggers, permission checks         |
| `string` (request ID) | RequestID middleware / interceptor                   | Loggers, error responders                    |
| `*errorHolder` (slot) | RequestID middleware / interceptor (`WithErrorSlot`) | Logger middleware / interceptor (`TryError`) |

### Error slot
A mutable `errorHolder` stored in context, error response helpers can write a reason **after** the logger has already captured the context. 
The reason is held in an `atomic.Pointer[string]` and the handler-side writing and the later logger-side reading are race-free.

- **Init**: `WithErrorSlot(ctx)` - installed by the **RequestID** middleware/interceptor (HTTP and gRPC, unary and stream).
- **Write**: `SetError(ctx, msg)` - called by `response.*` (HTTP) and `status.*` (gRPC) helpers. No-op if the slot was not initialized.
- **Read**: `TryError(ctx)` - called by the Logger middleware/interceptor to append the `"error"` field to the log line.

> **Ordering invariant.** Context values propagate only **downward**, so the slot must be installed by the **outermost** request-scoped layer (RequestID); that is why RequestID owns it, not Logger. 
> Every writer (handlers, response/status helpers) and the reader (Logger) sit inside RequestID and therefore share the same slot. 
> If a route is wired without RequestID, `SetError` silently no-ops.

## Request lifecycle

```text
  incoming request (HTTP or gRPC)
    │
    ▼
  ┌──────────────────────────────┐
  │  RequestID middleware        │  WithRequestID(ctx, rid)
  │  ── extract or generate ──   │  WithErrorSlot(ctx)
  └──────────────┬───────────────┘
                 │
                 ▼
  ┌──────────────────────────────┐
  │  Auth middleware             │  WithIdentity(ctx, id)
  │  ── verify token ──          │
  └──────────────┬───────────────┘
                 │
                 ▼
  ┌──────────────────────────────┐
  │  Handler / Interceptor       │  Identity(ctx), RequestID(ctx)
  │  ── business logic ──        │  SetError(ctx, msg) via response/status helpers
  └──────────────┬───────────────┘
                 │
                 ▼
  ┌──────────────────────────────┐
  │  Logger middleware           │  TryError(ctx) → "error" field in log
  │  ── log request ──           │
  └──────────────────────────────┘
```

## Why a separate package
HTTP middleware lives in `internal/transport/http/middleware`, gRPC interceptors in `internal/transport/grpc/interceptors`.
Both need to write and read the same context values.  
Putting the keys here avoids a circular import:
```text
  transport/http/middleware ──┐
                              ├──→ transportctx ←── handler/api.go
  transport/grpc/interceptors ┘                 ←── handler/ui.go
                                                ←── loggers, responders
```
