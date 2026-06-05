package model

import (
	"reflect"
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Spec] = (*Spec)(nil)

// BackoffConfig holds backoff parameters for task restart delays.
type BackoffConfig struct {
	Jitter  enum.JitterStrategy
	FirstMs int64
	MaxMs   int64
	Factor  float64
}

// Spec represents a desired task specification managed by the control-plane.
//
// A Spec defines what task should run on which agents. It is the "desired state"
// in the reconciliation model — the sync runner compares it against what agents actually have.
//
// # Versioning: two counters
//
// A Spec exposes two monotonic counters with different meanings:
//
//   - **Version** bumps on every `Upsert`, even if only metadata changed
//     (rename, target-list edit, etc.). UI-facing "edits counter" useful
//     for audit and for identifying which save the user is looking at.
//   - **Generation** bumps only when runtime fields (those that end up in
//     `SpecToProto` — slot, kind, timeout, restart, backoff, runnerLabels)
//     change. Rollouts track `ObservedGeneration`; a re-create on the agent
//     is triggered only when `ObservedGeneration != Generation`. This
//     prevents pure metadata edits from churning live tasks on every agent.
//
// # Soft delete
//
// `DeletionRequested` flips on `DELETE /specs/{id}`: the Spec record stays
// around so the sync runner can honor rollout uninstalls (DeleteTask on each
// agent) before the finalizer actually drops the Spec row. This mirrors
// the k8s `deletionTimestamp` + finalizer pattern.
//
// The spec fields mirror the agent's CreateSpec format: slot, kind, timeout,
// restart, backoff, admission, and runner labels.
type Spec struct {
	// CP metadata
	id                string
	name              string
	version           int
	generation        int
	deletionRequested bool
	targets           []string          // concrete agent IDs
	targetLabels      map[string]string // label selector for dynamic targeting
	createdAt         time.Time
	updatedAt         time.Time

	// Spec (mirrors agent CreateSpec)
	//
	// Admission is intentionally absent. The CP reconciles desired state and
	// upgrades via ApplyTask, which force-replaces at the agent — so only
	// Replace semantics ever apply (see internal/proxy/convert.go).
	// DropIfRunning/Queue are direct-submitter policies that don't fit
	// desired-state rollouts, so a per-spec value would be dead state.
	//
	// slot is immutable: set once in NewSpec, never changed. It is the task's
	// identity/lane on the agent — a different slot is a new deployment, not
	// an edit (there is deliberately no SetSlot). Changing it would orphan the
	// task in the old slot, since ApplyTask supersedes only within a slot.
	slot         string
	kindType     enum.TaskKindType
	kindConfig   map[string]any // e.g. {command, args, env, cwd, failOnNonZero} for subprocess
	timeoutMs    int64
	restartType  enum.RestartType
	intervalMs   int64 // only for RestartAlways
	backoff      BackoffConfig
	runnerLabels map[string]string
}

// NewSpec creates a new Spec domain entity with sensible defaults.
func NewSpec(id, name, slot string) (*Spec, error) {
	if id == "" {
		return nil, domain.ErrEmptyID
	}
	if slot == "" {
		return nil, domain.ErrFieldEmpty
	}
	now := time.Now()
	return &Spec{
		createdAt: now,
		updatedAt: now,

		id:         id,
		name:       name,
		slot:       slot,
		version:    1,
		generation: 1,

		targets:      nil,
		targetLabels: make(map[string]string),
		kindType:     enum.TaskKindSubprocess,
		kindConfig:   make(map[string]any),
		timeoutMs:    30000,
		restartType:  enum.RestartNever,
		intervalMs:   0,
		backoff: BackoffConfig{
			Jitter:  enum.JitterNone,
			FirstMs: 1000,
			MaxMs:   5000,
			Factor:  2.0,
		},
		runnerLabels: make(map[string]string),
	}, nil
}

// --- Getters ---

func (ts *Spec) ID() string              { return ts.id }
func (ts *Spec) Name() string            { return ts.name }
func (ts *Spec) Slot() string            { return ts.slot }
func (ts *Spec) Version() int            { return ts.version }
func (ts *Spec) Generation() int         { return ts.generation }
func (ts *Spec) DeletionRequested() bool { return ts.deletionRequested }
func (ts *Spec) CreatedAt() time.Time    { return ts.createdAt }
func (ts *Spec) UpdatedAt() time.Time    { return ts.updatedAt }

// SetCreatedAt / SetUpdatedAt / SetVersion / SetGeneration — used by
// persistence adapters to restore exact state on reconstruction.
func (ts *Spec) SetCreatedAt(t time.Time)      { ts.createdAt = t }
func (ts *Spec) SetUpdatedAt(t time.Time)      { ts.updatedAt = t }
func (ts *Spec) SetVersion(v int)              { ts.version = v }
func (ts *Spec) SetGeneration(g int)           { ts.generation = g }
func (ts *Spec) SetDeletionRequested(b bool)   { ts.deletionRequested = b }
func (ts *Spec) KindType() enum.TaskKindType   { return ts.kindType }
func (ts *Spec) TimeoutMs() int64              { return ts.timeoutMs }
func (ts *Spec) RestartType() enum.RestartType { return ts.restartType }
func (ts *Spec) IntervalMs() int64             { return ts.intervalMs }
func (ts *Spec) Backoff() BackoffConfig        { return ts.backoff }

// KindConfig returns a defensive copy of the kind configuration.
func (ts *Spec) KindConfig() map[string]any {
	out := make(map[string]any, len(ts.kindConfig))
	for k, v := range ts.kindConfig {
		out[k] = v
	}
	return out
}

