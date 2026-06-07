// Package status maps domain errors to gRPC statuses for the control-plane's gRPC handlers and interceptors.
//
//   - FromError(ctx, err) classifies a domain error and returns a *status.Status with the matching code,
//     a stable client message, and the request ID attached as an errdetails.RequestInfo detail.
//   - Errorf(ctx, code, ...) builds an explicit status when the caller already knows the code.
package status
