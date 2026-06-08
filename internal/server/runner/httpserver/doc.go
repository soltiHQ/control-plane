// Package httpserver implements a server.Runner that manages the lifecycle of an [http.Server]:
//   - Builds the server from a provided [http.Handler] and timeout config
//   - Binds a TCP listener on the configured address
//   - Graceful shutdown via [http.Server.Shutdown] with hard-close fallback.
package httpserver