// Targets returns a copy of the target agent IDs.
func (ts *Spec) Targets() []string {
	out := make([]string, len(ts.targets))
	copy(out, ts.targets)
	return out
}

// TargetLabels returns a defensive copy of the target label selector.
func (ts *Spec) TargetLabels() map[string]string {
	out := make(map[string]string, len(ts.targetLabels))
	for k, v := range ts.targetLabels {
		out[k] = v
	}
	return out
}

// RunnerLabels returns a defensive copy of the runner labels.
func (ts *Spec) RunnerLabels() map[string]string {
	out := make(map[string]string, len(ts.runnerLabels))
	for k, v := range ts.runnerLabels {
		out[k] = v
	}
	return out
}

// --- Setters ---

func (ts *Spec) SetName(name string) {
	ts.name = name
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetKindType(kt enum.TaskKindType) {
	ts.kindType = kt
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetKindConfig(cfg map[string]any) {
	cp := make(map[string]any, len(cfg))
	for k, v := range cfg {
		cp[k] = v
	}
	ts.kindConfig = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTimeoutMs(ms int64) {
	ts.timeoutMs = ms
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetRestartType(rt enum.RestartType) {
	ts.restartType = rt
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetIntervalMs(ms int64) {
	ts.intervalMs = ms
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetBackoff(b BackoffConfig) {
	ts.backoff = b
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTargets(targets []string) {
	cp := make([]string, len(targets))
	copy(cp, targets)
	ts.targets = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetTargetLabels(labels map[string]string) {
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.targetLabels = cp
	ts.updatedAt = time.Now()
}

func (ts *Spec) SetRunnerLabels(labels map[string]string) {
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.runnerLabels = cp
	ts.updatedAt = time.Now()
}

// IncrementVersion bumps the edits counter (version) and updates the
// timestamp. Callers should invoke this on every Upsert.
func (ts *Spec) IncrementVersion() {
	ts.version++
	ts.updatedAt = time.Now()
}

// BumpGeneration bumps the runtime-change counter. Callers should invoke
// this on Upsert **only if** a field that `SpecToProto` reads has
// changed (slot, kind, timeout, restart/interval, backoff, admission,
// runnerLabels). Pure metadata edits (name, targets) must NOT bump
// generation — otherwise every rename would force a re-create on every
// agent.
func (ts *Spec) BumpGeneration() {
	ts.generation++
	ts.updatedAt = time.Now()
}

// MarkForDeletion flips the soft-delete flag. The Spec record survives
// until every Rollout for it is uninstalled; the sync runner's finalizer
// pass then calls `DeleteSpec` for real.
func (ts *Spec) MarkForDeletion() {
	ts.deletionRequested = true
	ts.updatedAt = time.Now()
}

// Clone creates a deep copy of the Spec.
func (ts *Spec) Clone() *Spec {
	kindConfig := make(map[string]any, len(ts.kindConfig))
	for k, v := range ts.kindConfig {
		kindConfig[k] = v
	}
	targets := make([]string, len(ts.targets))
	copy(targets, ts.targets)
	targetLabels := make(map[string]string, len(ts.targetLabels))
	for k, v := range ts.targetLabels {
		targetLabels[k] = v
	}
	runnerLabels := make(map[string]string, len(ts.runnerLabels))
	for k, v := range ts.runnerLabels {
		runnerLabels[k] = v
	}

	return &Spec{
		id:                ts.id,
		name:              ts.name,
		version:           ts.version,
		generation:        ts.generation,
		deletionRequested: ts.deletionRequested,
		targets:           targets,
		targetLabels:      targetLabels,
		createdAt:         ts.createdAt,
		updatedAt:         ts.updatedAt,

		slot:         ts.slot,
		kindType:     ts.kindType,
		kindConfig:   kindConfig,
		timeoutMs:    ts.timeoutMs,
		restartType:  ts.restartType,
		intervalMs:   ts.intervalMs,
		backoff:      ts.backoff,
		runnerLabels: runnerLabels,
	}
}

// RuntimeEquals reports whether two specs are identical in every field
// that `SpecToProto` reads — i.e. every field that affects what the
// agent actually runs. Returns `true` for pure-metadata edits (name,
// targets, targetLabels, timestamps), which must NOT rotate the
// generation counter.
//
// Fields compared: slot, kindType, kindConfig, timeoutMs, restartType,
// intervalMs, backoff, runnerLabels.
//
// Reason this lives in the domain package and not in the service: it's
// a pure property of the Spec value type, used by Upsert to decide
// whether to call `BumpGeneration`. Keeping the list of runtime fields
// next to the struct definition is the easiest way to keep it honest
// when someone adds a new field and forgets to update the comparator.
func (ts *Spec) RuntimeEquals(other *Spec) bool {
	if ts == nil || other == nil {
		return ts == other
	}
	if ts.slot != other.slot ||
		ts.kindType != other.kindType ||
		ts.timeoutMs != other.timeoutMs ||
		ts.restartType != other.restartType ||
		ts.intervalMs != other.intervalMs ||
		ts.backoff != other.backoff {
		return false
	}
	if !stringMapEqual(ts.runnerLabels, other.runnerLabels) {
		return false
	}
	return anyMapEqual(ts.kindConfig, other.kindConfig)
}

// stringMapEqual returns true when both maps hold the same set of
// (key, value) pairs.
func stringMapEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// anyMapEqual compares two `map[string]any`. Nested values fall back to
// `reflect.DeepEqual` because kindConfig is user-supplied JSON — there is
// no richer type information to exploit. Called only on Upsert, not in a
// hot path.
func anyMapEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok || !reflect.DeepEqual(v, bv) {
			return false
		}
	}
	return true
}
