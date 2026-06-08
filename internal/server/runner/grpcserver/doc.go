// Package grpcserver implements a server.Runner that manages the lifecycle of a [grpc.Server]:
//   - Binds a TCP (or custom network) listener on the configured address
//   - Serves incoming RPCs until Stop is called
//   - Graceful shutdown with context-deadline fallback to hard stop.
package grpcserver
