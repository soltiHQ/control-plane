// Package lifecycle implements a server.Runner that periodically checks agent liveness.
//
// Transitions agents through status stages: (active → inactive → disconnected → deleted)
// Thresholds are expressed as multiples of each agent's heartbeat interval.
package lifecycle
