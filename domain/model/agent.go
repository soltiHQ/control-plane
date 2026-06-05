package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Agent] = (*Agent)(nil)

// Agent is a core domain entity that represents an Agent connected to the control-plane.
//
// Agent is a remote worker/node that runs an agent process and periodically reports its state to the control-plane.
//
// Notes:
//   - Metadata is agent-owned data reported by the agent (not modified).
//   - Labels are control-plane owned annotations (operators/system), not reported by the agent.
type Agent struct {
	heartbeatInterval time.Duration
	createdAt         time.Time
	updatedAt         time.Time
	lastSeenAt        time.Time
	staleAt           time.Time

	uptimeSeconds int64

	id       string
	name     string
	endpoint string

	os       string
	arch     string
	platform string

	endpointType enum.EndpointType
	apiVersion   enum.APIVersion
	status       enum.AgentStatus

	metadata     map[string]string
	labels       map[string]string
	capabilities []string
}

// NewAgent creates a new agent domain entity.
func NewAgent(id, name, endpoint string) (*Agent, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	now := time.Now()
	return &Agent{
		createdAt:  now,
		updatedAt:  now,
		lastSeenAt: now,

		id:       id,
		name:     name,
		endpoint: endpoint,

		metadata: make(map[string]string),
		labels:   make(map[string]string),

		status: enum.AgentStatusActive,
	}, nil
}

// AgentParams is set of fields for constructing an Agent from an external discovery payload (HTTP or gRPC).
type AgentParams struct {
	ID       string
	Name     string
	Endpoint string

	UptimeSeconds      int64
	HeartbeatIntervalS int

	EndpointType int
	APIVersion   int

	OS       string
	Arch     string
	Platform string

	Metadata     map[string]string
	Capabilities []string
}

// NewAgentFrom constructs an Agent from AgentParams.
func NewAgentFrom(p AgentParams) (*Agent, error) {
	if p.ID == "" {
		return nil, domain.ErrEmptyID
	}

	epType, err := enum.EndpointTypeFromInt(p.EndpointType)
	if err != nil {
		return nil, err
	}

	var (
		now  = time.Now()
		caps = make([]string, len(p.Capabilities))
		md   = make(map[string]string, len(p.Metadata))
	)
	for k, v := range p.Metadata {
		md[k] = v
	}
	copy(caps, p.Capabilities)

	return &Agent{
		createdAt: now,
		updatedAt: now,

		metadata:     md,
		labels:       make(map[string]string),
		capabilities: caps,

		id:           p.ID,
		name:         p.Name,
		endpoint:     p.Endpoint,
		endpointType: epType,
		apiVersion:   enum.APIVersionFromInt(p.APIVersion),
		os:           p.OS,
		arch:         p.Arch,
		platform:     p.Platform,

		uptimeSeconds: p.UptimeSeconds,

		status:            enum.AgentStatusActive,
		lastSeenAt:        now,
		heartbeatInterval: time.Duration(p.HeartbeatIntervalS) * time.Second,
	}, nil
}

// ID returns the agent's unique identifier.
func (a *Agent) ID() string { return a.id }

// Name returns the agent's name.
func (a *Agent) Name() string { return a.name }

// Endpoint returns the agent's network address.
func (a *Agent) Endpoint() string { return a.endpoint }

// EndpointType returns the agent's transport protocol.
func (a *Agent) EndpointType() enum.EndpointType { return a.endpointType }

// APIVersion returns the agent's API version.
func (a *Agent) APIVersion() enum.APIVersion { return a.apiVersion }

// UptimeSeconds returns the agent-reported uptime in seconds.
func (a *Agent) UptimeSeconds() int64 { return a.uptimeSeconds }

// OS returns the agent's operating system.
func (a *Agent) OS() string { return a.os }

// Arch returns the agent's architecture.
func (a *Agent) Arch() string { return a.arch }

// Platform returns the agent's platform.
func (a *Agent) Platform() string { return a.platform }

// Capabilities returns the agent's declared capabilities.
func (a *Agent) Capabilities() []string {
	out := make([]string, len(a.capabilities))
	copy(out, a.capabilities)
	return out
}

// HasCapability checks whether the agent declares a specific capability.
func (a *Agent) HasCapability(capability string) bool {
	for _, c := range a.capabilities {
		if c == capability {
			return true
		}
	}
	return false
}

// Status returns the agent's lifecycle status.
func (a *Agent) Status() enum.AgentStatus { return a.status }

// LastSeenAt returns the timestamp of the agent's last successful sync.
func (a *Agent) LastSeenAt() time.Time { return a.lastSeenAt }

// HeartbeatInterval returns the agent-reported heartbeat interval.
func (a *Agent) HeartbeatInterval() time.Duration { return a.heartbeatInterval }

// MarkStatus transitions the agent's lifecycle status.
func (a *Agent) MarkStatus(s enum.AgentStatus) {
	if a.status == s {
		return
	}
	a.status = s
	a.updatedAt = time.Now()
}

