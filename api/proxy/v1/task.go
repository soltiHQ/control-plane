package proxyv1

// Task is the current observed state of one task on an agent.
type Task struct {
	CreatedAt int64 `json:"created_at"`
	UpdatedAt int64 `json:"updated_at"`
	Attempt   int   `json:"attempt"`

	ID     string `json:"id"`
	Slot   string `json:"slot"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`

	ResourceVersion uint64 `json:"resource_version,omitempty"`
	ExitCode        *int32 `json:"exit_code,omitempty"`
}

// ListTasksResponse is the response for listing agent tasks.
type ListTasksResponse struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

// TaskRun represents a single execution attempt of a task.
type TaskRun struct {
	StartedAt  int64 `json:"started_at"`
	FinishedAt int64 `json:"finished_at,omitempty"`
	Attempt    int   `json:"attempt"`

	Status string `json:"status"`
	Error  string `json:"error,omitempty"`

	ExitCode *int32 `json:"exit_code,omitempty"`
}

// ListTaskRunsResponse is the response for listing task run history.
type ListTaskRunsResponse struct {
	Runs []TaskRun `json:"runs"`
}

// GetTaskResponse is the response for getting a single task by id.
type GetTaskResponse struct {
	Task *Task `json:"task,omitempty"`
}
