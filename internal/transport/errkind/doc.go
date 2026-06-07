// Package errkind classifies a domain error into a transport-agnostic Kind.
//
// It is the single source of truth for "what does this error mean", shared by both transports.
// An error always maps to the same client-facing outcome:
//
//   - gRPC: Kind → codes.Code (internal/transport/grpc/status).
//   - HTTP: Kind → status code + helper (internal/transport/http/response).
//
// Adding a new domain error means updating Classify once, not every handler.
package errkind
