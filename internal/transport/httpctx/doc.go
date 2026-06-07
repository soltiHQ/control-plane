// Package httpctx holds HTTP-specific request values.
// It complements internal/transportctx (transport-agnostic: identity, request id, error slot) with values that only make sense for HTTP.
//
// Two values, deliberately handled differently:
//
//   - Responder - the content-negotiated responder (JSON vs HTML).
//     It is a decision made once by the Negotiate middleware (path + render mode).
//   - RenderMode - full page vs HTMX fragment.
//     It is a pure function of the request header (HX-Request).
package httpctx
