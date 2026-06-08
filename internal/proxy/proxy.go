// Package proxy provides outbound communication with agents.
//
// The control-plane calls INTO agents to query tasks, submit work, etc.
// Each agent exposes either an HTTP or a gRPC endpoint; the proxy package
// selects the transport based on the endpoint type reported by the agent.
package proxy

import (
	"context"

	taskv1 "github.com/soltiHQ/control-plane/api/gen/solti/task/v1"
	proxyv1 "github.com/soltiHQ/control-plane/api/proxy/v1"
)

// TaskFilter holds optional filters and pagination for listing tasks.
type TaskFilter struct {
	Limit  int
	Offset int
	Slot   string
	Status string
}

// TaskSubmission describes a task to push to an agent.
//
// Spec uses the generated proto type so that both HTTP (via protojson) and
// gRPC transports share a single wire schema (solti.task.v1.CreateSpec).
type TaskSubmission struct {
	Spec *taskv1.CreateSpec
}

// AgentProxy is the interface for outbound communication with an agent.
//
// Methods beyond ListTasks/ApplyTask require the agent to declare the
// corresponding capability ("task_runs", "task_delete"). Callers must
// check agent.HasCapability before invoking these.
type AgentProxy interface {
	ListTasks(ctx context.Context, filter TaskFilter) (*proxyv1.ListTasksResponse, error)

	// ApplyTask applies a spec to its slot: the agent supersedes whatever
	// runs there, or installs if the slot is empty, and returns the TaskId
	// running after apply. Atomic and force-replacing — the control-plane's
	// upgrade primitive, so a new spec version wins the slot deterministically
	// without a separate DeleteTask and its teardown race.
	ApplyTask(ctx context.Context, sub TaskSubmission) (taskID string, err error)

	// GetTask returns a single task by ID.
	GetTask(ctx context.Context, taskID string) (*proxyv1.GetTaskResponse, error)
	// ListTaskRuns returns execution history for a specific task.
	ListTaskRuns(ctx context.Context, taskID string) (*proxyv1.ListTaskRunsResponse, error)
	// DeleteTask stops a task on the agent and purges its run history.
	// Idempotent — the agent returns success whether or not the task is
	// currently registered. Single teardown primitive: there is no
	// "cancel without purge" on the wire (taskvisor has no paused state).
	DeleteTask(ctx context.Context, taskID string) error

	// StreamTaskLogs subscribes to the live stdout/stderr stream of a task.
	// Returns a channel of typed events (OutputChunk, RunStarted, RunFinished,
	// Lagged). The channel is closed when the agent finishes the stream, the
	// context is cancelled, or the transport drops. Pre-stream errors
	// (dial, bad task id, capability not advertised) return immediately and
	// wrap [ErrStreamTaskLogs]; mid-stream errors are surfaced by closing
	// the channel — callers should treat early close as either normal end
	// or transport failure and act accordingly.
	StreamTaskLogs(ctx context.Context, taskID string) (<-chan *taskv1.StreamTaskLogsResponse, error)
}
