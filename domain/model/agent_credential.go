package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
)

var _ domain.Entity[*AgentCredential] = (*AgentCredential)(nil)

// AgentCredential is the per-agent bearer secret the control plane learns at
// discovery (trust-on-first-use) and reuses to authenticate outbound calls to
// that agent. Keyed by agent ID — one credential per agent.
//
// The token is a raw secret. It is replicated through Raft so any replica can
// verify inbound discovery and authenticate outbound proxy calls, but it MUST
// NOT be serialized to any API/UI response or logged in clear.
type AgentCredential struct {
	createdAt time.Time
	updatedAt time.Time

	agentID string
	token   string
}

// NewAgentCredential binds a raw token to an agent.
func NewAgentCredential(agentID, token string) (*AgentCredential, error) {
	if agentID == "" {
		return nil, domain.ErrEmptyID
	}
	if token == "" {
		return nil, domain.ErrFieldEmpty
	}

	now := time.Now()
	return &AgentCredential{
		createdAt: now,
		updatedAt: now,
		agentID:   agentID,
		token:     token,
	}, nil
}

// ID returns the agent ID — the credential key.
func (c *AgentCredential) ID() string { return c.agentID }

// AgentID is an alias for ID for call-site clarity.
func (c *AgentCredential) AgentID() string { return c.agentID }

// Token returns the raw bearer secret.
//
// SECURITY: never serialize this outside the control plane (no API/UI exposure)
// and never log it in clear. Used only for constant-time verification and for
// authenticating outbound proxy calls to the agent.
func (c *AgentCredential) Token() string { return c.token }

// CreatedAt returns the enrollment (first-seen) timestamp.
func (c *AgentCredential) CreatedAt() time.Time { return c.createdAt }

// UpdatedAt returns the last modification timestamp.
func (c *AgentCredential) UpdatedAt() time.Time { return c.updatedAt }

// SetCreatedAt restores the creation timestamp (persistence hook).
func (c *AgentCredential) SetCreatedAt(t time.Time) { c.createdAt = t }

// SetUpdatedAt restores the modification timestamp (persistence hook).
func (c *AgentCredential) SetUpdatedAt(t time.Time) { c.updatedAt = t }

// Clone returns a deep copy. All fields are value types, so a shallow struct
// copy is already a deep copy.
func (c *AgentCredential) Clone() *AgentCredential {
	cp := *c
	return &cp
}
