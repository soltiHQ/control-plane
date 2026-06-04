package restv1

import (
	"encoding/json"
)

// Spec is the REST representation of a task specification.
type Spec struct {
	KindConfig   map[string]any    `json:"kind_config,omitempty"`
	TargetLabels map[string]string `json:"target_labels,omitempty"`
	RunnerLabels map[string]string `json:"runner_labels,omitempty"`
	// CreateSpec is the exact canonical proto-JSON payload (solti.task.v1.CreateSpec)
	// the control-plane would send to an agent when submitting this spec — an
	// output-only, read-only preview (the UI shows it verbatim with a Copy button;
	// SpecCreateRequest has no input counterpart).
	//
	// Opaque island: emitted verbatim as json.RawMessage, so its shape is the SDK's
	// protojson output (camelCase + enum-as-string + uint64-as-string) — deliberately
	// NOT the snake_case used elsewhere in this DTO. Do not parse it as part of the
	// REST contract.
	CreateSpec json.RawMessage `json:"create_spec,omitempty"`
	Targets    []string        `json:"targets,omitempty"`

	BackoffFactor float64 `json:"backoff_factor"`

	TimeoutMs      int64 `json:"timeout_ms"`
	IntervalMs     int64 `json:"interval_ms,omitempty"`
	BackoffFirstMs int64 `json:"backoff_first_ms"`
	BackoffMaxMs   int64 `json:"backoff_max_ms"`

	// Version bumps on every save (UI edits counter).
	Version int `json:"version"`
	// Generation bumps only when runtime fields change — this is what
	// rollouts compare against to decide whether a re-create is needed.
	Generation int `json:"generation"`

	// DirtyForRollout is a derived flag: true when at least one rollout
	// for this spec is behind the current Generation or has a non-Noop
	// intent. The UI surfaces it as the "apply pending" banner on the
	// detail page; leave false when there are no rollouts at all.
	DirtyForRollout bool `json:"dirty_for_rollout"`
	// DeletionRequested mirrors the soft-delete flag. When true, the UI
	// shows a "Deleting…" tombstone and disables Edit/Deploy.
	DeletionRequested bool `json:"deletion_requested,omitempty"`

	ID          string `json:"id"`
	Name        string `json:"name"`
	Slot        string `json:"slot"`
	Jitter      string `json:"jitter"`
	KindType    string `json:"kind_type"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	RestartType string `json:"restart_type"`
}

// ListSpecsResponse is a cursor-paginated list of specs (next_cursor).
type ListSpecsResponse struct {
	Items      []Spec `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// SpecCreateRequest is the request body for creating/updating a spec.
//
// `Admission` is intentionally absent — the control-plane always sends
// admission=Replace on the wire regardless of any client-supplied value.
// The field was removed after alpha to avoid giving users the illusion
// that they can pick between Queue / DropIfRunning / Replace for
// CP-managed agents. If the field arrives in a request body, the
// handler's `json.Decode` ignores it (default unknown-field behaviour).
//
// `Version` is the optimistic-concurrency CAS token for update. The
// client echoes back the version it last loaded; the server rejects
// with 409 if the stored version has advanced (someone else saved in
// between). Ignored on create.
type SpecCreateRequest struct {
	TargetLabels map[string]string `json:"target_labels,omitempty"`
	RunnerLabels map[string]string `json:"runner_labels,omitempty"`
	KindConfig   map[string]any    `json:"kind_config,omitempty"`
	Targets      []string          `json:"targets,omitempty"`

	BackoffFactor float64 `json:"backoff_factor"`

	IntervalMs     int64 `json:"interval_ms,omitempty"`
	BackoffFirstMs int64 `json:"backoff_first_ms"`
	BackoffMaxMs   int64 `json:"backoff_max_ms"`
	TimeoutMs      int64 `json:"timeout_ms"`

	Version int `json:"version,omitempty"`

	RestartType string `json:"restart_type"`
	KindType    string `json:"kind_type"`
	Jitter      string `json:"jitter"`
	Name        string `json:"name"`
	Slot        string `json:"slot"`
}
