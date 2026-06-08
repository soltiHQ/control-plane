# internal/proxy
Outbound communication with agents.
The control plane calls INTO agents to list tasks, apply specs, stream logs, etc.

## Package map
```text
proxy/
├── proxy.go        AgentProxy interface, request/response DTOs
├── pool.go         Pool — connection manager (HTTP transport + gRPC conn cache)
├── httpclient.go   httpClient interface, proto-JSON GET/PUT/DELETE helpers
├── v1_http.go      httpProxyV1 — AgentProxy over HTTP (API v1)
├── v1_grpc.go      grpcProxyV1 — AgentProxy over gRPC (API v1)
├── convert.go      domain ⇆ proto conversion (SpecToProto, task mapping)
├── agenterr.go     AgentError — anti-corruption: agent wire status → errkind.Kind
└── error.go        sentinel errors
```

## Request flow
```text
  sync runner / handler
        │
        ▼
  Pool.Get(endpoint, type, version)
        │
   ┌────┴────────────────┐
   │ HTTP                │ gRPC
   │ httpProxyV1{        │ grpcProxyV1{
   │   endpoint, client  │   conn (cached)
   │ }                   │ }
   └────┬────────────────┘
        │
        ▼
  AgentProxy.ApplyTask / ListTasks / …
        │
   ┌────┴────────────────┐
   │ proto-JSON over HTTP │ taskv1.TaskServiceClient
   │ (httpclient.go)      │ (proto-generated)
   └─────────────────────┘
```

## Pool
```text
  Pool
  ├── httpCli    *http.Client              shared, Transport pools TCP connections
  └── grpcConns  map[endpoint]*ClientConn  one conn per endpoint, double-check lock
```
- `Get(endpoint, type, version)` dispatches to versioned factory (`getV1`)
- `Close()` drains HTTP idle conns + closes all gRPC conns
- Outbound TLS (CP-as-client) is configured at `NewPool`; nil keeps the
  pre-TLS plaintext behavior (HTTP TLS-1.2 for `https://`, gRPC insecure).

## AgentProxy interface
```go
type AgentProxy interface {
    ListTasks(ctx, filter)        → (*ListTasksResponse, error)
    ApplyTask(ctx, submission)    → (taskID string, error)   // supersede-or-install
    GetTask(ctx, taskID)          → (*GetTaskResponse, error)
    ListTaskRuns(ctx, taskID)     → (*ListTaskRunsResponse, error)
    DeleteTask(ctx, taskID)       → error                     // idempotent
    StreamTaskLogs(ctx, taskID)   → (<-chan *StreamTaskLogsResponse, error)
}
```
Methods beyond `ListTasks`/`ApplyTask` require the agent to advertise the
matching capability; callers must check `agent.HasCapability` before invoking.

## API v1 support matrix

| Method           | HTTP | gRPC |
|------------------|------|------|
| `ListTasks`      | ✓    | ✓    |
| `ApplyTask`      | ✓    | ✓    |
| `GetTask`        | ✓    | ✓    |
| `ListTaskRuns`   | ✓    | ✓    |
| `DeleteTask`     | ✓    | ✓    |
| `StreamTaskLogs` | ✓    | ✓    |

## Error handling
Every agent call wraps its result through `agentError`, producing an
`*AgentError` that (a) keeps the op sentinel + underlying error in the chain
(for `errors.Is`) and (b) translates the agent's wire status — gRPC code or the
HTTP status carried by `unexpectedStatusError` — into an `errkind.Kind` at the
boundary. Downstream code classifies via the single `errkind` contract without
sniffing transport details. Mid-stream `StreamTaskLogs` failures are surfaced
by closing the channel, not via an error return.

## HTTP helpers (httpclient.go)
| Helper                      | Purpose                                                  |
|-----------------------------|----------------------------------------------------------|
| `doProtoJSONGet`            | GET + decode proto-JSON into a `proto.Message`           |
| `doProtoJSONPutDecoding`    | PUT proto-JSON body, decode proto-JSON response          |
| `doDelete`                  | DELETE, accept 200 / 204                                 |

All use the `httpClient` interface (`Do`) for testability. Non-2xx responses
surface the SDK error envelope (`{"error","message"}`) with the status code via
`formatUnexpectedStatus`. Timeouts are controlled by the caller's `ctx`.
