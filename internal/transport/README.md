# internal/transport

Transport layer - protocol-specific wiring between the network and the domain handlers. 
HTTP and gRPC are kept symmetric where the protocols allow, and share two transport-agnostic packages so handlers stay protocol-neutral:
- `internal/transportctx` (sibling, outside this tree to avoid import cycles) – identity, request id, and the mutable error slot.
- `errkind` - the single error → category classifier consumed by both transports.

## Package map

```text
transport/
├── grpc/
│   ├── interceptor/   unary + stream interceptors
│   └── status/        Kind → gRPC code + requestID detail
│
├── http/
│   ├── middleware/    pipeline 
│   ├── responder/     Responder interface + JSON / HTML implementations
│   ├── response/      one-call helpers (OK, NotFound, …) + FromError
│   └── route/         middleware chaining + REST dispatch (Resource, Router)
│
├── httpctx/           HTTP-only request context (Responder, RenderMode)
├── tlsconf/           PEM config → *tls.Config (server + client, mTLS)
└── errkind/           domain/agent error → Kind (shared by both transports)
```

## Request lifecycle

### HTTP

```text
  Browser / API client
        │
        ▼
  RequestID   ── installs request id + error slot (outermost, owns the slot)
   Logger     ── reads the slot back after the handler returns
    Negotiate ── picks the Responder (JSON vs HTML), stores it in ctx
     
  Auth / RequirePermission (per route)
    handler/api.go | ui.go
        ├─ success → response.OK(w, r, mode, &View{Data: dto, Component: tmpl})
        │              └─ Responder from ctx → JSON (json.Marshal + sec headers)
        │                                    or HTML (templ.Render + CSP)
        └─ error   → response.FromError(w, r, mode, err)   ← classifies via errkind
                       └─ sets the error slot → Logger appends it to the log line
```

`RenderMode` (full page vs HTMX fragment) is a pure function of the request header, derived on demand via `httpctx.ModeFromRequest(r)`: 
only the negotiated `Responder` is stored in context.

### gRPC

```text
  gRPC client
        ▼
  UnaryRecovery → UnaryRequestID → UnaryLogger → UnaryRateLimit → UnaryLeader
        │   (UnaryAuth / UnaryRequirePermission exist but are not chained yet)
        ▼
  handler/discovery.go (GRPCDiscovery)
        │
        ├─ success → proto response
        └─ error   → status.FromError(ctx, err)            ← classifies via errkind
                       ├─ Kind → gRPC code + stable message
                       ├─ attaches requestID as errdetails.RequestInfo
                       └─ sets the error slot → UnaryLogger appends it
```

## Error model (shared)

A domain error maps to the same client-facing outcome on both transports because classification lives in **one** place: `errkind.Classify(err) → Kind`. 
Each transport only maps `Kind` to its wire form.

```text
                                    ┌─────────────────────────────┐
   domain sentinels ──────────────► │  errkind.Classify → Kind    │ ◄─── proxy AgentError
   (auth.*, storage.*, context.*)   │  (single source of truth)   │      (ErrorKind() tag)
                                    └──────────────┬──────────────┘
                                                   │
                            ┌──────────────────────┴───────────────────────┐
                            ▼                                              ▼
               http/response.FromError → status + helper        grpc/status.FromError → codes.*
```

Errors coming **from an agent** through the proxy are translated at the boundary:
`internal/proxy` is an anti-corruption layer that reads the agent's gRPC code / HTTP status and tags the error with a `Kind` (`AgentError.ErrorKind()`). 
Downstream code never sniffs gRPC/HTTP details - it just calls `errkind.Classify`, which trusts the tag. 
`NotFound` from an agent surfaces to the user as 404 / NotFound.

## TLS

`tlsconf` turns PEM file config into `*tls.Config`; the shared `config.TLS` block drives it. 
Both halves are opt-in (empty = plaintext):
- `tls.server` → the CP's serving identity for the HTTP, HTTP-discovery, and gRPC listeners (`client_ca_file` enables mTLS - client cert required).
- `tls.client` → the CP as a client when dialing agents (proxy); verifies agent certs and optionally presents a client cert.

Applied uniformly: one server `*tls.Config` for all listeners, one client config for the proxy pool.
