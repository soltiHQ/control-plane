# internal/server
Unified lifecycle management for all runtime components of the control plane.

## Package map
```text
server/
├── runner.go       Runner interface (Name / Start / Stop)
├── server.go       Server orchestrator: starts, monitors, shuts down runners
├── config.go       ShutdownTimeout configuration
├── error.go        RunnerError, RunnerExitedError, sentinel errors
│
└── runner/
    ├── grpcserver/  gRPC listener → grpc.Server.Serve
    ├── httpserver/  TCP listener  → http.Server.Serve
    ├── lifecycle/   periodic agent liveness checks (active → … → deleted)
    └── sync/        periodic rollout reconciliation (push specs to agents)
```

## Runner interface
```go
type Runner interface {
    Name()  string
    Start(ctx) error   // blocks until done or ctx cancelled
    Stop(ctx)  error   // graceful shutdown within ctx deadline
}
```

## Orchestration flow
```text
  New(cfg, logger, runners...)
        │
        ▼
  Run(ctx)
        │
   ┌────┴────────────────────────┐
   │  for each runner            │
   │    go runner.Start(runCtx)  │
   └────┬────────────────────────┘
        │
   ◄────┤  wait for:
        │    • ctx.Done()           → external signal (SIGINT/SIGTERM)
        │    • runner exited        → unexpected exit or fatal error
        │
        ▼
  Shutdown(background ctx + timeout)
        │
   ┌────┴────────────────────────┐
   │  for i := len-1 .. 0       │  ← reverse order (LIFO)
   │    go runner.Stop(ctx)      │
   └────┬────────────────────────┘
        │
        ▼
  wg.Wait() → errors.Join(errs...)
```

## Runner implementations

| Runner        | Tick-based  | Purpose                                     |
|---------------|-------------|---------------------------------------------|
| `httpserver`  | no          | Serve HTTP (UI + REST API)                  |
| `grpcserver`  | no          | Serve gRPC (agent discovery)                |
| `lifecycle`   | yes         | Transition stale agents through statuses    |
| `sync`        | yes         | Push pending rollouts to agents via proxy   |

### Server runners (httpserver, grpcserver)
Both follow the same pattern:
1. `New` validates config and handler/server
2. `Start` binds a TCP listener, serves, blocks
3. `Stop` attempts graceful shutdown, falls back to hard close on timeout
4. `ready` channel synchronises Stop with listener binding

### Tick runners (lifecycle, sync)
Both follow the same pattern:
1. `New` validates store dependency
2. `Start` runs a `time.Ticker` loop, calling `tick()` each interval
3. `Stop` closes a signal channel; safe for multiple calls
4. `tick()` lists entities, filters actionable ones, applies transitions
