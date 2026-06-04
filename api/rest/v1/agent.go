package restv1

// Agent is the REST representation of a connected agent.
type Agent struct {
	UptimeSeconds     int64 `json:"uptime_seconds"`
	HeartbeatInterval int   `json:"heartbeat_interval_s,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`

	ID           string `json:"id"`
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	EndpointType string `json:"endpoint_type"`
	APIVersion   string `json:"api_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Platform     string `json:"platform"`
	Status       string `json:"status"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
}

// ListAgentsResponse is a cursor-paginated list of agents (next_cursor).
type ListAgentsResponse struct {
	Items      []Agent `json:"items"`
	NextCursor string  `json:"next_cursor,omitempty"`
}

// AgentPatchLabelsRequest is the request body for patching agent labels.
type AgentPatchLabelsRequest struct {
	Labels map[string]string `json:"labels"`
}
