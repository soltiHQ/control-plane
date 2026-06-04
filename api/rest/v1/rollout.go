package restv1

// RolloutSpec embeds Spec with a per-agent delivery state.
type RolloutSpec struct {
	Spec
	Entries []RolloutEntry `json:"rollout,omitempty"`
}

// RolloutEntry tracks the delivery state of a spec on a single agent.
type RolloutEntry struct {
	DesiredGeneration  int `json:"desired_generation"`
	ObservedGeneration int `json:"observed_generation"`
	Attempts           int `json:"attempts,omitempty"`

	LastPushedAt string `json:"last_pushed_at,omitempty"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
	AgentID      string `json:"agent_id"`
	Status       string `json:"status"`
	Intent       string `json:"intent"`
	ActualTaskID string `json:"actual_task_id,omitempty"`
	Error        string `json:"error,omitempty"`
}
