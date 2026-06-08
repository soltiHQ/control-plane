// Package base provides a leader-gated periodic server.Runner.
//
// It factors out the start/stop/leadership/ticker scaffolding shared by the background reconcilers:
// invoke a tick on a fixed interval, but only while this replica holds leadership;
// stay idle on followers;
// drain cleanly on shutdown.
package base
