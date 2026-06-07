// Package interceptor provides gRPC server interceptors (unary and streaming) for the control-plane server.
//
// Unary:
//   - UnaryRequestID         ensures every request carries a unique ID for log correlation.
//   - UnaryAuth              verifies access tokens and stores identity in context.
//   - UnaryRequirePermission guards RPCs by checking identity permissions.
//   - UnaryLogger            structured request/response logging with zerolog.
//   - UnaryRecovery          catches panics and returns codes.Internal to the client.
//   - UnaryRateLimit         per-IP failure-based throttle (codes.ResourceExhausted).
//   - UnaryLeader            rejects write RPCs on followers with codes.Unavailable + x-leader.
//
// Streaming counterparts (long-lived RPCs: log tail, watches):
//   - StreamRequestID, StreamLogger, StreamRecovery, StreamRateLimit, StreamLeader.
package interceptor
