package model

import (
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Rollout] = (*Rollout)(nil)

// Rollout tracks the reconciliation state of a Spec on a specific agent.
type Rollout struct {
	createdAt    time.Time
	updatedAt    time.Time
	lastPushedAt time.Time
	lastSyncedAt time.Time

	desiredGeneration  int
	observedGeneration int
	attempts           int

	id           string
	specID       string
	agentID      string
	actualTaskID string
	errMsg       string

	status enum.SyncStatus
	intent enum.RolloutIntent
}

// RolloutID returns the deterministic identifier for a Spec-Agent pair.
func RolloutID(specID, agentID string) string {
	return "rid-" + specID + "-" + agentID
}

// NewRollout creates a new Rollout for a Spec-Agent pair with Install intent.
func NewRollout(specID, agentID string, desiredGeneration int) (*Rollout, error) {
	if specID == "" || agentID == "" {
		return nil, domain.ErrEmptyID
	}
	now := time.Now()
	return &Rollout{
		createdAt: now,
		updatedAt: now,

		id:      RolloutID(specID, agentID),
		specID:  specID,
		agentID: agentID,

		desiredGeneration: desiredGeneration,
		status:            enum.SyncStatusPending,
		intent:            enum.RolloutIntentInstall,
	}, nil
}

// ID returns the rollout's unique identifier.
func (ss *Rollout) ID() string { return ss.id }

// SpecID returns the associated Spec ID.
func (ss *Rollout) SpecID() string { return ss.specID }

// AgentID returns the target agent ID.
func (ss *Rollout) AgentID() string { return ss.agentID }

// DesiredGeneration returns the Spec generation.
func (ss *Rollout) DesiredGeneration() int { return ss.desiredGeneration }

// ObservedGeneration returns the Spec generation most recently installed on the agent.
func (ss *Rollout) ObservedGeneration() int { return ss.observedGeneration }

// Status returns the current sync status.
func (ss *Rollout) Status() enum.SyncStatus { return ss.status }

// Intent returns what the sync runner should do on the next tick.
func (ss *Rollout) Intent() enum.RolloutIntent { return ss.intent }

// ActualTaskID returns the TaskId the agent reported on the last successful ApplyTask.
func (ss *Rollout) ActualTaskID() string { return ss.actualTaskID }

// LastPushedAt returns when the spec was last pushed to the agent.
func (ss *Rollout) LastPushedAt() time.Time { return ss.lastPushedAt }

// LastSyncedAt returns when the agent last confirmed sync.
func (ss *Rollout) LastSyncedAt() time.Time { return ss.lastSyncedAt }

// Error returns the last error message.
func (ss *Rollout) Error() string { return ss.errMsg }

// Attempts returns the retry counter.
func (ss *Rollout) Attempts() int { return ss.attempts }

// CreatedAt returns the creation timestamp.
func (ss *Rollout) CreatedAt() time.Time { return ss.createdAt }

// UpdatedAt returns the last modification timestamp.
func (ss *Rollout) UpdatedAt() time.Time { return ss.updatedAt }

// SetCreatedAt restores the creation timestamp (persistence hook).
func (ss *Rollout) SetCreatedAt(t time.Time) { ss.createdAt = t }

// SetUpdatedAt restores the modification timestamp (persistence hook).
func (ss *Rollout) SetUpdatedAt(t time.Time) { ss.updatedAt = t }

// SetIntent records the next reconciliation action the sync runner should take.
func (ss *Rollout) SetIntent(intent enum.RolloutIntent) {
	if ss.intent == intent {
		return
	}
	ss.updatedAt = time.Now()
	ss.intent = intent
}

// SetActualTaskID records the TaskId the agent returned for this rollout.
// Pass an empty string to clear (e.g. when the rollout has no live task).
func (ss *Rollout) SetActualTaskID(taskID string) {
	if ss.actualTaskID == taskID {
		return
	}
	ss.updatedAt = time.Now()
	ss.actualTaskID = taskID
}

// MarkPending resets the rollout so the sync runner treats it as an actionable item on the next tick.
func (ss *Rollout) MarkPending(desiredGeneration int) {
	ss.desiredGeneration = desiredGeneration
	ss.updatedAt = time.Now()

	ss.status = enum.SyncStatusPending
	ss.attempts = 0
	ss.errMsg = ""
}

// MarkSynced marks the agent as having the exact spec generation applied.
func (ss *Rollout) MarkSynced(observedGeneration int) {
	ss.observedGeneration = observedGeneration
	ss.lastSyncedAt = time.Now()
	ss.updatedAt = time.Now()

	ss.intent = enum.RolloutIntentNoop
	ss.status = enum.SyncStatusSynced
	ss.errMsg = ""
}

// MarkDrift marks a version mismatch detected via export.
func (ss *Rollout) MarkDrift() {
	ss.status = enum.SyncStatusDrift
	ss.updatedAt = time.Now()
}

// MarkFailed records a push failure.
func (ss *Rollout) MarkFailed(errMsg string) {
	ss.status = enum.SyncStatusFailed
	ss.lastPushedAt = time.Now()
	ss.updatedAt = time.Now()

	ss.errMsg = errMsg
	ss.attempts++
}

// MarkUnknown sets the state when the agent is unreachable.
func (ss *Rollout) MarkUnknown() {
	ss.status = enum.SyncStatusUnknown
	ss.updatedAt = time.Now()
}

// SetLastPushedAt records a push attempt timestamp.
func (ss *Rollout) SetLastPushedAt(t time.Time) {
	ss.lastPushedAt = t
	ss.updatedAt = time.Now()
}

// IsStaleFor reports whether this rollout still has work to do for a spec at `generation`.
func (ss *Rollout) IsStaleFor(generation int) bool {
	if ss == nil {
		return false
	}
	return ss.intent != enum.RolloutIntentNoop || ss.observedGeneration < generation
}

// Clone creates a deep copy of the Rollout.
func (ss *Rollout) Clone() *Rollout {
	return &Rollout{
		createdAt:    ss.createdAt,
		updatedAt:    ss.updatedAt,
		lastPushedAt: ss.lastPushedAt,
		lastSyncedAt: ss.lastSyncedAt,

		id:           ss.id,
		specID:       ss.specID,
		agentID:      ss.agentID,
		actualTaskID: ss.actualTaskID,
		errMsg:       ss.errMsg,

		desiredGeneration:  ss.desiredGeneration,
		observedGeneration: ss.observedGeneration,
		attempts:           ss.attempts,

		status: ss.status,
		intent: ss.intent,
	}
}
