// Package route provides HTTP routing helpers used across UI and API handlers.
//
// Composition:
//   - [BaseMW], [PermMW]     - middleware signatures.
//   - [Chain]                - wrap a handler with middleware left-to-right.
//   - [Handle], [HandleFunc] - register a pattern on ServeMux with a chain.
//
// REST dispatch (see dispatch.go):
//   - [CollectionHandler], [EntityHandler] - handler types with render mode.
//   - [Endpoint], [Subroute]               - method/permission/handler triples.
//   - [Guard]                              - apply permission middleware.
//   - [Resource]                           - dispatch /collection by method.
//   - [Router]                             - dispatch /collection/{id}[/action].
package route
