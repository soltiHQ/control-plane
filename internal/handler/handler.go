// Package handler implements HTTP and gRPC request handlers for the control plane:
//   - REST + HTMX endpoints for specs, agents, users, sessions
//   - Full-page HTML renders (login, dashboard, detail pages)
//   - Embedded static file serving (CSS, JS, images)
//   - Agent discovery/heartbeat over HTTP
//   - Agent discovery/heartbeat over gRPC
package handler
