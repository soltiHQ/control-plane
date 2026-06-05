package model

import (
	"reflect"
	"slices"
	"time"

	"github.com/soltiHQ/control-plane/domain"
	"github.com/soltiHQ/control-plane/domain/enum"
)

var _ domain.Entity[*Spec] = (*Spec)(nil)

// BackoffConfig holds backoff parameters for task restart delays.
type BackoffConfig struct {
	Jitter  enum.JitterStrategy
	Factor  float64
	FirstMs int64
	MaxMs   int64
}

// Spec represents a desired task specification managed by the control-plane.
type Spec struct {
	id                string
	name              string
	version           int
	generation        int
	deletionRequested bool
	targets           []string
	targetLabels      map[string]string
	createdAt         time.Time
	updatedAt         time.Time

	slot         string
	kindType     enum.TaskKindType
	restartType  enum.RestartType
	timeoutMs    int64
	intervalMs   int64
	backoff      BackoffConfig
	kindConfig   map[string]any
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
		restartType:  enum.RestartNever,
		timeoutMs:    30000,
		intervalMs:   0,
		backoff: BackoffConfig{
			Jitter:  enum.JitterNone,
			FirstMs: 1000,
			MaxMs:   5000,
			Factor:  2.0,
		},
		kindConfig:   make(map[string]any),
		runnerLabels: make(map[string]string),
	}, nil
}

// ID returns the spec's unique identifier.
func (ts *Spec) ID() string { return ts.id }

// Name returns the human-readable spec name.
func (ts *Spec) Name() string { return ts.name }

// Slot returns the immutable task slot — its lane/identity on the agent.
func (ts *Spec) Slot() string { return ts.slot }

// Version returns the edits counter (bumps on every Upsert).
func (ts *Spec) Version() int { return ts.version }

// Generation returns the runtime-change counter (drives re-create on agents).
func (ts *Spec) Generation() int { return ts.generation }

// DeletionRequested reports whether the spec is soft-deleted and awaiting finalization.
func (ts *Spec) DeletionRequested() bool { return ts.deletionRequested }

// CreatedAt returns the creation timestamp.
func (ts *Spec) CreatedAt() time.Time { return ts.createdAt }

// UpdatedAt returns the last modification timestamp.
func (ts *Spec) UpdatedAt() time.Time { return ts.updatedAt }

// KindType returns the task backend kind.
func (ts *Spec) KindType() enum.TaskKindType { return ts.kindType }

// TimeoutMs returns the task execution timeout in milliseconds.
func (ts *Spec) TimeoutMs() int64 { return ts.timeoutMs }

// RestartType returns the task restart policy.
func (ts *Spec) RestartType() enum.RestartType { return ts.restartType }

// IntervalMs returns the restart interval in milliseconds (meaningful only for RestartAlways).
func (ts *Spec) IntervalMs() int64 { return ts.intervalMs }

// Backoff returns the restart backoff parameters.
func (ts *Spec) Backoff() BackoffConfig { return ts.backoff }

// SetCreatedAt restores the creation timestamp (persistence hook).
func (ts *Spec) SetCreatedAt(t time.Time) { ts.createdAt = t }

// SetUpdatedAt restores the modification timestamp (persistence hook).
func (ts *Spec) SetUpdatedAt(t time.Time) { ts.updatedAt = t }

// SetVersion restores the edits counter (persistence hook).
func (ts *Spec) SetVersion(v int) { ts.version = v }

// SetGeneration restores the runtime-change counter (persistence hook).
func (ts *Spec) SetGeneration(g int) { ts.generation = g }

// SetDeletionRequested restores the soft-delete flag (persistence hook).
func (ts *Spec) SetDeletionRequested(b bool) { ts.deletionRequested = b }

// KindConfig returns a defensive copy of the kind configuration.
func (ts *Spec) KindConfig() map[string]any {
	out := make(map[string]any, len(ts.kindConfig))
	for k, v := range ts.kindConfig {
		out[k] = v
	}
	return out
}

// Targets return a copy of the target agent IDs.
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

// SetName updates the spec name (metadata-only).
func (ts *Spec) SetName(name string) {
	if ts.name == name {
		return
	}
	ts.name = name
	ts.updatedAt = time.Now()
}

// SetKindType updates the task backend kind (runtime field).
func (ts *Spec) SetKindType(kt enum.TaskKindType) {
	if ts.kindType == kt {
		return
	}
	ts.kindType = kt
	ts.updatedAt = time.Now()
}

// SetKindConfig replaces the backend config with a defensive copy (runtime field).
func (ts *Spec) SetKindConfig(cfg map[string]any) {
	if anyMapEqual(ts.kindConfig, cfg) {
		return
	}
	cp := make(map[string]any, len(cfg))
	for k, v := range cfg {
		cp[k] = v
	}
	ts.kindConfig = cp
	ts.updatedAt = time.Now()
}

// SetTimeoutMs updates the execution timeout in milliseconds (runtime field).
func (ts *Spec) SetTimeoutMs(ms int64) {
	if ts.timeoutMs == ms {
		return
	}
	ts.timeoutMs = ms
	ts.updatedAt = time.Now()
}

// SetRestartType updates the restart policy (runtime field).
func (ts *Spec) SetRestartType(rt enum.RestartType) {
	if ts.restartType == rt {
		return
	}
	ts.restartType = rt
	ts.updatedAt = time.Now()
}

// SetIntervalMs updates the restart interval in milliseconds (runtime field).
func (ts *Spec) SetIntervalMs(ms int64) {
	if ts.intervalMs == ms {
		return
	}
	ts.intervalMs = ms
	ts.updatedAt = time.Now()
}

// SetBackoff updates the restart backoff parameters (runtime field).
func (ts *Spec) SetBackoff(b BackoffConfig) {
	if ts.backoff == b {
		return
	}
	ts.backoff = b
	ts.updatedAt = time.Now()
}

// SetTargets replaces the explicit target agent IDs with a defensive copy (metadata-only).
func (ts *Spec) SetTargets(targets []string) {
	if slices.Equal(ts.targets, targets) {
		return
	}
	cp := make([]string, len(targets))
	copy(cp, targets)
	ts.targets = cp
	ts.updatedAt = time.Now()
}

// SetTargetLabels replaces the target label selector with a defensive copy (metadata-only).
func (ts *Spec) SetTargetLabels(labels map[string]string) {
	if stringMapEqual(ts.targetLabels, labels) {
		return
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.targetLabels = cp
	ts.updatedAt = time.Now()
}

// SetRunnerLabels replaces the runner labels with a defensive copy (runtime field).
func (ts *Spec) SetRunnerLabels(labels map[string]string) {
	if stringMapEqual(ts.runnerLabels, labels) {
		return
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	ts.runnerLabels = cp
	ts.updatedAt = time.Now()
}

// IncrementVersion bumps the edits counter (version) and updates the timestamp. Callers should invoke this on every Upsert.
func (ts *Spec) IncrementVersion() {
	ts.version++
	ts.updatedAt = time.Now()
}

// BumpGeneration bumps the runtime-change counter.
func (ts *Spec) BumpGeneration() {
	ts.generation++
	ts.updatedAt = time.Now()
}

// MarkForDeletion flips the soft-delete flag.
func (ts *Spec) MarkForDeletion() {
	if ts.deletionRequested {
		return
	}
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

// RuntimeEquals reports whether two specs are equals.
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