// SetStatus restores the agent's status WITHOUT bumping UpdatedAt.
func (a *Agent) SetStatus(s enum.AgentStatus) {
	a.status = s
}

// SetHeartbeatInterval sets the agent's heartbeat interval.
func (a *Agent) SetHeartbeatInterval(d time.Duration) { a.heartbeatInterval = d }

// StaleAt returns the time by which the next heartbeat is expected.
func (a *Agent) StaleAt() time.Time { return a.staleAt }

// SetStaleAt sets the staleness deadline for the agent.
func (a *Agent) SetStaleAt(t time.Time) { a.staleAt = t }

// CreatedAt returns the creation timestamp.
func (a *Agent) CreatedAt() time.Time { return a.createdAt }

// SetCreatedAt overrides the creation timestamp.
func (a *Agent) SetCreatedAt(t time.Time) { a.createdAt = t }

// UpdatedAt returns the last modification timestamp.
func (a *Agent) UpdatedAt() time.Time { return a.updatedAt }

// SetUpdatedAt overrides the modification timestamp.
func (a *Agent) SetUpdatedAt(t time.Time) { a.updatedAt = t }

// SetLastSeenAt / SetEndpointType / SetAPIVersion / SetOS / SetArch /
// SetPlatform / SetUptimeSeconds / SetMetadata / SetLabels / SetCapabilities
// are persistence-adapter hooks that restore every field on reconstruction.
func (a *Agent) SetLastSeenAt(t time.Time)           { a.lastSeenAt = t }
func (a *Agent) SetEndpointType(e enum.EndpointType) { a.endpointType = e }
func (a *Agent) SetAPIVersion(v enum.APIVersion)     { a.apiVersion = v }
func (a *Agent) SetOS(s string)                      { a.os = s }
func (a *Agent) SetArch(s string)                    { a.arch = s }
func (a *Agent) SetPlatform(s string)                { a.platform = s }
func (a *Agent) SetUptimeSeconds(v int64)            { a.uptimeSeconds = v }

// SetMetadata replaces the full metadata map with a defensive copy.
func (a *Agent) SetMetadata(m map[string]string) {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	a.metadata = cp
}

// SetLabels replaces the full label map with a defensive copy.
func (a *Agent) SetLabels(m map[string]string) {
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	a.labels = cp
}

// SetCapabilities replaces the capability list with a defensive copy.
func (a *Agent) SetCapabilities(c []string) {
	cp := make([]string, len(c))
	copy(cp, c)
	a.capabilities = cp
}

// Metadata returns the metadata value for the given key.
func (a *Agent) Metadata(key string) (string, bool) {
	v, ok := a.metadata[key]
	return v, ok
}

// MetadataAll returns a copy of the agent's metadata.
func (a *Agent) MetadataAll() map[string]string {
	out := make(map[string]string, len(a.metadata))
	for k, v := range a.metadata {
		out[k] = v
	}
	return out
}

// Label returns a label value for the given key.
func (a *Agent) Label(key string) (string, bool) {
	v, ok := a.labels[key]
	return v, ok
}

// LabelsAll returns a copy of the agent's labels.
func (a *Agent) LabelsAll() map[string]string {
	out := make(map[string]string, len(a.labels))
	for k, v := range a.labels {
		out[k] = v
	}
	return out
}

// LabelAdd sets a control-plane owned label on the agent.
func (a *Agent) LabelAdd(key, value string) {
	if existing, ok := a.labels[key]; ok && existing == value {
		return
	}
	a.labels[key] = value
	a.updatedAt = time.Now()
}

// LabelDelete removes a control-plane owned label from the agent.
func (a *Agent) LabelDelete(key string) {
	if _, ok := a.labels[key]; !ok {
		return
	}
	delete(a.labels, key)
	a.updatedAt = time.Now()
}

// Clone creates a deep copy of the agent model.
func (a *Agent) Clone() *Agent {
	var (
		md     = make(map[string]string, len(a.metadata))
		labels = make(map[string]string, len(a.labels))
		caps   = make([]string, len(a.capabilities))
	)
	for k, v := range a.metadata {
		md[k] = v
	}
	for k, v := range a.labels {
		labels[k] = v
	}
	copy(caps, a.capabilities)

	return &Agent{
		createdAt: a.createdAt,
		updatedAt: a.updatedAt,

		metadata:     md,
		labels:       labels,
		capabilities: caps,

		id:           a.id,
		name:         a.name,
		endpoint:     a.endpoint,
		endpointType: a.endpointType,
		apiVersion:   a.apiVersion,
		os:           a.os,
		arch:         a.arch,
		platform:     a.platform,

		uptimeSeconds: a.uptimeSeconds,

		status:            a.status,
		lastSeenAt:        a.lastSeenAt,
		heartbeatInterval: a.heartbeatInterval,
		staleAt:           a.staleAt,
	}
}
